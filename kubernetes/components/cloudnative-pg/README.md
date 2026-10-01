# CloudNativePG

The operator running the shared PostgreSQL for every app that only supports it - Zitadel
first - the counterpart of the shared MariaDB (user decision, 2026-09-29)
(`kubernetes/clusters/prod/cloudnative-pg.yaml`, both charts pinned, values in this directory):
a primary and a replica on two volumes, like MariaDB, built small while prod-03 waits for its
resize. The operator runs in `cnpg-system` (two replicas, one active), next to the Barman Cloud
plugin, which ships backups and WAL to `d3strukt0r-prod-postgres-backups`. The instance itself,
its backups and runbooks are in [`../postgres/README.md`](../postgres/README.md). How the
cluster's manifests fit together is in [`kubernetes/README.md`](../../README.md).

## Design

- **Why it needs none of MariaDB's workarounds:** PostgreSQL archives its WAL continuously and
  starts a new file at least every 5 minutes (`archive_timeout`), so no flush job; and its
  timelines chain the WAL of the old and the new primary across a failover, so no backup after
  a switch. Confirmed by the failover tests (see [`../postgres/README.md`](../postgres/README.md)):
  a restore reached through five timelines, including the
  last minutes before a hard power-off that the dead primary never archived - the synchronous
  replica had them, and PostgreSQL carries them into the new timeline's first WAL file.
- **The operator runs twice, on different nodes** (`replicaCount`, required anti-affinity,
  leader election built into the chart), for the same reason as mariadb-operator: it performs
  the failover and serves its own webhook (`failurePolicy: Fail`).
- **The Barman Cloud plugin** ships base backups and WAL to `d3strukt0r-prod-postgres-backups`.
  In-tree Barman support (`barmanObjectStore`) is deprecated and removed in CloudNativePG 1.31.
  The plugin must run in the operator's namespace, where the operator finds it by the label on
  its Service; its TLS comes from its own cert-manager Issuer. **It runs as one replica only** -
  a second never becomes ready, since the plugin serves only on its leader
  (plugin-barman-cloud#1105, unreleased). The archiving itself runs in a sidecar inside each
  PostgreSQL pod; the Deployment is needed to start new PostgreSQL pods and backups, so it
  tolerates an unreachable or not-ready node for 30 seconds instead of five minutes.
- **Archiving after a switch** (plugin-barman-cloud#828, "Expected empty archive"): reported to
  stop archiving after a switchover or failover up to 1.30.0; the operator-side fix is in 1.30.1,
  and the plugin honours it since 0.14. Not reproduced in four switches (2026-09-29) - each
  timeline's WAL reached the bucket. The only "failed" archive attempts are CloudNativePG's own
  "switchover in progress, refusing archiving", retried a second later. Should it ever appear,
  the workaround is the annotation `cnpg.io/skipEmptyWalArchiveCheck`.
- **The CRDs come with the operator chart**, marked `helm.sh/resource-policy: keep`, which Argo CD
  honours as `Delete=false` - so, unlike mariadb-operator's, they need no Application of their
  own. The chart renders 1.3 MB (`ServerSideApply=true` required). Bump both charts together.
- Both charts meet the restricted Pod Security Standard as they are; images are on `ghcr.io`.
  The operator's metrics are scraped through the chart's PodMonitor, and its Grafana dashboard
  comes as a ConfigMap. Kubernetes 1.37 is "tested, but not supported" for 1.30.x.

## Looking at it

```shell
kubectl --context d3strukt0r-prod-admin -n cnpg-system get pods,lease,certificate
```

The `cnpg` kubectl plugin (`brew install kubectl-cnpg`) adds `kubectl cnpg status postgres -n
postgres`, which shows the primary, replication and the archiving state in one view.
