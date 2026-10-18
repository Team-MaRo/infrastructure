# Tempo

Trace storage for the cluster (`kubernetes/clusters/prod/tempo.yaml`, the `tempo` chart of
grafana-community pinned there, values in this directory, namespace `tempo`). Spans arrive over
OTLP from Alloy, which receives them on every node (`components/alloy`); Grafana reads them
(datasource `Tempo`, `components/kube-prometheus-stack`). How the cluster's manifests fit together
is in [`kubernetes/README.md`](../../README.md).

## Design

- **One single-binary Tempo** (chart 3.2.0, Tempo 3.1.0): distributor, live-store, querier and
  backend worker in one pod, no Kafka - Tempo 3's distributed mode needs one, the single binary
  does not. The chart moved from grafana/helm-charts to grafana-community.
- **Traces in Object Storage**, bucket `d3strukt0r-prod-tempo` (`tofu/objectstorage/tempo.tf`),
  with Tempo's own key - the bucket policy admits only that key and the admin key. The key comes
  from OpenBao `secret/tempo-s3` through the ExternalSecret `tempo-s3` as environment variables
  that the config expands (`-config.expand-env`); Reloader restarts Tempo when it changes.
- **30 days**, as Loki's logs, so a trace link in a log line stays valid as long as the line
  (`tempo.retention: 720h`; the chart renders it into the backend worker's
  `compaction.block_retention`, its default is 24 h). The bucket is unversioned and its lifecycle
  rule only clears unfinished uploads - Tempo's compactor deletes old blocks itself.
- **No volume**: the WAL and the blocks not yet in S3 live on an emptyDir at `/var/tempo` - a few
  MB, cleared once uploaded. A deleted pod loses at most the last seconds of spans, which
  tracing can afford, and there is no live-store PVC to go wrong (grafana/tempo#8004). The root
  filesystem is read-only. The emptyDir's 1 GiB cap matches Kyverno's ephemeral-storage limit,
  which counts it and would evict the pod at 1 GiB anyway: if S3 is unreachable long enough for
  blocks to pile up that far, the pod restarts empty instead of filling the node's disk.
- **Receivers**: OTLP gRPC 4317 and HTTP 4318 - only Alloy sends. The chart's Jaeger receivers
  stay because its Service template needs them; nothing sends to them, and like every port here
  they are reachable inside the cluster only.
- **The metrics generator is on**: from the spans it computes request rate, errors and duration
  per service and operation (`traces_spanmetrics_*`) and who calls whom
  (`traces_service_graph_*`), and pushes them into Prometheus by remote write
  (`kube-prometheus-stack-prometheus.monitoring.svc:9090/api/v1/write`, which Prometheus accepts
  for it). Grafana's service map is drawn from them. Their number grows with distinct span names:
  a sender that puts IDs or full URLs into span names would multiply them - check with
  `count({__name__=~"traces_.+"})` after a new sender.
- **The finalizer is on** (AGENTS.md): Tempo brings no CRDs and no volume, and its traces are in
  the bucket - removing the Application removes the namespace, not the traces.
- Memory 256Mi requested, 1Gi limit - a first guess for a few spans a second, to be measured.
  Restricted Pod Security as the chart ships it plus seccomp, no capabilities, read-only root.
  Usage reports to Grafana are off.

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
