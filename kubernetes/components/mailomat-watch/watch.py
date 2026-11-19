"""Copies every mailomat event into the pod's log and counts them for Prometheus (README).

mailomat keeps a domain's events only briefly, so every few minutes this fetches each domain's
newest events, prints the ones it has not seen as one JSON line each (the event as mailomat
returns it, plus "domain") and counts them by type. The IDs it has seen live in the ConfigMap
mailomat-watch-state, which this pod creates and updates itself; for a local test, STATE_FILE
points to a file instead.

The tokens come from the apps that send: each puts a Secret labelled d3strukt0r.dev/mailomat-token
into this namespace, every key a sending domain and its value that domain's API token. They are
looked up on every pass, so an app's token arrives or leaves without a restart. For a local test,
TOKENS_FILE points to a JSON file {"<domain>": "<token>"} instead.
"""

import base64
import http.server
import json
import os
import ssl
import sys
import threading
import time
import urllib.error
import urllib.request

EVENTS_URL = "https://api.mailomat.swiss/events"
PAGE_SIZE = 100
SEEN_PER_DOMAIN = 1000
STATE_CONFIGMAP = "mailomat-watch-state"
TOKEN_LABEL = "d3strukt0r.dev/mailomat-token"
SERVICE_ACCOUNT = "/var/run/secrets/kubernetes.io/serviceaccount"

INTERVAL = int(os.environ.get("POLL_INTERVAL_SECONDS", "300"))

metrics_lock = threading.Lock()
events_total = {}
poll_errors_total = {}
poll_last_success = {}
loop_alive = time.time()


def request(url, token, method="GET", body=None, headers=None, context=None):
    data = None if body is None else body.encode()
    req = urllib.request.Request(url, data=data, method=method, headers=headers or {})
    req.add_header("Authorization", f"Bearer {token}")
    with urllib.request.urlopen(req, timeout=30, context=context) as response:
        return response.read().decode()


def kubernetes(method, path, body=None):
    with open(f"{SERVICE_ACCOUNT}/token") as token_file:
        token = token_file.read()
    context = ssl.create_default_context(cafile=f"{SERVICE_ACCOUNT}/ca.crt")
    url = f"https://kubernetes.default.svc{path}"
    return request(url, token, method, body, {"Content-Type": "application/json"}, context)


def namespace_path(kind, name=""):
    with open(f"{SERVICE_ACCOUNT}/namespace") as namespace_file:
        namespace = namespace_file.read()
    return f"/api/v1/namespaces/{namespace}/{kind}{'/' + name if name else ''}"


def configmap_path(name=""):
    return namespace_path("configmaps", name)


def tokens():
    """Every sending domain's token, from the labelled Secrets the apps put here."""
    if os.environ.get("TOKENS_FILE"):
        with open(os.environ["TOKENS_FILE"]) as tokens_file:
            return json.load(tokens_file)
    secrets = json.loads(kubernetes("GET", f"{namespace_path('secrets')}?labelSelector={TOKEN_LABEL}%3Dtrue"))
    found = {}
    for secret in secrets["items"]:
        for domain, token in (secret.get("data") or {}).items():
            found[domain] = base64.b64decode(token).decode().strip()
    return found


def load_state():
    """The IDs seen per domain, and whether the state exists yet."""
    if os.environ.get("STATE_FILE"):
        if not os.path.exists(os.environ["STATE_FILE"]):
            return {}, False
        with open(os.environ["STATE_FILE"]) as state_file:
            return json.load(state_file), True
    try:
        configmap = json.loads(kubernetes("GET", configmap_path(STATE_CONFIGMAP)))
    except urllib.error.HTTPError as error:
        if error.code == 404:
            return {}, False
        raise
    return json.loads(configmap["data"]["state.json"]), True


def save_state(state, exists):
    if os.environ.get("STATE_FILE"):
        with open(os.environ["STATE_FILE"], "w") as state_file:
            json.dump(state, state_file)
        return
    configmap = json.dumps({
        "apiVersion": "v1",
        "kind": "ConfigMap",
        "metadata": {"name": STATE_CONFIGMAP},
        "data": {"state.json": json.dumps(state)},
    })
    if exists:
        kubernetes("PUT", configmap_path(STATE_CONFIGMAP), configmap)
    else:
        kubernetes("POST", configmap_path(), configmap)


def new_events(token, seen):
    """The events not seen yet, oldest first. Pages are newest first; the first page holding a
    seen event, or an empty one, ends the walk."""
    found = []
    page = 1
    while True:
        url = f"{EVENTS_URL}?itemsPerPage={PAGE_SIZE}&page={page}&order%5BoccurredAt%5D=desc"
        events = json.loads(request(url, token, headers={"Accept": "application/json"}))
        unseen = [event for event in events if event["id"] not in seen]
        found.extend(unseen)
        if len(unseen) < len(events) or len(events) < PAGE_SIZE:
            return list(reversed(found))
        page += 1


def track(domains):
    """Metrics for the domains that have a token now. A new one starts its last success at the
    moment it appears, so a token that never works still ages into MailomatWatchStale; one whose
    app left stops being reported, so it does not either."""
    with metrics_lock:
        for domain in domains:
            poll_errors_total.setdefault(domain, 0)
            poll_last_success.setdefault(domain, time.time())
        for domain in set(poll_last_success) - set(domains):
            del poll_last_success[domain]


def poll(state, domains):
    """One pass over every domain; returns whether the state changed. A domain's first successful
    pass logs what mailomat still has without counting it: that is history, not news for an
    alert."""
    changed = False
    for domain, token in domains.items():
        known = domain in state
        seen = state.get(domain, [])
        try:
            events = new_events(token, set(seen))
        except (urllib.error.URLError, TimeoutError, ValueError) as error:
            print(json.dumps({"domain": domain, "error": str(error)}), file=sys.stderr, flush=True)
            with metrics_lock:
                poll_errors_total[domain] += 1
            continue
        for event in events:
            print(json.dumps({**event, "domain": domain}, ensure_ascii=False), flush=True)
        with metrics_lock:
            for event in events if known else []:
                key = (domain, event["eventType"])
                events_total[key] = events_total.get(key, 0) + 1
            poll_last_success[domain] = time.time()
        if events or not known:
            state[domain] = (seen + [event["id"] for event in events])[-SEEN_PER_DOMAIN:]
            changed = True
    return changed


def metrics_text():
    lines = [
        "# HELP mailomat_events_total mailomat events seen since the pod started, by type.",
        "# TYPE mailomat_events_total counter",
    ]
    with metrics_lock:
        for (domain, event_type), count in sorted(events_total.items()):
            lines.append(f'mailomat_events_total{{domain="{domain}",event_type="{event_type}"}} {count}')
        lines += [
            "# HELP mailomat_poll_errors_total Polls of a domain's events that failed.",
            "# TYPE mailomat_poll_errors_total counter",
        ]
        for domain, count in sorted(poll_errors_total.items()):
            lines.append(f'mailomat_poll_errors_total{{domain="{domain}"}} {count}')
        lines += [
            "# HELP mailomat_poll_last_success_timestamp_seconds When a domain's events were last fetched.",
            "# TYPE mailomat_poll_last_success_timestamp_seconds gauge",
        ]
        for domain, timestamp in sorted(poll_last_success.items()):
            lines.append(f'mailomat_poll_last_success_timestamp_seconds{{domain="{domain}"}} {timestamp}')
    return "\n".join(lines) + "\n"


class Handler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path == "/metrics":
            self.answer(200, metrics_text())
        elif self.path == "/healthz":
            healthy = time.time() - loop_alive < 3 * INTERVAL
            self.answer(200 if healthy else 503, "ok\n" if healthy else "loop stalled\n")
        else:
            self.answer(404, "not found\n")

    def answer(self, status, body):
        self.send_response(status)
        self.send_header("Content-Type", "text/plain; version=0.0.4")
        self.end_headers()
        self.wfile.write(body.encode())

    def log_message(self, *args):
        pass


def main():
    global loop_alive
    server = http.server.ThreadingHTTPServer(("", int(os.environ.get("METRICS_PORT", "9100"))), Handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    state, exists = load_state()
    while True:
        domains = tokens()
        track(domains)
        if poll(state, domains):
            save_state(state, exists)
            exists = True
        loop_alive = time.time()
        time.sleep(INTERVAL)


if __name__ == "__main__":
    main()
