"""Reports changes in the Hetzner Cloud API to ntfy, once per change (README).

Collects a few values as a flat dict of short strings, compares them with the state saved by the
previous run and sends one message listing what changed. Nothing changed, nothing is sent. The
state lives in the ConfigMap hetzner-watch-state, which this job creates and updates itself;
for a local test, STATE_FILE points to a file instead and DRY_RUN=1 prints the message.
"""

import json
import os
import ssl
import urllib.error
import urllib.request

HCLOUD_API = "https://api.hetzner.cloud/v1"
STATE_CONFIGMAP = "hetzner-watch-state"
SERVICE_ACCOUNT = "/var/run/secrets/kubernetes.io/serviceaccount"


def request(url, token, method="GET", body=None, headers=None, context=None):
    data = None if body is None else body.encode()
    req = urllib.request.Request(url, data=data, method=method, headers=headers or {})
    if token:
        req.add_header("Authorization", f"Bearer {token}")
    with urllib.request.urlopen(req, timeout=30, context=context) as response:
        return response.read().decode()


def hcloud(path):
    return json.loads(request(f"{HCLOUD_API}{path}", os.environ["HCLOUD_TOKEN"]))


def deprecation_text(deprecation):
    if not deprecation:
        return "none"
    return f"announced {deprecation['announced']}, unavailable after {deprecation['unavailable_after']}"


def collect():
    """The current values, as key -> short text."""
    state = {}
    location = os.environ.get("WATCH_LOCATION", "nbg1")
    watched_types = [t.strip() for t in os.environ.get("WATCH_SERVER_TYPES", "").split(",") if t.strip()]

    servers = hcloud("/servers?per_page=50")["servers"]
    used_types = {server["server_type"]["name"] for server in servers}

    for name in sorted(set(watched_types) | used_types):
        server_types = hcloud(f"/server_types?name={name}")["server_types"]
        if not server_types:
            state[f"server-type/{name}"] = "unknown"
            continue
        server_type = server_types[0]
        state[f"server-type/{name}/deprecation"] = deprecation_text(server_type["deprecation"])
        if name not in watched_types:
            continue
        locations = {entry["name"]: entry for entry in server_type.get("locations", [])}
        entry = locations.get(location)
        state[f"server-type/{name}/{location}"] = (
            "not offered" if entry is None else "orderable" if entry["available"] else "not orderable"
        )

    for server in servers:
        name = server["name"]
        state[f"server/{name}/rescue"] = "on" if server["rescue_enabled"] else "off"
        state[f"server/{name}/locked"] = "yes" if server["locked"] else "no"
        included = server.get("included_traffic") or 0
        outgoing = server.get("outgoing_traffic") or 0
        state[f"server/{name}/traffic"] = (
            "at or above 80 % of the included traffic" if included and outgoing >= 0.8 * included else "below 80 %"
        )
        image = server.get("image")
        if image:
            state[f"image/{image['name']}/deprecation"] = deprecation_text(image.get("deprecation"))

    for ip in hcloud("/primary_ips?per_page=50")["primary_ips"]:
        state[f"primary-ip/{ip['ip']}/blocked"] = "yes" if ip["blocked"] else "no"

    return state


def kubernetes(method, path, body=None):
    with open(f"{SERVICE_ACCOUNT}/token") as token_file:
        token = token_file.read()
    context = ssl.create_default_context(cafile=f"{SERVICE_ACCOUNT}/ca.crt")
    url = f"https://kubernetes.default.svc{path}"
    return request(url, token, method, body, {"Content-Type": "application/json"}, context)


def configmap_path(name=""):
    with open(f"{SERVICE_ACCOUNT}/namespace") as namespace_file:
        namespace = namespace_file.read()
    return f"/api/v1/namespaces/{namespace}/configmaps{'/' + name if name else ''}"


def load_state():
    """The previous run's values, or None on the first run."""
    if os.environ.get("STATE_FILE"):
        if not os.path.exists(os.environ["STATE_FILE"]):
            return None
        with open(os.environ["STATE_FILE"]) as state_file:
            return json.load(state_file)
    try:
        configmap = json.loads(kubernetes("GET", configmap_path(STATE_CONFIGMAP)))
    except urllib.error.HTTPError as error:
        if error.code == 404:
            return None
        raise
    return json.loads(configmap["data"]["state.json"])


def save_state(state, exists):
    if os.environ.get("STATE_FILE"):
        with open(os.environ["STATE_FILE"], "w") as state_file:
            json.dump(state, state_file, indent=2, sort_keys=True)
        return
    configmap = json.dumps({
        "apiVersion": "v1",
        "kind": "ConfigMap",
        "metadata": {"name": STATE_CONFIGMAP},
        "data": {"state.json": json.dumps(state, indent=2, sort_keys=True)},
    })
    if exists:
        kubernetes("PUT", configmap_path(STATE_CONFIGMAP), configmap)
    else:
        kubernetes("POST", configmap_path(), configmap)


def notify(title, lines):
    message = "\n".join(lines)
    if os.environ.get("DRY_RUN") == "1":
        print(f"{title}\n{message}")
        return
    request(
        f"https://ntfy.sh/{os.environ['NTFY_TOPIC']}",
        os.environ["NTFY_TOKEN"],
        "POST",
        message,
        {"Title": title, "Priority": "3", "Tags": "cloud"},
    )


def main():
    current = collect()
    previous = load_state()

    if previous is None:
        notify("Hetzner watch started", [f"{key}: {value}" for key, value in sorted(current.items())])
    else:
        changes = []
        for key in sorted(set(previous) | set(current)):
            old, new = previous.get(key, "(not watched)"), current.get(key, "(gone)")
            if old != new:
                changes.append(f"{key}: {old} -> {new}")
        if not changes:
            print("No changes")
            return
        notify("Hetzner Cloud changed", changes)

    # Saved only after the message went out, so a failed notification is sent again on the next run.
    save_state(current, previous is not None)
    print("State saved")


if __name__ == "__main__":
    main()
