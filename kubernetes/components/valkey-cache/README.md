# valkey-cache

The cluster's shared cache (`kubernetes/clusters/prod/valkey-cache.yaml`, plain manifests here,
namespace `valkey-cache`), reached as `valkey-cache.valkey-cache.svc:6379`. Today it holds the
WordPress sites' object cache (`charts/wordpress`, value `objectCache.database`). Valkey is the
Linux Foundation's fork of Redis (BSD); every Redis client speaks to it.

## What runs

One pod (`deployment.yaml`):

| Container | Image | Does |
|---|---|---|
| `valkey` | `valkey/valkey:9.1` | the cache, configured by `valkey.conf`; its users from the aclfile `acl-sync` builds from the Secret `users` and each app's Secret `db-<n>` |
| `exporter` | `oliver006/redis_exporter:latest` | Valkey's figures for Prometheus on port 9121 (`pod-monitor.yaml`), logged in as the user `exporter` |
| `acl-sync` | `valkey/valkey:9.1` | builds Valkey's aclfile from the users Secrets, every line checked, and loads it again when they change ([`../../shared/valkey-acl-sync/README.md`](../../shared/valkey-acl-sync/README.md)); its initContainer `acl-sync-init` writes the first one |

## Design

- **A cache, nothing more.** `maxmemory 64mb` with `allkeys-lru`: when it is full, the least
  recently used keys of any app go. Nothing is written to disk (`save ""`, `appendonly no`) and
  there is no volume, so a restart - a rollout, a reboot - starts empty and the
  apps fill it again. One app's keys can push out another's; that only costs speed. State that
  must survive (sessions, locks) belongs in valkey-state
  ([`../valkey-state/README.md`](../valkey-state/README.md)), with the same database number.
- **One pod, no operator, no replicas.** A second replica would be a second, separate cache, and
  failover would buy nothing a restart does not. While the pod is away the WordPress sites run
  without the cache, only slower (`WP_REDIS_GRACEFUL`, `charts/wordpress`). No priority class:
  under memory pressure a cache may go first.
- **A database per app, enforced by ACL.** Valkey 9.1 can restrict a user to databases
  (`resetdbs db=<n>`): selecting another one, or working before selecting its own, is refused
  with `NOPERM`. So the apps need no key prefixes, and FLUSHDB - WordPress's "Flush cache" -
  empties only the app's own database. Each app's user may run everything but `@dangerous`, plus
  FLUSHDB and INFO (Redis Object Cache sends INFO on every connect and stays disconnected
  without it); FLUSHALL, CONFIG, KEYS, MONITOR and the like stay refused. Tested with this image
  (2026-10-19): two include files, `default` locked, `NOPERM` on another database, FLUSHDB and
  EVAL in its own.
- **The default user is off.** A users file that does not name `default` leaves it on, without
  a password and allowed everything; here it is `off`, so every connection must log in.
  `protected-mode` is off for the same reason: it only matters while `default` has no password.
- **Each app brings its own user**, as its MariaDB objects come with it into `mariadb`: an
  ExternalSecret `db-<n>` in this namespace, named after its database number, rendering one
  Valkey config line (`user <app> on #<sha256 of its password> resetdbs db=<n> ...`) from
  `secret/valkey-apps/<app>`, which the app reads as well. For a WordPress site the chart writes
  it from `objectCache.database`. This component names no app: the Deployment mounts `users`
  (`default` and `exporter`, `external-secrets.yaml`) and `db-1` to `db-15`, all optional, as one
  directory. Two apps claiming the same number would both own `db-<n>`, and Argo CD reports the
  conflict instead of letting them share a database. Removing an app removes its user with it
  (its Application's finalizer).
- **Users applied while it runs, and never able to stop it.** Valkey reads only an `aclfile`
  that the sidecar `acl-sync` builds from that directory, with every line checked first, and
  loads again when a Secret changes - a new, changed or removed `db-<n>` takes effect within one
  to two minutes, without a restart (a removed one after a further five minutes' grace)
  ([`../../shared/valkey-acl-sync/README.md`](../../shared/valkey-acl-sync/README.md)). A line
  Valkey refuses stays out: that app keeps its previous user until the pod is recreated (a new
  app, or any in a new pod, gets none) and
  `ValkeyCacheUsersBroken` fires, while Valkey and every other app carry on - also at the next
  start.
- **Reloader restarts it only for the Secret `users`**
  (`secret.reloader.stakater.com/reload: users`): the exporter reads its password from it only at
  start. A restart only empties the cache.
- **Only labelled namespaces may connect** (`network-policy.yaml`): port 6379 from namespaces
  labelled `valkey.d3strukt0r.dev/cache: "true"` (the WordPress chart sets it with the object
  cache), port 9121 from `monitoring`. The ACLs would refuse anyone else anyway.
- **Probes on the port only**: with `default` off, a PING without logging in only says NOAUTH.
- **The 9.1 line, kept current by Keel** (since 2026-11-17; `../keel/README.md`): Valkey's tag
  is set once, in `kustomization.yaml` (`images:`), for Valkey and `acl-sync`'s containers alike,
  and Keel rolls all three onto a new 9.1 build together, so the lines are checked with the Valkey
  that loads them. Never a line still in release candidates - `valkey/valkey:9` and `:9.2` already
  pointed at 9.2.0-rc1 (2026-10-19); moving to 9.2 is a deliberate change once it is released. 9.1
  or newer is needed for the database ACLs. The exporter runs on `latest` (it has no `v1` tag).
- **Restricted Pod Security**: Valkey runs as the image's user 999, started directly as
  `valkey-server` (the image's entrypoint would only change the owner of `/data` as root), with a
  read-only root and an emptyDir as its working directory; the exporter as its own 59000.
- **Memory**, over 2026-10-23..11-02: Valkey 96Mi / 192Mi (`maxmemory` plus its own overhead;
  real-memory p95 14 MiB, working set peak 25 MiB, so `maxmemory` went from 192mb to 64mb on
  2026-11-02); the exporter on the namespace's defaults (real p95 22 MiB, peak 33 MiB).
- **Not an operator, not the official chart.** valkey-operator (v0.7.1, `v1alpha1`) runs only
  Valkey Cluster and calls itself not ready for production; the chart `valkey-io/valkey-helm`
  (0.12.0) leaves the default user open and probes without logging in. A few plain manifests are
  less to check.

## Databases

| Database | Taken by |
|---|---|
| 0 | nobody - where a connection lands before it selects its own |
| 1 | robines-portfolio (`objectCache.database`) |
| 2 | arepazo (`objectCache.database`) |
| 3 | a site from the private repository |

Valkey has 16 databases by default (`databases` in `valkey.conf` raises it; the Deployment mounts
`db-1` to `db-15`). Which are taken right now:
`kubectl --context d3strukt0r-prod-admin -n valkey-cache get secrets -o name | grep /db-`.

## Secrets

| OpenBao | Fields | From |
|---|---|---|
| `secret/valkey-cache` | `exporter-password` (20 characters) | 1Password [`Valkey | Prod | valkey-cache`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=2jjgal65ruf7p7vagp2aijynbq&h=my.1password.com) |
| `secret/valkey-apps/<app>` | `password` (20 characters) | 1Password `Valkey | Prod | <app>`, named in the app's own README |

## Runbooks

### Adding an app

1. A free database number from the table above.
2. Its password, logged in to OpenBao (`bao login -method=oidc -no-print` against
   `BAO_ADDR=https://openbao.d3strukt0r.dev`):
   ```shell
   op item create --account my.1password.com --vault Private --category password \
     --title 'Valkey | Prod | <app>' --generate-password='letters,digits,symbols,20' >/dev/null
   jq -n --arg p "$(op item get 'Valkey | Prod | <app>' --account my.1password.com --vault Private --fields password --reveal)" \
     'if ($p|length)==0 then error("empty value - 1Password lookup failed") else {password:$p} end' \
   | bao kv put secret/valkey-apps/<app> -
   ```
3. In the app's own component: the ExternalSecret `db-<n>` with `namespace: valkey-cache`,
   rendering `users.conf` as
   `user <app> on #{{ .password | sha256sum }} resetdbs db=<n> resetchannels ~* +@all -@dangerous +flushdb +info`
   (no quotes or backslashes: `acl-sync` splits on spaces);
   the label `valkey.d3strukt0r.dev/cache: "true"` on its namespace; and the password in the
   app's own Secret. A WordPress site gets all of it from `objectCache.database`
   (`charts/wordpress/templates/valkey-user.yaml`).
4. The number into the table above.

### After a cache outage

The WordPress sites fall back to their database while they cannot reach Valkey
(`WP_REDIS_GRACEFUL`), and what changes in that time - a price, stock after an order, a setting -
never reaches the cache. If Valkey itself kept running (a network problem, an overloaded pod,
the sites' 1 s timeout), it still holds the old values afterwards and serves them. A restart
empties it for every app at once:

```shell
kubectl --context d3strukt0r-prod-admin -n valkey-cache rollout restart deploy/valkey-cache
```

Not needed when Valkey itself restarted (a crash, a rollout, a reboot): it starts empty.
`ValkeyCacheDown` only sees Valkey itself down - the exporter sits in the same pod; a site that
cannot reach it logs the connection error in its own php-fpm log
(`{namespace="<site>", container="php-fpm"}`).

### Looking inside

As an app's user, in its own database (`--askpass` asks for the password instead of taking it
on the command line):

```shell
kubectl --context d3strukt0r-prod-admin -n valkey-cache exec -it deploy/valkey-cache -c valkey -- valkey-cli --user <app> --askpass -n <n> DBSIZE
```

Who is connected, as the exporter (`CLIENT LIST` is `@dangerous` for the apps):

```shell
kubectl --context d3strukt0r-prod-admin -n valkey-cache exec -it deploy/valkey-cache -c valkey -- valkey-cli --user exporter --askpass CLIENT LIST
```

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin -n valkey-cache get pods,externalsecrets
kubectl --context d3strukt0r-prod-admin -n valkey-cache logs deploy/valkey-cache -c valkey
kubectl --context d3strukt0r-prod-admin -n valkey-cache logs deploy/valkey-cache -c acl-sync
```

`acl-sync` logs `checked <n> users, nothing to change` after a start, `loaded <n> users` on each
change, and a `failed: ...` line naming the file and Valkey's error every 15 seconds while a line
is refused. The initContainer `acl-sync-init` logs the first aclfile.

In Prometheus: `redis_up{job="valkey-cache/valkey-cache"}`, `redis_memory_used_bytes`,
`redis_db_keys`, and the hit rate
`rate(redis_keyspace_hits_total[5m]) / (rate(redis_keyspace_hits_total[5m]) + rate(redis_keyspace_misses_total[5m]))`.
Alerts (`rules.yaml`): `ValkeyCacheDown`, `ValkeyCacheRejectedConnections`,
`ValkeyCacheUsersBroken` (an app's user line `acl-sync` keeps out - "Users applied while it
runs" in Design).
