# Loki

Log storage (`kubernetes/clusters/prod/loki.yaml`, chart `grafana-community/loki` pinned - the
chart moved out of `grafana/helm-charts` - values and extra manifests in this directory). How the
cluster's manifests fit together is in [`kubernetes/README.md`](../../README.md).

Alloy ([`../alloy/README.md`](../alloy/README.md)) runs on every node and sends that node's
container logs (`/var/log/pods`) and whole journal (k3s, kernel, apt, sshd), plus the cluster's
Kubernetes events, to Loki, which keeps them 30 days in `d3strukt0r-prod-loki`. Grafana has
Loki as a datasource (`additionalDataSources` in the kube-prometheus-stack values).

## Design

- **Loki is one monolithic instance** in namespace `loki`, an ordinary namespace like
  `monitoring`. The chart defaults `write`/`read`/`backend` to 3 replicas each and refuses them
  next to `singleBinary.replicas: 1`, so they are set to 0. Gateway, canary, Helm test and the
  memcached caches are off - `chunksCache` alone would reserve several GB. Single tenant
  (`auth_enabled: false`).
- **The Thanos object-store client** (`use_thanos_objstore: true`), built on minio-go - the
  client k3s's etcd snapshots already upload to Hetzner with. Loki's default aws-sdk-go-v2
  client sends checksum headers that S3-compatible stores have rejected. If Hetzner ever
  refuses it, the fallback is the default client with `AWS_REQUEST_CHECKSUM_CALCULATION` and
  `AWS_RESPONSE_CHECKSUM_VALIDATION` set to `when_required`.
- **Its key** is its own (console label `prod loki`, OpenBao `secret/loki-s3`), delivered by
  the ExternalSecret `loki-s3` as environment variables that `-config.expand-env` fills into
  the config. The bucket policy admits only it and the admin key.
- **Retention is the compactor's** (`retention_period: 720h`, `delete_request_store: s3`);
  the bucket's lifecycle rule only clears uploads that never completed.
- **Its chart's RBAC is not used.** Loki's rules sidecar would bring a ClusterRole reading every
  Secret; with the ruler (log-based alerting, not used yet) and sidecar off and
  `rbac.namespaced`, no RBAC is rendered.
- **Labels**: `namespace`, `pod`, `container`, `node`, `app` for containers; `job="node-journal"`,
  `unit`, `node` for the journal; `job="loki.source.kubernetes_events"` for events. Anything
  else is searched in the line, not labelled - Loki stays fast with few labels.
- **Restarted by Reloader** when Secret `loki-s3` changes in OpenBao (environment variables read at
  start; `singleBinary.annotations`, [`../reloader/README.md`](../reloader/README.md)).

## Searching logs

Every container's log, every node's journal and the cluster's events end up in Loki for 30
days, searchable in Grafana → Explore → datasource **Loki**. A few queries to start from:

```
{namespace="gatus"}                                        # one namespace's containers
{namespace="monitoring", container="prometheus"} |= "error"  # lines containing "error"
{job="node-journal", unit="k3s.service", node="prod-01"}   # k3s's own log on one node
{job="node-journal", unit="ssh.service"}                   # SSH logins
{job="loki.source.kubernetes_events"}                      # events: scheduling, OOM kills, pulls
```

Logs of pods that no longer exist stay searchable - `kubectl logs` only reaches running ones.

## If Loki cannot write to Object Storage

New logs still arrive but pile up on its volume, and its log says so:

```shell
kubectl --context d3strukt0r-prod-admin -n loki logs loki-0 | grep -i -E 'error|denied|failed to flush'
```

A `403` right after the key was created is Hetzner's propagation delay; anything persistent
is the key in OpenBao `secret/loki-s3` or the bucket policy in `tofu/objectstorage/loki.tf`.
