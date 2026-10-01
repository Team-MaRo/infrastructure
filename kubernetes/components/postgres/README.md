# PostgreSQL

The shared PostgreSQL for every app that only supports it (`kubernetes/clusters/prod/postgres.yaml`,
manifests in this directory, namespace `postgres`): `Cluster` `postgres`, 2 instances on 10 GB
each, one per node, run by CloudNativePG ([`../cloudnative-pg/README.md`](../cloudnative-pg/README.md)).
Apps connect to the Service **`postgres-rw`** (`postgres-rw.postgres.svc:5432`), which follows
the primary (`postgres-ro` reads from the replica). It is backed up every night and its WAL is
archived continuously (a new file at least every 5 minutes), so it can be restored to any moment
in the last 30 days. How the cluster's manifests fit together is in
[`kubernetes/README.md`](../../README.md).

## Design

- **Settings that differ from the operator's defaults:**
  - the image pinned to one dated build (`18.6-<build>-minimal-trixie`), since the plain tag moves
    with every rebuild and the two instances could end up on different images;
  - `podAntiAffinityType: required` (the default only prefers separate nodes);
  - `primaryUpdateMethod: switchover` (the default restarts the primary in place);
  - synchronous replication with `dataDurability: preferred` - a commit waits for the replica
    while it is up, and the primary goes on alone while it is not, like MariaDB's semi-sync;
  - 256Mi/512Mi and `shared_buffers` 128MB, raised with real data.
  WAL stays on the data volume - a WAL volume per instance would make four. The initdb bootstrap
  creates CloudNativePG's default `app` database and owner, which nothing uses.
- **Apps get their users and databases declaratively**, as `DatabaseRole` (the docs' recommended
  way, over the Cluster's inline `managed.roles`) and `Database` objects in `postgres`. The role's
  password comes from a `kubernetes.io/basic-auth` Secret there, built by an ExternalSecret from
  OpenBao `secret/postgres-apps/<app>`; the app's own namespace gets the same value by a second
  ExternalSecret. No app reads CloudNativePG's generated Secrets. These objects sit in the
  app's own component (`components/<app>/database.yaml`, with `namespace: postgres`), not in
  the postgres component: everything about an app stays in one folder (user decision,
  2026-09-29). Steps in "Adding an app" below.
- **Backups** (`backups.yaml`), to `d3strukt0r-prod-postgres-backups`:
  - `ObjectStore` `postgres-backups`: WAL and base backups gzipped, `retentionPolicy: 30d`. Its
    `instanceSidecarConfiguration` sets `AWS_REQUEST_CHECKSUM_CALCULATION` and
    `AWS_RESPONSE_CHECKSUM_VALIDATION` to `when_required` (boto3's checksum headers, which
    S3-compatible stores reject - the plugin docs' workaround) and 64Mi/256Mi for the sidecar,
    whose uploads need more than the LimitRange's 128Mi. No region is set: Hetzner accepts
    requests signed for boto3's default (tested 2026-09-29).
  - `ScheduledBackup` `postgres-daily` at 23:00 UTC, from the replica (the cluster's default
    target), and once right away when created.
- **Alerts** (`rules.yaml`, from the instances' metrics through `pod-monitor.yaml` - the Cluster's
  `enablePodMonitor` is deprecated): `PostgresNoReadyPrimary` (critical, 5 minutes),
  `PostgresReplicaMissing`, `PostgresReplicaLagging`, `PostgresWALArchivingFailing` (a failure
  newer than the last success - a quiet database archives nothing for hours without that being a
  problem) and `PostgresBackupFailed` (last backup failed, or none for 26 hours).
- **`ServerSideDiff=true` on the Application** from the start, the MariaDB lesson.
- **A `DatabaseRole`'s password Secret must carry `cnpg.io/reload: "true"`.** CloudNativePG
  applies a role only when its spec or its Secret changes; the first attempt raced the operator
  adding the Secret to the instances' Role and was refused, and nothing retried it until the
  Secret changed. The label also makes password changes apply at once. The per-app
  ExternalSecret sets it through its template ("Adding an app" below).

## Failover tests (2026-09-29)

With a pod writing a row every second through `postgres-rw`:

| Test | Writes paused | Result |
|---|---|---|
| Planned switchover (`kubectl cnpg promote`) | ~10 s | no row lost |
| Primary pod deleted | ~8 s | no row lost, pod rejoined as replica |
| Primary's node powered off, with the active operator on it | ~96 s | no acknowledged row lost; 65 s until the node was NotReady, then 29 s for the failover |

The identity column jumps by up to 32 after each switch: PostgreSQL logs sequence values in
batches of 32, and a new primary continues after the logged batch - not lost rows. While the
replica is gone, `synchronous_standby_names` is empty (`dataDurability: preferred`). The
restore drill ("Restore to a point in time" below) matched row count, highest id and checksum
exactly and took 90 seconds.

## Before the first sync

The S3 key goes into OpenBao (`secret/postgres-backups-s3`) - see
[`tofu/objectstorage/README.md`](../../../tofu/objectstorage/README.md).

```shell
kubectl --context d3strukt0r-prod-admin -n postgres get cluster,pods,backups,scheduledbackups
```

The `cnpg` kubectl plugin (`brew install kubectl-cnpg`) adds `kubectl cnpg status postgres -n
postgres`, which shows the primary, replication and the archiving state in one view.

## Adding an app

1. A generated password in 1Password (`PostgreSQL | Prod | <app>`), then into OpenBao as
   `secret/postgres-apps/<app>` with a `password` field (JSON on stdin, as for MariaDB).
2. In the app's own component, `components/<app>/database.yaml` (`zitadel/` is the model),
   every object with `namespace: postgres` - CloudNativePG wants a role, its password Secret
   and a database in the Cluster's namespace:
   - an ExternalSecret building Secret `<app>-db` of type `kubernetes.io/basic-auth`
     (`username: <app>` as a literal, `password` from OpenBao) **with the label
     `cnpg.io/reload: "true"`** (through `target.template.metadata.labels`) - without it the
     role may never be created, see "Design" above;
   - a `DatabaseRole` (`cluster: postgres`, `name: <app>`, `login: true`,
     `passwordSecret: <app>-db`) and a `Database` (`cluster: postgres`, `name: <app>`,
     `owner: <app>`).

   Check both with
   `kubectl -n postgres get databaseroles.postgresql.cnpg.io,databases.postgresql.cnpg.io`
   (`APPLIED` true; `status.message` says why not).
3. In the app's own component: an ExternalSecret reading the same `secret/postgres-apps/<app>`,
   host `postgres-rw.postgres.svc`, and `sslmode=verify-full` against CloudNativePG's CA. The CA
   certificate is copied into the app's namespace by `postgres-ca.yaml` - copy Zitadel's, which
   holds a ServiceAccount, a Role in `postgres` that may `get` only Secret `postgres-ca`, a
   `SecretStore` (External Secrets' Kubernetes provider) and an ExternalSecret taking just
   `ca.crt`; rename the Role and RoleBinding after the app.

## Failover hangs

`PostgresNoReadyPrimary` means apps cannot write. A failover normally finishes on its own: 8-11
seconds for a pod, about a minute and a half for a dead node (tested 2026-09-29).

1. Look: `kubectl cnpg status postgres -n postgres` and the operator's log
   (`kubectl -n cnpg-system logs deploy/cloudnative-pg`) - which instance was primary, on which
   node, is that node `NotReady`?
2. If the old primary's node stays gone, its pod stays `Terminating` and its volume attached, so
   that instance cannot be recreated elsewhere. The failover itself does not need it; to get
   the second instance back, declare the node gone
   (`kubectl taint nodes <node> node.kubernetes.io/out-of-service=nodeshutdown:NoExecute`, as for
   MariaDB) and remove the taint once the node is back or replaced.
3. A primary can be chosen by hand: `kubectl cnpg promote postgres <instance> -n postgres`.

## Restore to a point in time

A restore creates a **new** cluster from the backups; the running one is not touched.

1. Apply a second `Cluster` in `postgres` with its own name, the same image and storage, one
   instance, and no `plugins` (so it never archives into the production path):
   ```yaml
   bootstrap:
     recovery:
       source: origin
       recoveryTarget:
         targetTime: "2026-10-01 16:00:00+00"   # UTC; omitted = as late as the archive goes
   externalClusters:
     - name: origin
       plugin:
         name: barman-cloud.cloudnative-pg.io
         parameters:
           barmanObjectName: postgres-backups
           serverName: postgres
   ```
2. Wait for `kubectl -n postgres get cluster <name>` to report "Cluster in healthy state", then
   check the data (`kubectl -n postgres exec <name>-1 -c postgres -- psql -d <db> ...`).
3. Copy what is needed back (`pg_dump` from the restored instance), or point the app at it.
   Then delete the Cluster, its PVC, the PV and the Hetzner volume by hand (Retain).

## Disaster - restore as production

**Untested** (it would mean deleting the production database). Only when the running cluster
is beyond repair - both volumes lost or corrupt; for anything less, restore next to it as above.
Apps cannot write until it is done.

The restored cluster must be called `postgres` again, so `postgres-rw` and the Application
still fit, and it must archive into a new path: a cluster that finds WAL of its own name in the
archive refuses to archive ("Expected empty archive"), which is the check protecting the old
history. Argo CD recreates whatever git describes the moment the Cluster is deleted, so the
restore goes through git:

1. In `components/postgres/cluster.yaml`, in one commit: `bootstrap.recovery` and
   `externalClusters` as in the section above (`serverName: postgres` - the old path, read
   only), and the archiving plugin's parameters get `serverName: postgres-<date>` - the new
   path. Push. Argo CD may fail to apply the change to the broken cluster; that is expected.
2. Delete the Cluster `postgres` and its PVCs (`postgres-1`, `postgres-2`) plus their PVs and
   Hetzner volumes. Argo CD recreates it from git, and it recovers from the backups.
3. Check the data, then take a base backup at once:
   `kubectl cnpg backup postgres -n postgres --method plugin --plugin-name barman-cloud.cloudnative-pg.io`.
4. The old path (`postgres/` in the bucket) is no longer pruned by anyone - it has no cluster
   archiving to it. Keep it as long as its restore points may matter, then delete it by hand.
   The `bootstrap` and `externalClusters` are only read at creation; leave them or remove them
   in a later commit.
