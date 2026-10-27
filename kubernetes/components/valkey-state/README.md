# valkey-state

The cluster's shared state (`kubernetes/clusters/prod/valkey-state.yaml`, plain manifests here,
namespace `valkey-state`), reached as `valkey-state.valkey-state.svc:6379`: sessions, locks, rate
limits and logins of the apps that keep them in Valkey. Its sibling
[`valkey-cache`](../valkey-cache/README.md) holds what may be lost; this one holds what must not.
Valkey is the Linux Foundation's fork of Redis (BSD); every Redis client speaks to it.

## What runs

One pod (`statefulset.yaml`, `valkey-state-0`) with a 10 GiB Hetzner Volume:

| Container | Image | Does |
|---|---|---|
| `valkey` | `valkey/valkey:9.1.2` | the state, configured by `valkey.conf`, its own users from the Secret `users`, each app's from the app's Secret `db-<n>` |
| `exporter` | `oliver006/redis_exporter:v1.94.0` | Valkey's figures for Prometheus on port 9121 (`pod-monitor.yaml`), logged in as the user `exporter` |

## Design

Everything valkey-cache's README says about **a database per app enforced by ACL**, **the
default user off**, **each app bringing its own user**, **restarts on new users** (Reloader),
**labelled namespaces only**, **TCP probes**, **pinned images** and **restricted Pod Security**
holds here as well, with `valkey-state` for `valkey-cache` and the namespace label
`valkey.d3strukt0r.dev/state: "true"`. What differs:

- **Every write is on disk before Valkey confirms it**: the append-only file with
  `appendfsync always`, no RDB snapshots (`save ""`). A crash or a killed pod loses nothing it
  confirmed - `everysec` could lose the last second, and with it a lock a second process would
  then take while the first still works.
- **Nothing is evicted** (`maxmemory 128mb`, `noeviction`): when it is full, writes fail for every
  app, hence `ValkeyStateMemoryHigh` at 80 %. Every key an app writes should carry a TTL.
- **One instance on a volume, no replica or Sentinel.** Replication is asynchronous, so a
  failover could lose locks again; a single instance whose writes are on its volume is the safe
  form. A lost node costs about 6 minutes until the volume is detached and attached elsewhere
  ("A node is lost"). `stateful-core` priority: evicted last (`components/priority-classes`).
- **No backup.** What it holds can be recreated: sessions mean logging in again, locks expire,
  zot's API keys are made again ("zot lost its keys"). Hetzner keeps no backups of volumes;
  the volume is `Retain` (`components/hcloud-csi`).
- **A StatefulSet** for the volume (`volumeClaimTemplates`, PVC `data-valkey-state-0`), with
  `fsGroup: 999` so the image's user can write to it. Its Application has **no finalizer**:
  deleting the file must not take the data along.

### What `appendfsync always` costs

Measured 2026-10-18 (Valkey 9.1, prod-01, `valkey-benchmark -t set -n 20000 -d 256`, c = parallel
clients):

| Storage | Persistence | c=1 writes/s | c=1 p50 / p99 | c=10 writes/s | c=10 p50 / p99 |
|---|---|---|---|---|---|
| Hetzner volume | none | 17,575 | 0.05 / 0.18 ms | 51,414 | 0.13 / 0.39 ms |
| Hetzner volume | `everysec` | 14,316 | 0.06 / 0.20 ms | 81,633 | 0.09 / 0.30 ms |
| Hetzner volume | `always` | 111 | 8.5 / 16.8 ms (max 221 ms) | 559 | 17.2 / 29.2 ms |
| Local disk (emptyDir) | none | 15,601 | 0.05 / 0.24 ms | 68,729 | 0.09 / 0.42 ms |
| Local disk (emptyDir) | `everysec` | 14,631 | 0.06 / 0.20 ms | 70,671 | 0.10 / 0.34 ms |
| Local disk (emptyDir) | `always` | 640 | 1.4 / 3.7 ms | 3,077 | 3.0 / 8.5 ms |

`always` waits about 9 ms per write for the volume's fsync; several writes share one fsync, so
the ceiling is about 560 writes/s - around a hundred times the few writes per second expected.
Reads are not affected. Should an app with many fast, session-heavy requests ever need more, the
way out is a split: sessions here with `everysec`, locks in a Valkey without a volume that only
reports ready after the longest lock TTL of every app.

## Databases

The same number per app as in valkey-cache, so an app has one number for both:

| Database | Taken by |
|---|---|
| 0 | nobody - where a connection lands before it selects its own |
| 1-3 | kept for the WordPress sites' numbers in valkey-cache |

Which are taken right now:
`kubectl --context d3strukt0r-prod-admin -n valkey-state get secrets -o name | grep /db-`.

## Secrets

| OpenBao | Fields | From |
|---|---|---|
| `secret/valkey-state` | `exporter-password` (20 characters) | 1Password [`Valkey \| Prod \| valkey-state`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=scdbf2irlstgsxy4apbn4ggati&h=my.1password.com) |
| `secret/valkey-apps/<app>` | `password` (20 characters) | 1Password `Valkey \| Prod \| <app>`, the same as for valkey-cache |

## Runbooks

### Adding an app

As in valkey-cache's "Adding an app", with these differences: the ExternalSecret `db-<n>` goes
to `namespace: valkey-state`, the namespace label is `valkey.d3strukt0r.dev/state: "true"`, the
number is the app's number in valkey-cache if it has one, and it goes into the table above. An
app in both instances uses one password (`secret/valkey-apps/<app>`).

### A node is lost

The pod's volume is attached to the lost node. Kubernetes moves the pod only once the node is
marked gone and the volume force-detached - about 6 minutes; until then `ValkeyStateDown`
fires. Nothing to do unless it takes longer:

```shell
kubectl --context d3strukt0r-prod-admin -n valkey-state get pods,pvc -o wide
kubectl --context d3strukt0r-prod-admin -n valkey-state describe pod valkey-state-0
```

### zot lost its keys

If the volume or its data is ever lost, zot's users and API keys go with it: the nodes can no
longer pull from zot until a new key for the user `cluster` is made and handed to them
(`components/zot`).

### Resizing the volume

`hcloud-volumes` can grow a volume in place: raise `storage` in the PVC (not the template -
a StatefulSet's `volumeClaimTemplates` cannot change), then the template to match:

```shell
kubectl --context d3strukt0r-prod-admin -n valkey-state patch pvc data-valkey-state-0 -p '{"spec":{"resources":{"requests":{"storage":"20Gi"}}}}'
```

The template change needs the StatefulSet recreated with `--cascade=orphan` (the pod and volume
stay); Argo CD then recreates it from git.

### Looking inside

As an app's user, in its own database (`--askpass` asks for the password instead of taking it
on the command line):

```shell
kubectl --context d3strukt0r-prod-admin -n valkey-state exec -it valkey-state-0 -c valkey -- valkey-cli --user <app> --askpass -n <n> DBSIZE
```

The append-only file's state and who is connected, as the exporter:

```shell
kubectl --context d3strukt0r-prod-admin -n valkey-state exec -it valkey-state-0 -c valkey -- valkey-cli --user exporter --askpass INFO persistence
kubectl --context d3strukt0r-prod-admin -n valkey-state exec -it valkey-state-0 -c valkey -- valkey-cli --user exporter --askpass CLIENT LIST
```

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin -n valkey-state get pods,pvc,externalsecrets
kubectl --context d3strukt0r-prod-admin -n valkey-state logs valkey-state-0 -c valkey
```

In Prometheus: `redis_up{job="valkey-state/valkey-state"}`, `redis_memory_used_bytes` against
`redis_memory_max_bytes`, `redis_aof_last_write_status`, `redis_db_keys`. Alerts (`rules.yaml`):
`ValkeyStateDown`, `ValkeyStateMemoryHigh`, `ValkeyStateAofWriteFailing`,
`ValkeyStateRejectedConnections`.
