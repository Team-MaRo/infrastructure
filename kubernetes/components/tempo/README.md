# Tempo

Trace storage for the cluster (`kubernetes/clusters/prod/tempo.yaml`, the `tempo` chart of
grafana-community pinned there, values in this directory, namespace `tempo`). Spans arrive over
OTLP from Alloy, which receives them on every node (`components/alloy`); Grafana reads them
(datasource `Tempo`, `components/kube-prometheus-stack`). How the cluster's manifests fit together
is in [`kubernetes/README.md`](../../README.md).

## Design

- **One single-binary Tempo** (chart 3.2.x, Tempo 3.1): distributor, live-store, querier and
  backend worker in one pod, no Kafka - Tempo 3's distributed mode needs one, the single binary
  does not. The chart moved from grafana/helm-charts to grafana-community.
- **Traces in Object Storage**, bucket `d3strukt0r-prod-tempo` (`tofu/objectstorage/tempo.tf`),
  with Tempo's own key - the bucket policy admits only that key and the admin key. The key comes
  from OpenBao `secret/tempo-s3` through the ExternalSecret `tempo-s3` as environment variables
  that the config expands (`-config.expand-env`); Reloader restarts Tempo when it changes.
- **30 days**, as Loki's logs, so a trace link in a log line stays valid as long as the line
  (`tempo.retention: 720h`; the chart renders it into the backend worker's
  `compaction.block_retention`, its default is 24 h). The bucket is unversioned and its lifecycle
  rule only clears unfinished uploads - Tempo's compactor deletes old blocks itself. A block
  goes once its newest span is 30 days old; with a day per block ("Memory" below) its oldest
  spans stay up to a day longer.
- **No volume**: the WAL and the blocks not yet in S3 live on an emptyDir at `/var/tempo` - a few
  MB, cleared once uploaded. A deleted pod loses at most the last seconds of spans, which
  tracing can afford, and there is no live-store PVC to go wrong (grafana/tempo#8004). The root
  filesystem is read-only. The emptyDir's 1 GiB cap matches Kyverno's ephemeral-storage limit,
  which counts it and would evict the pod at 1 GiB anyway: if S3 is unreachable long enough for
  blocks to pile up that far, the pod restarts empty instead of filling the node's disk.
- **Receivers**: OTLP gRPC 4317 and HTTP 4318 - only Alloy sends. The chart's Jaeger receivers
  stay because its Service template needs them; nothing sends to them, and like every port here
  they are reachable inside the cluster only.
- **What a trace holds**: every request through Traefik and every Zitadel request, with service,
  route, path, status and timings - query parameter names but not their values, and no visitor
  address or user agent. Alloy also removes, from any sender, the text of database queries and of
  errors (operation, table and error type stay), who the user is and e-mail addresses
  ([`../alloy/README.md`](../alloy/README.md)). Only
  the admin reads them, in Grafana; they serve finding faults and slow requests, and go after 30
  to 31 days.
- **The metrics generator is on**: from the spans it computes request rate, errors and duration
  per service and operation (`traces_spanmetrics_*`) and who calls whom
  (`traces_service_graph_*`), and pushes them into Prometheus by remote write
  (`kube-prometheus-stack-prometheus.monitoring.svc:9090/api/v1/write`, which Prometheus accepts
  for it). Grafana's service map is drawn from them. Their number grows with distinct span names:
  a sender that puts IDs or full URLs into span names would multiply them - check with
  `count({__name__=~"traces_.+"})` after a new sender, and per name with
  `count by (service, span_name) (traces_spanmetrics_calls_total)`.
  - **Only the requests a service answers** (`SPAN_KIND_SERVER`, `filter_policies`) become span
    metrics: a service's rate, errors and duration are those of its server spans. Its internal
    steps and its calls to others stay in the traces, and the calls still draw the service
    graph (its own processor). On 2026-11-12, 1558 span-metric series came almost all from
    Zitadel's internal Go function spans; 304 were server spans.
  - **At most 5000 series** (`max_active_series`), a cap against a sender gone wrong: beyond it
    new series land in one overflow series, and `TempoSpanMetricsLimited` fires.
- **The finalizer is on** (AGENTS.md): Tempo brings no CRDs and no volume, and its traces are in
  the bucket - removing the Application removes the namespace, not the traces.
- **Memory** 384Mi / 864Mi: the request from real-memory p95 373 MiB (working set p95 383 MiB)
  over 2026-10-23..11-02, the limit from the highest working set seen x 1.1 - 768 MiB, the
  OOMKill below. Queries add to that: a trace lookup reads every block in the bucket,
  split into jobs of 30 blocks. On 2026-11-12 the first lookups after a search pushed the pod from
  404 MiB past its limit within a second (OOMKilled) - too fast for the 30 s memory samples to
  show; the pod cgroup's `container_memory_max_usage_bytes` caught it. Three settings answer it:
  - **Fewer blocks.** Compaction merges only blocks of the same window, by default an hour: the
    bucket held 647 blocks, 576 of them a single hour of about 0.5 MB. With a day as the window
    (`compaction_window: 24h`) 30 days are about 50 blocks, and a lookup reads about a
    thirteenth of the files.
  - **Fewer jobs at once.** The querier works on 20 batches at once by default, and the frontend
    packs several jobs into one batch (`max_batch_size` 7, a lookup job weighing 2), all run at
    once. With `max_concurrent_queries: 4` and `max_batch_size: 1` it is 4 jobs. It slows every
    query alike - searches too - which at a few MB of traces a day is not felt.
  - **`GOMEMLIMIT`** at 90 % of the limit (777MiB - Go's units; Kubernetes' `Mi` stops Tempo at
    start) makes Go collect garbage harder before the kernel kills
    it. Tempo's own `memory.automemlimit_enabled` would do the same from the cgroup's limit, but
    the chart sets it only by replacing its whole configuration template (`config`), which would
    then miss the chart's own changes; so `GOMEMLIMIT` is written out, and a new memory limit
    needs it changed by hand (a note next to the limit says so).
- Restricted Pod Security as the chart ships it plus seccomp, no capabilities, read-only root.
  Usage reports to Grafana are off.

## Alerts

`rules.yaml`, all warnings:

- **`TempoNoSpansReceived`**: no span for an hour (and 15 minutes more). Traefik traces every
  request and Gatus alone makes some every minute, so this is a broken sender, Alloy or receiver.
- **`TempoBackendErrors`**: requests to the bucket answered 403 (key, policy) or 5xx (Hetzner)
  for 15 minutes. A 404 is normal - Tempo looks for files before they exist.
- **`TempoBlocksFailing`**: blocks that could not be flushed, finished or compacted - traces that
  never reach the bucket.
- **`TempoRemoteWriteFailing`**: the metrics generator's samples refused by Prometheus (its
  NetworkPolicy, or remote write switched off).
- **`TempoSpanMetricsLimited`**: the metrics generator reached its 5000 series; find the span
  names that multiply (above), fix the sender, or raise the cap if they are legitimate.

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin -n tempo get pods,externalsecrets
kubectl --context d3strukt0r-prod-admin -n tempo logs statefulset/tempo
kubectl --context d3strukt0r-prod-admin -n tempo port-forward svc/tempo 3200:3200   # then curl localhost:3200/ready
```

**A restart at the very first start is harmless**: right after the bucket was created, Hetzner
answered Tempo's backend scheduler "The specified bucket does not exist" although the bucket was
there and writable (2026-10-18, as Loki's README describes for keys); Tempo stopped cleanly, the
second start found it. Restarts after that are not expected.

Traces are searched in Grafana → Explore → Tempo (by service, span name, duration or TraceQL), or
reached from a log line's trace ID.
