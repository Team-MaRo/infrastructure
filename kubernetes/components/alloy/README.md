# Alloy

Log collection and the traces' entry on every node (`kubernetes/clusters/prod/alloy.yaml`, chart
`grafana/alloy` pinned, values and extra manifests in this directory). How the cluster's
manifests fit together is in [`kubernetes/README.md`](../../README.md).

Alloy runs on every node and sends that node's container logs (`/var/log/pods`) and whole
journal (k3s, kernel, apt, sshd), plus the cluster's Kubernetes events, to Loki, which keeps
them 30 days. Storage, labels and searching are in [`../loki/README.md`](../loki/README.md).

It also receives traces: Traefik and Zitadel send their spans over OTLP (gRPC 4317, HTTP 4318)
to the Alloy on their node, which strips the visitor's user agent and address, batches them and
forwards them to Tempo ([`../tempo/README.md`](../tempo/README.md)).

## Design

- **Traces pass through, they are not stored here** (`otelcol.receiver.otlp` ->
  `otelcol.processor.attributes` -> `otelcol.processor.batch` -> `otelcol.exporter.otlp` to
  `tempo.tempo.svc:4317`, plain gRPC inside the cluster). The exporter keeps a queue and
  retries, so a Tempo restart costs no spans as long as the queue holds; the senders only need
  to know Alloy. The ports are opened through `alloy.extraPorts` on the pods and the Service,
  and the Service's `trafficDistribution: PreferSameNode` sends each sender to the Alloy on its
  own node - Traefik on the host network too - and to another node's only while its own is
  down. A new app that should be traced sends OTLP to `alloy.kube-system.svc:4317` (gRPC) or
  `:4318` (HTTP).
- **Spans lose the visitor's user agent and address** before they leave
  (`otelcol.processor.attributes "redact"`, the current OpenTelemetry names and the older ones).
  Every request is traced and kept 30 days, and finding a fault needs only path, status and
  timings; hosts not proxied by Cloudflare would otherwise keep visitors' real IPs. Traefik
  replaces query values with `REDACTED` in spans itself, keeping only the parameter names. The
  access log keeps address and user agent for failed and slow requests, under the same trace ID.
  A new sender that records other personal data (a user name, an e-mail address) gets its
  attribute added here.
- **Alloy runs in `kube-system`**, the PSA-exempt namespace, because it mounts `/var/log` from
  the host. The log files belong to root with mode 0640, so it runs as UID 0 - but with no
  capabilities, no privilege escalation and a read-only root filesystem; as the files' owner,
  root needs no capability to read them. Its read positions live on the node
  (`/var/lib/alloy`), so a restarted pod neither re-sends nor skips lines, and it mounts
  `/etc/machine-id`, by which the journal reader finds the node's own journal.
- **Its chart's RBAC is not used.** Alloy's chart role reads every Secret and ConfigMap, so
  `rbac.create: false` and `clusterrole.yaml` grants only pods, namespaces and events.
- **Events are collected once**: the Alloy instances form a cluster, and a
  `loki.source.kubernetes_events` watching all namespaces runs on one of them. The API keeps
  events for an hour; Loki keeps them 30 days.
- **Labels**: `namespace`, `pod`, `container`, `node`, `app` for containers; `job="node-journal"`,
  `unit`, `node` for the journal; `job="loki.source.kubernetes_events"` for events. Anything
  else is searched in the line, not labelled - Loki stays fast with few labels.
- Alloy sends no usage statistics (`enableReporting: false`).

## Alerts

`pod-monitor.yaml` has Prometheus scrape every Alloy pod (job `kube-system/alloy`, label `node`);
`rules.yaml` alerts, all warnings:

- **`AlloyDown`**: a pod that has not answered for 10 minutes - that node's logs and traces are
  not arriving. A pod missing altogether is kube-prometheus-stack's `KubeDaemonSet*` alerts.
- **`AlloyComponentUnhealthy`**: a block of the configuration not healthy for 15 minutes - a bad
  setting after a change, or a source or sender that failed. The log or Alloy's UI (port-forward
  a pod's 12345) names it.
- **`AlloyLogsDropped`**: log lines Alloy gave up on after retrying Loki (queue full, rate
  limits, Loki's errors), with the reason as a label - they are missing in Loki.
- **`AlloySpansDropped`**: spans refused from a sender, or given up after the exporter's retries
  or with its queue full - they are missing in Tempo.
