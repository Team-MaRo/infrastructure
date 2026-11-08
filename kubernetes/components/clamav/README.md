# ClamAV

Back to [`kubernetes/README.md`](../../README.md).

The cluster's virus scanner: one `clamd` that uploads are handed to before they reach an app
(`kubernetes/clusters/prod/clamav.yaml`, plain manifests in this directory, namespace `clamav`).
The goal is that nobody can upload malware to any app on the cluster - WordPress forms and
wp-admin, the wedding app, apps to come - without each app setting something up: Traefik's
plugin `upload-scan` hands each uploaded file to `clamav.clamav.svc:3310`
([`traefik/README.md`](../traefik/README.md), "Upload scan"). Being rolled out, reporting only:
Traefik loads it, the dashboard's route comes next, then every site.

## Design

- **One pod, four containers** (`deployment.yaml`), each started directly instead of through
  the image's entrypoint, which runs clamd and freshclam in the background and stays up when
  clamd dies:
  - `clamd` scans, over TCP 3310 (`clamd.conf`);
  - `freshclam` fetches new signatures into the folder both share (`freshclam.conf`, 12 checks a
    day, its default) and tells clamd to reload right away (`--daemon-notify`, over localhost:3310
    - the containers of a pod share its network); without that clamd would find them only at its
    self-check, up to 10 minutes later;
  - `exporter` (`rekzi/clamav-prometheus-exporter`) reports clamd's state and the signatures'
    age to Prometheus;
  - `backup` keeps the last good signatures in the private bucket `d3strukt0r-prod-clamav`
    (below).
- **Floating tags through Keel** (`clamav/clamav:stable_base`, the exporter's `latest`;
  [`keel/README.md`](../keel/README.md)): every new release rolls the pod by itself, a new
  feature release (1.6) included - nothing to change here (user decision 2026-11-06). `stable` is
  not among the tags ClamAV's image documentation describes (it names `X.Y`, `X.Y.Z` and
  `unstable`); should it stop being updated, `ClamavSignaturesStale` does not notice, so check
  Keel's messages and the image's age now and then. `_base`
  carries no signatures - the tags that do are rebuilt every day, which would restart clamd every
  day. So an init container downloads them at every start into the shared folder (an emptyDir,
  about 110 MB, 3 s in a test); the mirrors limit downloads per address, which a pod starting a
  few times a month stays far below. **The mirror's limit is strict**: on 2026-11-06 the first
  start downloaded four times (the init container was OOM-killed at 128Mi) and the node's
  address was blocked for six hours; freshclam then exited 0 without `main.cvd`. So the init
  container checks that all three files exist before clamd starts, and freshclam has 768Mi
  (376 MiB peak for `main.cvd`). A start during such a cool-down waits in the init container
  (`ClamavDown`) until it ends - unless the bucket has a copy.
- **The last good signatures in a private bucket**, so a start never depends on ClamAV's mirror:
  `backup` uploads them as one archive (`signatures.tar`, about 115 MB) whenever they changed,
  checking every 6 hours; the init container `restore` unpacks it before freshclam runs, which
  then fetches only what changed since. When the mirror refuses this address, clamd starts with
  the copy (at most a few hours behind) and only freshness suffers (`ClamavSignaturesStale` after
  two days). The bucket stays private - S3 key `secret/clamav-s3`, its policy in
  `tofu/objectstorage`. Not a mirror of our own (ClamAV's cvdupdate): freshclam cannot sign S3
  requests, so a mirror would need the bucket public or a server in front of it, and ClamAV
  recommends one only from more than ten hosts - here is one clamd for everything (2026-11-06).
  A failed upload only shows in `backup`'s log; it matters only for a start during a block.
- **One replica** (user decision 2026-11-06): a second would cost another ~1 GiB of memory. While
  the pod restarts - a node reboot, a resize, a drain - nothing can be scanned; clamd answers
  about 10 s after it starts.
- **Settings that differ from ClamAV's defaults** (only these are in the files):
  - `TCPSocket 3310`.
  - `MaxThreads 4`, `MaxQueue 16`: scans at once (default 10) and streams accepted at once
    (default 100). Each stream lands in `/tmp` as it arrives and each scan unpacks archives there,
    and the nodes have only a few GiB of disk free (2026-11-06: 6-14 GiB). With the queue full,
    clamd accepts nothing, PING included, until a slot frees up - 16 streams, 4 at a time, of up
    to 30 s each take about 120 s, and longer while streams keep coming. So readiness gives up
    only after 18 x 10 s, or the only clamd would leave the Service and every scan would fail at
    once instead of waiting; and the liveness probe restarts clamd only after 15 minutes without
    an answer: a busy one answers again within minutes, a restart would reload the signatures,
    and one silent for longer is frozen, not busy ("When `ClamavDown` fires" below).
  - `MaxScanTime 30000`: a scan ends after 30 s (default 120), well inside the 45 s Traefik
    gives a request, so Traefik still gets the answer (`Heuristics.Limits.Exceeded.MaxScanTime`)
    and a scan nobody waits for any more holds a thread for 30 s at most.
  - `ConcurrentDatabaseReload no`: during a reload clamd keeps one copy of the signatures in
    memory instead of two. A scan arriving then waits until the reload is done - about 9 s,
    measured 2026-11-06 - instead of being refused.
  - `AlertExceedsMax yes`: a file beyond ClamAV's limits (MaxFileSize and StreamMaxLength 100 MB,
    MaxScanSize 400 MB, archive depth and file count) is reported as
    `Heuristics.Limits.Exceeded` instead of passing as clean.
  - freshclam: `DatabaseMirror database.clamav.net` (without one it does not run) and
    `TestDatabases no` (no second copy of the signatures loaded just to test an update).
- **Size limits**: clamd reads at most 100 MB of a stream (`StreamMaxLength`, its default); the
  Traefik plugin does not send it larger uploads - they pass unscanned and are logged
  `skipped:too-large`. `/tmp` (3584Mi) is clamd's spool and freshclam's downloads: 4 scans x (100
  MB stream + up to 400 MB unpacked, `MaxScanSize`), 8 streams waiting in the queue and up to 12
  still arriving from the Traefiks x 100 MB, and freshclam's files come to about 4 GiB at worst -
  four large archives unpacking at once plus twenty more uploads. The limit stays below that (user
  decision): beyond it the kubelet evicts the pod (it restarts from the bucket's signatures). That
  protects the node only if it has that much disk free: with less (2026-11-06: 6-14 GiB free,
  images taking most of the rest) the node reaches disk pressure first and the kubelet evicts by
  usage over request - an age limit for unused images (the kubelet's `imageMaximumGCAge`, not set
  yet) is what would make the room. Larger uploads would need the plugin to spool to disk,
  `StreamMaxLength` raised (at most 4 GB) and `/tmp` with it.
- **clamd has no login**, so `network-policy.yaml` admits only Prometheus (the exporter's port)
  and Traefik (3310). Traefik runs on the host network: on clamav's own node kube-router admits
  it anyway, and from another node it arrives from that node's `.0` address, so the Kyverno
  GeneratingPolicy `clamav-from-nodes` creates a NetworkPolicy `clamav-from-<node>` for every
  ingress node - the pattern and Kyverno rights of `prometheus-from-nodes`
  ([`kube-prometheus-stack/README.md`](../kube-prometheus-stack/README.md)).
- **Restricted Pod Security**: the Alpine image's user `clamav` (100:101), read-only root.
- **Memory**: about 1.04 GiB after start and after a reload (Docker, 2026-11-06); clamd's request
  1088Mi, limit 2176Mi. To be measured on the cluster over days and set by the repo's rule.

## Secrets

| OpenBao | Fields | 1Password |
|---|---|---|
| `secret/clamav-s3` | `access-key`, `secret-key` | [`Hetzner \| S3 \| prod clamav`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=b4q73ophzc3gdk36l4djg4d2eu&h=my.1password.com) |

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin -n clamav get pods,networkpolicies
kubectl --context d3strukt0r-prod-admin -n clamav logs deploy/clamav -c clamd --tail=20
kubectl --context d3strukt0r-prod-admin -n clamav logs deploy/clamav -c freshclam --tail=20
kubectl --context d3strukt0r-prod-admin -n clamav logs deploy/clamav -c restore
kubectl --context d3strukt0r-prod-admin -n clamav logs deploy/clamav -c backup --tail=5
kubectl --context d3strukt0r-prod-admin -n clamav exec deploy/clamav -c clamd -- clamdcheck.sh
```

`ClamavDown` (`rules.yaml`) fires when clamd does not answer for 5 minutes,
`ClamavSignaturesStale` when the signatures are older than two days.

### When `ClamavDown` fires

clamd is busy, frozen or not running. Uploads pass unscanned meanwhile (report mode), and a
frozen clamd is restarted by its liveness probe after 15 minutes without an answer - waiting is
fine. To tell which it is:

```shell
kubectl --context d3strukt0r-prod-admin -n clamav get pods
kubectl --context d3strukt0r-prod-admin -n clamav logs deploy/clamav -c clamd --since=15m
kubectl --context d3strukt0r-prod-admin -n clamav exec deploy/clamav -c clamd -- clamdcheck.sh
```

- **Not running** (`CrashLoopBackOff`, `Pending`, restarts climbing): the logs of `restore`
  and `clamd` say why - most often memory (`OOMKilled` in `kubectl describe pod`) or the
  signatures ("Signatures stale" below).
- **Busy**: in Grafana, `clamav_queue_length` near 16 and `clamav_threads_live` at 4 just
  before it stopped answering, and many `upload_scan` lines in Traefik's log at the same time.
  It answers again once the load drops.
- **Frozen**: running, no PONG, an empty queue before it went quiet and nothing new in its log.
  The liveness probe restarts it; to restart it sooner (scans fail until it has loaded the
  signatures again, about 10 s after the start):

```shell
kubectl --context d3strukt0r-prod-admin -n clamav rollout restart deploy/clamav
```

### Testing a scan

The EICAR test string is not kept in any file here - virus scanners on a developer's machine
flag it on checkout. Build it in the shell from two halves and send it through clamd's own
client, from the pod. The client needs `TCPAddr`, which `clamd.conf` leaves out - there it would
limit clamd to that address - so it gets a config of its own:

```shell
kubectl --context d3strukt0r-prod-admin -n clamav exec deploy/clamav -c clamd -- sh -c \
  'printf "TCPSocket 3310\nTCPAddr 127.0.0.1\n" > /tmp/client.conf;
   printf "%s%s" "X5O!P%@AP[4\PZX54(P^)7CC)7}\$" "EICAR-STANDARD-ANTIVIRUS-TEST-FILE!\$H+H*" |
     clamdscan --config-file=/tmp/client.conf --no-summary --stream -'
```

The answer is `stream: Eicar-Test-Signature FOUND` (tested 2026-11-06); a clean input says
`stream: OK`.

### Signatures stale

`ClamavSignaturesStale`: freshclam's log says why. The usual cause is the mirror refusing our
address for too many downloads (a cool-down it names in the log); it clears by itself once the
cool-down is over. Restarting the pod does not help - it would only fetch again from the same
address.
