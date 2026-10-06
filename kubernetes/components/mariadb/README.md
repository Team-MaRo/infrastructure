# MariaDB

The app database (`kubernetes/clusters/prod/mariadb.yaml`, manifests in this directory,
namespace `mariadb`): `MariaDB` `mariadb`, image pinned, 2 replicas on 10 GB each, one per
node, run by the mariadb-operator ([`../mariadb-operator/README.md`](../mariadb-operator/README.md)).
Apps connect to the Service **`mariadb-primary`** (`mariadb-primary.mariadb.svc:3306`), which
follows the primary through a failover (`mariadb-secondary` reads from the replica). It is
backed up every night and its binary logs are archived every ten minutes, so it can be restored
to any moment in the last 30 days. Its pods carry the PriorityClass `stateful-core`, so they are
evicted last when a node runs short of memory
([`../priority-classes/README.md`](../priority-classes/README.md)). How the cluster's manifests
fit together is in [`kubernetes/README.md`](../../README.md).

## Failover

- **The mitigations for #1628** (the operator's known weak spot, see
  [`../mariadb-operator/README.md`](../mariadb-operator/README.md)):
  - `semiSyncWaitPoint: AfterSync` - a commit waits until the replica has it, so a failover
    loses nothing while the replica is up.
  - `semiSyncBootAsReplica` - a returning old primary boots read-only until configured.
  - `standaloneProbes` - the replication-aware liveness probe would restart exactly the
    replica a failover needs.
  - `readinessProbe.failureThreshold: 10` - with the default 3 the replica turns unready
    before the failover starts, leaving no candidate. The value comes from the issue thread;
    the failover tests measure what it needs here.
  - `MariaDBNoReadyPrimary` (critical, 5 min) - no ready endpoint behind `mariadb-primary`,
    i.e. a failover that hangs; the runbook is "Failover hangs" below.
- **Known MariaDB bug, accepted: a replica returning under a new IP hangs the primary** (found
  and reproduced 2026-09-29). When the replica's node dies hard, its connection on the primary
  stays open - no FIN or RST ever arrives. When the replica comes back as a new pod (new IP), the
  primary ends that old connection, and `Ack_receiver::remove_slave()` waits for the semi-sync
  ack receiver thread to confirm - but that thread sits in `poll(..., -1)` on the replica
  sockets, and unlike `add_slave()`, `remove_slave()` does not wake it with `signal_listener()`
  (`sql/semisync_master_ack_receiver.cc`, unchanged on MariaDB's main branch). It wakes only when
  the kernel gives the dead connection up - with `tcp_retries2` 15, up to ~15 minutes.
  Meanwhile the new replica connection and every new login hang; the liveness probe restarts
  the primary after 30 s, sometimes followed by a failover and a replica rebuild. It heals
  itself within about a minute and loses nothing. A replica returning under its old IP does not
  hit it (its kernel resets the old connection).
  - Not fixed by `net_write_timeout` (tested at 15 s), and TCP keepalive does not apply (the
    primary's heartbeats keep the connection in retransmission, not idle).
  - `net.ipv4.tcp_retries2` would (a dead connection gone in ~12 s at 5), but it is not on
    Kubernetes' safe sysctl list: it needs `--allowed-unsafe-sysctls` on every kubelet and the
    `mariadb` namespace out of the restricted Pod Security Standard - too much for this.
  - Turning semi-sync off would avoid it, but then a failover can lose the last transactions;
    semi-sync stays (user decision: a replica must always hold every acknowledged commit).
  - Reproduction, for re-testing a MariaDB upgrade: on the replica's node,
    `iptables -t raw -I PREROUTING 1 -s <pod ip> -j DROP` and the same with `-d` (the `raw`
    table, since k3s's kube-router accepts established traffic before the filter chains), write
    once on the primary, wait a few minutes, force-delete the replica pod, and watch new logins
    on the primary; remove both rules afterwards. Reported upstream as
    [MDEV-41349](https://jira.mariadb.org/browse/MDEV-41349) (2026-09-29); once a fixed release
    is out, the bump is the fix - re-test with the reproduction.

## Pod Security

- **Restricted Pod Security** needs securityContexts on the pod (999, the image's mysql user -
  setting `podSecurityContext` replaces the operator's default, so it is repeated), the
  container, the agent, the init container and the exporter (UID 65534, the image only names
  its user). Once the init container's block is set at all, its `image` is required: the
  operator's own image, **bumped together with the operator**. Custom init or sidecar
  containers cannot pass restricted (mariadb-operator#1835) - none are used.

## Backups and restore

- **Backups** (`backups.yaml`), to `d3strukt0r-prod-mariadb-backups`:
  - `PhysicalBackup` `mariadb-daily` at 23:30 UTC from the replica, kept 30 days. Staged on
    the node's disk before the upload, so it sets an ephemeral-storage limit of its own.
  - `PointInTimeRecovery` `pitr`: the agent next to the primary archives **closed** binary logs
    every ten minutes (hardcoded, counted from the agent's start, not the clock). The operator
    rotates only by size, so at low traffic the active file would stay unarchived for weeks; the CronJob `mariadb-flush-binlogs` runs
    `FLUSH BINARY LOGS` on the primary every ten minutes, so a restore loses at most about
    twenty. The bucket expires archived logs after 35 days; the server itself deletes its local
    copies after 7 (`binlog_expire_logs_seconds`, default 0 = never, which would fill the
    volume).
  - `syncBinlog: 1`: every commit's binary log event goes to disk before the commit returns.
    With the server's default 0, the failover test's hard power-off (2026-09-28) lost the end of
    the binary log, and crash recovery rolled back a commit the client had already been told
    succeeded: the primary came back without that row while the replica kept it, so the two
    silently differed. The same power-off left the file with a zeroed tail; the archiver stops
    at such a file and every later one waits (runbook "Binary log archiving stuck" below).
  - `mariadb-replica-recovery`: a never-scheduled template the operator uses to rebuild a
    replica whose replication stays broken.
  - The CronJob `mariadb-backup-after-switchover` starts `mariadb-daily` on demand after every
    switch of the primary (see the next-but-one bullet). Every 15 minutes it compares the
    `PrimarySwitched` condition's time with the backup's `lastScheduleTime`; its Role may only
    read that MariaDB and patch that PhysicalBackup. Image `docker.io/alpine/kubectl`, pinned
    like the others - kubectl alone has no shell for the comparison.
  - Alerts: `MariaDBBinlogArchivingFailing` (`BinlogsArchived` not True for 30 minutes) and
    `MariaDBBackupFailed` (`mariadb-daily`'s last run did not complete, or none has for 26
    hours - also a failed after-switchover backup, which the CronJob does not retry), from
    kube-state-metrics' custom resource metrics (see
    [`../kube-prometheus-stack/README.md`](../kube-prometheus-stack/README.md)). A failed Job's
    `Complete=False` makes the `status="True"` series vanish, so an age check alone would miss
    exactly a failure.
- **A restore always goes into a new MariaDB** (`bootstrapFrom.pointInTimeRecoveryRef` with a
  `targetRecoveryTime`), never in place. Its success condition is `BinlogsReplayed=True`.
  `targetRecoveryTime` cannot be changed afterwards: retrying with another time means deleting
  the restore MariaDB, its PVCs and PVs, and the Hetzner volumes they leave behind (`Retain`).
  `tls.caSecretKeyRef` must be set on every S3 reference - without it the restore panics in
  the operator yet reports Ready with only the base backup (mariadb-operator#1915). `s3-ca`
  holds the ISRG roots Hetzner's certificate chains to; they are public and live in git.
  Tested 2026-09-28: restored to the millisecond (runbook "Restore to a point in time" below).
- **`strictMode: true` stays**, although an operator bug makes it refuse a target inside the
  newest archived binary log: it fails loudly rather than restoring less than asked, and
  "as late as possible" still works by giving `LAST RECOVERABLE TIME` exactly (the end of
  the newest file is accepted). Without strict mode a restore that falls short reports
  success anyway.
- **Every switch of the primary leaves a hole in the archive until the next physical
  backup**: a failover, but also the planned switch of an update or a node drain. The old
  primary's last binary log (up to ten minutes) never reaches the bucket (only the primary
  archives), the new primary does not log the transactions it replicated, and the restore timeline
  follows the server the backup came from - so `LAST RECOVERABLE TIME` stays at the old
  primary's last upload. No data is lost, only restorability past that point. Seen in the
  failover test, 2026-09-28; closed within about 15 minutes by the after-switchover backup.
  `LAST RECOVERABLE TIME` only moves once something writes after that backup, so its age
  alone says nothing on a quiet database - which is why no alert watches it.
- **PITR works only in this replication topology**, not with Galera or a standalone instance.

## Passwords and Argo CD

- **The root password is kept twice**: in Secret `mariadb` (from OpenBao) and in the
  operator's own `internal-mariadb`, which it compares against to rotate a changed password.
  An empty password synced first (a failed 1Password lookup, 2026-09-28) left
  `internal-mariadb` empty while MariaDB initialised with the real one, and the operator
  looped on "Access denied" until `internal-mariadb` was patched to match - the fix is under
  "Before the first sync" below. MariaDB itself refuses to initialise without a password, so no
  empty root was set.
- **`ServerSideDiff=true` on the Application**: the client-side diff kept the MariaDB
  OutOfSync after every successful sync, although a server-side apply changes nothing in it.

## Before the first sync

Its passwords and S3 key go into OpenBao (port-forward and `BAO_TOKEN` as in
[OpenBao](../openbao/README.md)):

1. A 1Password item [`MariaDB | Prod | Root & replication`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=gsadufiuptw2el3nhww22ub22m&h=my.1password.com), a Login
   (username `root`, URL `https://phpmyadmin.d3strukt0r.dev/`) whose generated password is the
   root password, plus a second generated password field `repl-password`.
2. As JSON on stdin (generated passwords may start with `@`; an empty lookup must not be
   stored - see "Putting secret values in" in
   [`tofu/openbao/README.md`](../../../tofu/openbao/README.md)):

   ```shell
   jq -n --arg r "$(op item get 'MariaDB | Prod | Root & replication' --account my.1password.com --vault Private --fields password --reveal)" \
         --arg p "$(op item get 'MariaDB | Prod | Root & replication' --account my.1password.com --vault Private --fields repl-password --reveal)" \
     'if ($r|length)==0 or ($p|length)==0 then error("empty value - 1Password lookup failed") else {"root-password":$r,"repl-password":$p} end' \
   | BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN="$(op item get 'OpenBao | Prod | Recovery keys & root token' --account my.1password.com --vault Private --fields credential --reveal)" \
     bao kv put secret/mariadb -
   ```

   **If an empty or wrong root password reached the cluster first**, the operator keeps its
   own copy in Secret `internal-mariadb` and loops on "Access denied" trying to rotate it
   from there. Copy the real value over:
   `kubectl --context d3strukt0r-prod-admin -n mariadb patch secret internal-mariadb --type=json -p "[{\"op\":\"replace\",\"path\":\"/data/root-password\",\"value\":\"$(kubectl --context d3strukt0r-prod-admin -n mariadb get secret mariadb -o jsonpath='{.data.root-password}')\"}]"`
3. The S3 key (`secret/mariadb-backups-s3`) - see
   [`tofu/objectstorage/README.md`](../../../tofu/objectstorage/README.md).

Changing a password in OpenBao later does not change it in MariaDB.

```shell
kubectl --context d3strukt0r-prod-admin -n mariadb get mariadb,pods,physicalbackup,pitr
kubectl --context d3strukt0r-prod-admin -n mariadb get mariadb mariadb -o jsonpath='{.status.replication}' | jq
```

## Adding an app

1. A generated password in 1Password (`MariaDB | Prod | <app>`), then into OpenBao as
   `secret/mariadb-apps/<app>` with a `password` field (JSON on stdin, as for the root
   password).
2. In the app's own component, `components/<app>/database.yaml` (`phpmyadmin/` is the model),
   every object with `namespace: mariadb` and `mariaDbRef: {name: mariadb}` - mariadb-operator
   wants a user, its password Secret, a grant and a database in the MariaDB's namespace:
   - an ExternalSecret building Secret `<app>-db` with the `password` from OpenBao;
   - a `Database` `<app>` with `characterSet: utf8mb4` and `collate: utf8mb4_uca1400_ai_ci`
     (the server's default; the operator's own default is the old three-byte `utf8`), a `User`
     `<app>` (host `%`, `passwordSecretKeyRef` to `<app>-db`, `require.ssl: true`) and a
     `Grant` of what the app needs on `<app>.*`. The operator only creates a database, and its
     webhook refuses any change to `characterSet` or `collate` on an existing `Database` object
     (immutable) - Argo CD then fails the sync and, after five tries, stops retrying that commit.
     To change them later: `ALTER DATABASE` by hand (existing tables keep theirs), change the
     object in git, and delete the live object
     (`kubectl --context d3strukt0r-prod-admin -n mariadb delete database.k8s.mariadb.com <app>`)
     so Argo CD recreates it - with `cleanupPolicy: Skip` the database itself stays. If Argo CD
     has already given up on the commit, the recreation waits for the next push or a manual
     sync of the app.
   - **`cleanupPolicy: Skip` on all three.** The operator's default is `Delete`: removing the
     component would drop the database with it.

   Check them with
   `kubectl --context d3strukt0r-prod-admin -n mariadb get databases.k8s.mariadb.com,users.k8s.mariadb.com,grants.k8s.mariadb.com`
   (`READY` true; `status.conditions` says why not).
3. In the app's own component: an ExternalSecret reading the same `secret/mariadb-apps/<app>`,
   host `mariadb-primary.mariadb.svc` (it follows a failover), port 3306, and TLS verified
   against the operator's CA. The CA comes from trust-manager: the label
   `trust.d3strukt0r.dev/mariadb-ca: "true"` on the app's `namespace.yaml` puts it there as
   ConfigMap `mariadb-ca` (key `ca.crt`), which the app mounts as a directory
   ([`../trust-manager/README.md`](../trust-manager/README.md)).

To look into the databases by hand, phpMyAdmin is at `https://phpmyadmin.d3strukt0r.dev`
([`../phpmyadmin/README.md`](../phpmyadmin/README.md)).

## Queries pile up

`MariaDBQueriesPilingUp` fires when more than 20 queries run at once for a minute (usually 2-5).
That is what happened on **2026-10-06 at 08:52**: the primary (`mariadb-1`) stopped answering,
running queries climbed to 47, the operator's readiness check timed out after 90 s and it
switched over to `mariadb-0` (read-locking the old primary for three minutes meanwhile - the
WordPress apps' probes and wp-cron runs failed), then rebuilt `mariadb-1` as a replica. Not the
semi-sync bug above (the replica had been up for days, no semi-sync waits). Which query or lock
jammed it was not recorded - since then the slow log is on (`myCnf`: over 2 s, to stderr).

1. While it lasts, see what runs and what waits on the primary:

   ```shell
   P=$(kubectl --context d3strukt0r-prod-admin -n mariadb get mariadb mariadb -o jsonpath='{.status.currentPrimary}')
   kubectl --context d3strukt0r-prod-admin -n mariadb exec "$P" -c mariadb -- sh -c 'mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" -e "SHOW FULL PROCESSLIST"'
   ```

2. Afterwards, the slow queries in Grafana's Explore (Loki):
   `{namespace="mariadb", container="mariadb"} |= "Query_time"` - each entry is the
   `# Query_time` line, followed by the statement in the next lines of the same stream.
3. If a failover followed, check that the old primary rejoined as a replica (`kubectl get mariadb`
   shows both ready) and that the after-switchover backup ran.

## Failover hangs

`MariaDBNoReadyPrimary` means apps cannot write. Most failovers finish on their own within a
couple of minutes; the known case that does not is a primary whose node died hard
(mariadb-operator#1628) - the operator waits for the old primary until its node returns.

A primary that restarts once, about a minute, right after the **replica's** node came back from
a hard failure is a known MariaDB bug ("Known MariaDB bug" under "Failover" above); it heals
itself, nothing to do.

1. Look: `kubectl --context d3strukt0r-prod-admin -n mariadb get mariadb mariadb` (status
   column) and `get pods -o wide` - which pod was primary, on which node, is that node
   `NotReady`?
2. If the node comes back soon (a reboot), wait: the failover completes, or the old primary
   simply returns.
3. If the node is gone for good, tell Kubernetes so - its pods are then deleted and their
   volumes released:
   ```shell
   kubectl --context d3strukt0r-prod-admin taint nodes <node> node.kubernetes.io/out-of-service=nodeshutdown:NoExecute
   ```
4. If the operator still does not promote the replica, name it primary yourself (`0` or `1`,
   the replica's pod number):
   ```shell
   kubectl --context d3strukt0r-prod-admin -n mariadb patch mariadb mariadb --type=merge -p '{"spec":{"replication":{"primary":{"podIndex":1}}}}'
   ```
   Argo CD does not undo this - the MariaDB in git sets no `podIndex`.
5. Remove the taint once the node is back or replaced:
   `kubectl --context d3strukt0r-prod-admin taint nodes <node> node.kubernetes.io/out-of-service-`.

**After every switch of the primary** point-in-time recovery cannot go past the old primary's
last upload until a backup from the new primary exists. The CronJob
`mariadb-backup-after-switchover` starts one within 15 minutes; its log says what it decided
(`kubectl --context d3strukt0r-prod-admin -n mariadb logs job/<latest job>`). To start one by
hand - also when `MariaDBBackupFailed` fires:

```shell
kubectl --context d3strukt0r-prod-admin -n mariadb patch physicalbackup mariadb-daily --type=merge -p "{\"spec\":{\"schedule\":{\"onDemand\":\"$(date +%s)\"}}}"
```

## Binary log archiving stuck

`MariaDBBinlogArchivingFailing` fires, the MariaDB reports Ready=False with `Error archiving
binlogs: ... error getting binary log mariadb-bin.NNNNNN metadata`, and `kubectl get pitr`
shows no or an old `LAST RECOVERABLE TIME`. The archiver uploads the files in order and stops
at one it cannot read - after a hard crash of the primary, the file that was being written.
Writes are not affected, only point-in-time recovery.

1. Confirm which file, and that later ones exist:
   ```shell
   kubectl --context d3strukt0r-prod-admin -n mariadb logs <primary-pod> -c agent --tail=20
   kubectl --context d3strukt0r-prod-admin -n mariadb exec <primary-pod> -c mariadb -- bash -c 'mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" -e "SHOW BINARY LOGS"'
   ```
2. Remove the unreadable file from the server's list - `PURGE ... TO` removes every file
   **before** the one named, so name the one after it:
   ```shell
   kubectl --context d3strukt0r-prod-admin -n mariadb exec <primary-pod> -c mariadb -- bash -c 'mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" -e "PURGE BINARY LOGS TO '"'"'mariadb-bin.NNNNNN+1'"'"'"'
   ```
3. That leaves a gap no restore can cross, so take a fresh physical backup at once - it is
   the new starting point for everything after the gap:
   ```shell
   kubectl --context d3strukt0r-prod-admin -n mariadb patch physicalbackup mariadb-daily --type=merge -p "{\"spec\":{\"schedule\":{\"onDemand\":\"$(date +%s)\"}}}"
   ```
4. Within ten minutes the agent archives the remaining files; `kubectl get pitr` shows a
   `LAST RECOVERABLE TIME` again and the MariaDB turns Ready.

## Restore to a point in time

A restore creates a **new** MariaDB from the backups; the running one is not touched.

1. Pick the target time (UTC). `strictMode` refuses any time the archive cannot reach, and
   an operator bug also refuses one that falls inside the newest archived binary log:
   - **As late as possible**: copy `LAST RECOVERABLE TIME` from `kubectl get pitr`
     exactly - the end of the newest file is accepted. Leaving the time out means "now",
     which is always refused.
   - **A moment before an accident**: if it is refused as "timeline did not reach target
     time" although it lies before `LAST RECOVERABLE TIME`, it is inside the newest file;
     wait for the next upload (at most twenty minutes) and retry.
2. Apply a second MariaDB - a copy of `components/mariadb/mariadb.yaml` with another name
   (e.g. `mariadb-restore`), without `pointInTimeRecoveryRef` and without
   `replication.replica.recovery`/`bootstrapFrom` (both point at the original's backups),
   and with:
   ```yaml
   bootstrapFrom:
     pointInTimeRecoveryRef:
       name: pitr
     targetRecoveryTime: 2026-10-01T18:00:00Z
   ```
   Keep `replicas: 2` - the webhook refuses replication with one.
3. Wait for it to be ready, and **check the condition `BinlogsReplayed`**:
   `kubectl --context d3strukt0r-prod-admin -n mariadb get mariadb mariadb-restore -o jsonpath='{.status.conditions}' | jq`.
   Ready without it means only the nightly backup was restored.
4. Copy what is needed back (`mariadb-dump` from the restored instance), or switch the app to
   it. Then delete the restored MariaDB, and its PVCs and Hetzner volumes by hand (Retain).

**The target time cannot be changed** once applied. A failed restore is retried by deleting
the MariaDB, its PVCs (`storage-mariadb-restore-0`/`-1`), their PVs and the Hetzner volumes
(`hcloud volume delete`, once they show no server) - a new MariaDB on the old volumes finds
data there and skips the restore.

A drill on 2026-09-28 restored the nightly backup plus ten minutes of binary logs in under
two minutes, to the millisecond.

## Disaster - restore as production

**Untested** (it would mean deleting the production database). Only when the running MariaDB is
beyond repair - both volumes lost or corrupt; for anything less, restore next to it as above.
Apps cannot write until it is done.

The restored instance must be called `mariadb` again, so the apps' Service names and the
Application still fit. Argo CD recreates whatever git describes the moment the MariaDB is
deleted, so the restore goes through git:

1. Pick the target time as above.
2. In `components/mariadb/`, in one commit:
   - `mariadb.yaml`: add `bootstrapFrom` (`pointInTimeRecoveryRef: pitr`, `targetRecoveryTime`)
     and point `pointInTimeRecoveryRef` at a new `PointInTimeRecovery`;
   - `backups.yaml`: that new object, a copy of `pitr` named e.g. `pitr-<date>` with prefix
     `pitr-<date>`. The restored servers get the same server ids as the old ones, and their
     binary logs must not mix with the old history in `pitr`, which stays readable (for older
     restore points) until the bucket expires it after 35 days.

   Push. Argo CD may fail to apply `bootstrapFrom` to the broken instance; that is expected.
3. Delete the broken MariaDB and its PVCs (`storage-mariadb-0`/`-1`) plus their PVs and Hetzner
   volumes. Argo CD recreates it from git, and it bootstraps from the backups.
4. Check `BinlogsReplayed` as above, then take a physical backup at once (the command under
   "Failover hangs").
5. `bootstrapFrom` is only read at creation; leave it or remove it in a later commit.
