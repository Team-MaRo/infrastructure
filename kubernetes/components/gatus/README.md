# Gatus

The public status page at `https://status.d3strukt0r.dev`, and the watch on the alerting
pipeline itself (`kubernetes/clusters/prod/gatus.yaml`, plain manifests in this directory, image
pinned, namespace `gatus`). How the cluster's manifests fit together is in
[`kubernetes/README.md`](../../README.md).

Gatus watches the alerting pipeline itself: Alertmanager's always-firing `Watchdog` alert
arrives there as a heartbeat, and when it stops, Gatus alerts through ntfy. It replaced Uptime
Kuma (2026-09-30), which has no OIDC and whose maintainers declined adding it. Like every
monitoring component it has its own namespace, under the restricted Pod Security Standard.

## Design

- **No admin UI, no login.** Everything Gatus checks and whom it alerts is `config.yaml` in git
  (a ConfigMap without a name hash); Argo CD updates it and Gatus reloads the file by itself
  within about a minute (it polls every 30 s). So administering it is a git push, and the page
  can be public without any gate. Gatus' own OIDC option would only put a login in front of
  viewing the whole page.
- **An invalid `config.yaml` makes Gatus exit**, and the pod crash-loops - loud rather than
  silently running an old config. Secrets reach the file through `${VAR}` from Secret `gatus`
  (ntfy topic and token from `secret/ntfy`, the heartbeat token from `secret/gatus`, the
  database password); Gatus expands every `$` in the file, so a literal one is written `$$`,
  and the generated values contain no `$`.
- **History is in the shared PostgreSQL** (database `gatus`, `sslmode=verify-full` against
  CloudNativePG's CA, the ConfigMap `postgres-ca` from trust-manager), so a restart keeps uptime and response times and
  the nightly Postgres backup covers them. Postgres unreachable at start: Gatus exits and
  retries with the pod (seen once on the first deploy, before the database existed); gone
  while running: checks and ntfy alerts go on (alerting runs before the result is saved),
  only the results of that time are missing.
- **One replica, `strategy: Recreate`**: a second instance would run every check and send every
  alert twice.
- **The image is `FROM scratch` and names no user**, so the pod sets UID 65534; with Postgres as
  storage it writes nothing, so the root filesystem is read-only. About 17 MiB, within the
  namespace's LimitRange defaults, so no `resources` are set.
- **The Watchdog** is the external endpoint `alerting_watchdog`: Alertmanager POSTs to
  `/api/v1/endpoints/alerting_watchdog/external?success=true` with the 32-character token as a
  Bearer header (anything else is `401`); without one for 5 minutes, Gatus alerts. Alertmanager
  sends every 2 minutes, not every minute: it checks each `group_interval` (1m) but repeats only
  after a full `repeat_interval` (1m), which the check misses by a hair every other time.
- **It cannot report a dead cluster** while it runs inside it: a complete outage takes the page
  down too. It moves to a machine outside (the home server) later - one binary and this config.
- **Restarted by Reloader** when Secret `gatus` changes in OpenBao: Gatus reads it as environment
  variables at start ([`../reloader/README.md`](../reloader/README.md)). Its ConfigMap needs no
  restart - Gatus reloads it itself.

## Adding or changing a check

Gatus checks the services in `config.yaml` and watches the alerting pipeline itself -
Alertmanager sends its always-firing `Watchdog` alert to Gatus every two minutes, and when none
has come for 5 minutes, Gatus alerts through ntfy. There is no admin UI and no login: **adding or
changing a check is editing `config.yaml` and pushing**; Gatus reloads it within about a minute,
without a restart. A check is one entry:

```yaml
endpoints:
  - name: Zitadel
    group: Cluster
    url: https://auth.d3strukt0r.dev/debug/healthz
    interval: 1m
    conditions:
      - "[STATUS] == 200"
    alerts:
      - type: ntfy
```

An invalid file makes Gatus exit and the pod crash-loop; its last log shows why. A literal `$`
in the file is written `$$`. Check what it sees:

```shell
kubectl --context d3strukt0r-prod-admin -n gatus logs deploy/gatus --previous   # after a crash
curl -s https://status.d3strukt0r.dev/api/v1/endpoints/statuses | jq -c '.[] | {key, last: [.results[-3:][] | .success]}'
```

## Before the first sync

ntfy and two secrets of its own must be in OpenBao.

Alerts go to one ntfy.sh topic, whose name is the secret: on the free plan a topic cannot be
reserved, so anyone who knows the name could read it.

1. **ntfy**, once for all monitoring (Gatus and Alertmanager share it):
   - Create the topic name with `echo "prod-alerts-$(openssl rand -hex 16)"` - the prefix says
     what it is, the 128 random bits make it unguessable.
   - Create an access token at ntfy.sh → Account → Access tokens, named `prod cluster alerts`,
     **never expiring** (an expiring token would silently stop the alerts).
   - Store both in 1Password as [`ntfy | prod alerts`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=htj7nc5vbwet6osr3mfdm3b5yy&h=my.1password.com) (topic as `username`, token as
     `credential`), and subscribe to the topic in the ntfy Android app.
   - Copy them into OpenBao (port-forward to OpenBao open):

     ```shell
     BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN="$(op item get 'OpenBao | Prod | Recovery keys & root token' --account my.1password.com --vault Private --fields credential --reveal)" \
       bao kv put secret/ntfy \
         topic="$(op item get 'ntfy | prod alerts' --account my.1password.com --vault Private --fields username)" \
         token="$(op item get 'ntfy | prod alerts' --account my.1password.com --vault Private --fields credential --reveal)"
     ```

2. **The heartbeat token and the database password**, letters and digits only: Gatus expands
   every `$` in its config, and the password sits inside a `postgres://` URL.

   ```shell
   op item create --account my.1password.com --vault Private --category password --title 'Gatus | Prod | Watchdog token' \
     --generate-password='letters,digits,32' >/dev/null
   op item create --account my.1password.com --vault Private --category password --title 'PostgreSQL | Prod | gatus' \
     --generate-password='letters,digits,20' >/dev/null
   jq -n --arg t "$(op item get 'Gatus | Prod | Watchdog token' --account my.1password.com --vault Private --fields password --reveal)" \
     'if ($t|length)!=32 then error("watchdog token must be 32 characters - 1Password lookup failed?") else {"watchdog-token":$t} end' \
   | bao kv put secret/gatus -
   jq -n --arg p "$(op item get 'PostgreSQL | Prod | gatus' --account my.1password.com --vault Private --fields password --reveal)" \
     'if ($p|length)!=20 then error("password must be 20 characters - 1Password lookup failed?") else {"password":$p} end' \
   | bao kv put secret/postgres-apps/gatus -
   ```

   (logged in with `bao login -method=oidc -no-print`; on a fresh OpenBao, with the root token
   and port-forward as above).

On the first start the pod may restart a few times until CloudNativePG has created its
database. Until Alertmanager sends its first heartbeat, Gatus reports the Watchdog as down after
5 minutes - a free test of the alert path.
