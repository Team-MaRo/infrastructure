# Hetzner watch

An hourly look at the Hetzner Cloud API that tells ntfy when something there **changed** - and
nothing as long as it stays the same (`kubernetes/clusters/prod/hetzner-watch.yaml`, plain
manifests in this directory, namespace `hetzner-watch`). How the cluster's manifests fit together
is in [`kubernetes/README.md`](../../README.md).

## What it watches, and why

Signals that no other part of the cluster sees:

| Key | Values | Why |
|---|---|---|
| `server-type/<type>/nbg1` | `orderable`, `not orderable`, `not offered` | Whether a server of that type can be created in Nuremberg - for a new node, a replacement after a delete, or a resize. The watched types are `WATCH_SERVER_TYPES` (`cx33`, the type the nodes run, and `cx43`, the planned upgrade) plus every type a server currently uses. |
| `server-type/<type>/deprecation` | `none` or the announced dates | Hetzner announcing the end of a server type before a rebuild runs into it. |
| `image/<image>/deprecation` | `none` or the date | The same for the image the servers were created from. |
| `primary-ip/<ip>/blocked` | `yes`, `no` | Hetzner blocking an address for abuse - since 2026-11-01 outbound mail ports are open, so a compromised app could send spam from them. |
| `server/<name>/rescue` | `on`, `off` | Rescue mode switched on - someone working in the console, or not us. |
| `server/<name>/locked` | `yes`, `no` | Hetzner running an action on the server. |
| `server/<name>/traffic` | `below 80 %`, `at or above 80 % …` | Outgoing traffic against the 20 TB included; above them costs extra. |

Not watched here, because something else does: a server being off (Kubernetes alerts on a
missing node), volume state (the CSI driver), delete protection (drift in `tofu/hcloud`).
Not in the API at all: Hetzner's maintenance notices (email, status.hetzner.com) and the limits.

Since 2026-10 the API no longer has the datacenter endpoint; availability per location is part of
the server type (`GET /v1/server_types?name=…` → `locations[].available`).

## How it works

- **`watch.py`** (stdlib Python in `python:3.14-alpine`, mounted from a ConfigMap) collects the
  values above as `key: value` lines, compares them with the previous run's and sends one ntfy
  message listing every line that changed (`old -> new`). The first run sends "Hetzner watch
  started" with the current values, so a working setup is visible once.
- **Only changes are sent.** An unchanged hour prints "No changes" in the job's log and nothing
  else - Alertmanager would repeat a firing alert every few hours instead.
- **The message goes out before the state is saved**, so a failed notification is sent again an
  hour later rather than being lost. Any error (API, ntfy, Kubernetes) fails the job, which raises
  kube-prometheus-stack's `KubeJobFailed`, and nothing is saved.
- **The same ntfy topic as Alertmanager and Gatus** (OpenBao `secret/ntfy`), priority 3.

## The state

The previous run's values live in the ConfigMap `hetzner-watch-state`, which **the job creates
and updates itself - it is deliberately not in git**: Argo CD resets anything it manages to the
git version (self-heal), and would wipe the state every few minutes. Objects it did not create it
leaves alone, and removing the Application removes them with the namespace.

The job's ServiceAccount may create ConfigMaps in this namespace and read and replace exactly
`hetzner-watch-state` (`rbac.yaml`) - nothing else in the cluster.

## The token

A **read-only** Hetzner Cloud API token of the project, only for this job, so it can never change
anything: 1Password [`Hetzner | Cloud API | prod hetzner-watch`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=6w2j3saopdpmtpa447cp5gnhke&h=my.1password.com), OpenBao `secret/hetzner-watch`
(field `hcloud-token`), delivered by the ExternalSecret `hetzner-watch`. Creating it (once):
Hetzner console → project → Security → API tokens → Generate, name `prod hetzner-watch` (the
1Password title ends the same way, like the S3 keys' `prod <user>` labels), permission **Read**,
then

```shell
op item create --account my.1password.com --category 'API Credential' --title 'Hetzner | Cloud API | prod hetzner-watch' 'credential[password]=<token>'
```
```shell
jq -n --arg t "$(op item get 'Hetzner | Cloud API | prod hetzner-watch' --account my.1password.com --fields credential --reveal)" '{"hcloud-token":$t}' | bao kv put secret/hetzner-watch -
```

## Runbooks

### Run it now

```shell
kubectl --context d3strukt0r-prod-admin -n hetzner-watch create job --from=cronjob/hetzner-watch manual-$(date +%s)
```
```shell
kubectl --context d3strukt0r-prod-admin -n hetzner-watch logs job/<name of that job>
```

### See or reset the state

```shell
kubectl --context d3strukt0r-prod-admin -n hetzner-watch get configmap hetzner-watch-state -o jsonpath='{.data.state\.json}'
```

Deleting the ConfigMap makes the next run a first run again ("Hetzner watch started" with all
values).

### Watch another server type or location

Add it to `WATCH_SERVER_TYPES` (or change `WATCH_LOCATION`) in `cronjob.yaml`. The next run
reports the new keys as changes from `(not watched)`.

### Test the script locally

With a token in `HCLOUD_TOKEN`, a state file instead of the ConfigMap and the message printed
instead of sent:

```shell
docker run --rm -e HCLOUD_TOKEN -e WATCH_SERVER_TYPES=cx33,cx43 -e STATE_FILE=/tmp/state.json -e DRY_RUN=1 -v "$PWD/kubernetes/components/hetzner-watch:/app:ro" python:3.14-alpine python /app/watch.py
```
