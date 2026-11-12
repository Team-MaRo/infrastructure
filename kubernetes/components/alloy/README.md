# Alloy

Log collection and the traces' entry on every node (`kubernetes/clusters/prod/alloy.yaml`, chart
`grafana/alloy` pinned, values and extra manifests in this directory). How the cluster's
manifests fit together is in [`kubernetes/README.md`](../../README.md).

Alloy runs on every node and sends that node's container logs (`/var/log/pods`) and whole
journal (k3s, kernel, apt, sshd), plus the cluster's Kubernetes events, to Loki, which keeps
them 30 days. Storage, labels and searching are in [`../loki/README.md`](../loki/README.md).

It also receives traces: Traefik and Zitadel send their spans over OTLP (gRPC 4317, HTTP 4318)
to the Alloy on their node, which strips the visitor's user agent and address, redacts values
that can be personal data, batches them and forwards them to Tempo ([`../tempo/README.md`](../tempo/README.md)).

## Design

- **Traces pass through, they are not stored here** (`otelcol.receiver.otlp` ->
  `otelcol.processor.attributes` -> `otelcol.processor.transform` -> `otelcol.processor.batch` ->
  `otelcol.exporter.otlp` to
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
- **Values that can be anyone's data are redacted next** (`otelcol.processor.transform
  "redact_values"`) - the standard OpenTelemetry names, from any sender. A trace shows where the
  time went and what failed; the details stay in the app's own log.
  - **Free text goes whole**, since no pattern finds every value inside it: a database query's
    text (`db.statement`, `db.query.text`, the bound `db.query.parameter.*` and
    `db.operation.parameter.*`, `graphql.document`) and an error's text (the span's status
    message, `error.message`, Zitadel's `error_msg`, `exception.message` and
    `exception.stacktrace` in its events). What stays is the
    operation and the table (`db.operation.name`, `db.collection.name`, `db.query.summary`), the
    status code and the error's type (`exception.type`): a trace shows that a `SELECT` on
    `wp_posts` took 230 ms or that it failed with a `PDOException`, not its `WHERE` or the
    message. Scrubbing only the values out was tried and given up on 2026-11-12 after three
    reviews: every SQL dialect quotes differently (PostgreSQL's `E'...'` and `$$...$$`, a
    backslash that escapes in MySQL and not in PostgreSQL, hex literals, text cut off by the
    sender), Valkey commands carry their values with no quotes at all, and error messages quote
    with `'`, `"`, `«»` or not at all - each case let a value through in tests.
  - **Who the user is** goes: `enduser.*` and `user.*`.
  - **URLs** (`url.full`, `url.original`, `http.url`, `http.target`, `url.query`,
    `url.fragment`, and span names, which an HTTP client may build from the URL): values after
    `?`, `&`, `#` or `;` (`;jsessionid=`) become `REDACTED`, the names stay - as Traefik writes
    them itself; a user and password before the host (`postgres://app:pw@db`) become
    `REDACTED`. A bare
    value without a name (`/reset?abc123`) stays; it cannot be told from a flag.
  - **E-mail addresses**, also with the `@` URL-encoded (`%40`), become `REDACTED` in every text
    attribute and the name of a span and its events. The local part takes the usual special
    characters (`o'brien`, `jane&john`) but not `/`, `=`, `?` or `%`, which would swallow the
    (encoded) path in front of an address. A file name shaped like an address (`logo@2x.png`) is redacted too.
    Not covered: list-valued, resource and link attributes, addresses with non-ASCII characters
    or encoded twice.

  This is a list of what to remove, so a name nobody listed passes - deliberately, since the logs
  in Loki hold more personal data than the traces and get no redaction at all (2026-11-12: the
  Traefik access log keeps visitors' addresses, the apps' errors their full text). Once PHP
  sends spans, the traces hold what no log has - every query a page runs - and the query text is
  covered above.

  A new sender that records personal data under its own names (a user name in `wp.user_login`,
  captured headers such as `http.request.header.cookie`) gets them deleted in `attributes
  "redact"` - look at a few of its spans before it goes live. The patterns are defined once, as
  Helm template variables at the top of the block (the chart passes the configuration through
  `tpl`). **A statement that fails refuses the sender's whole batch** (`error_mode` `propagate`)
  instead of letting it through unredacted, and `AlloySpansDropped` fires: traces are missing
  until it is fixed, but nothing leaks, and nothing needs deleting from Tempo afterwards (a
  statement Alloy cannot read at all is another case: Alerts below). Its
  warning in Alloy's log, `failed to execute statement`, names the statement. Tested in Docker
  on 2026-11-12 (Alloy v1.20.1, the rendered configuration) with crafted spans for every rule
  above, a Traefik span that must pass unchanged, and a failing statement (503, counted in
  `otelcol_receiver_refused_spans_total`, nothing passed).
- **Alloy runs in `kube-system`**, the PSA-exempt namespace, because it mounts `/var/log` from
  the host. The log files belong to root with mode 0640, so it runs as UID 0 - but with no
  capabilities, no privilege escalation and a read-only root filesystem; as the files' owner,
  root needs no capability to read them. Its read positions live on the node
  (`/var/lib/alloy`), so a restarted pod neither re-sends nor skips lines, and it mounts
  `/etc/machine-id`, by which the journal reader finds the node's own journal.
- **Memory** (`values.yaml`; kube-system has no LimitRange), over 2026-10-23..11-02, the
  request from real memory's p95, the limit from the working set's: Alloy 240Mi / 512Mi (real p95
  230 MiB; working set p95 255 MiB, peak 259 MiB), its config reloader 16Mi / 32Mi (14; 15, peak
  18 MiB) instead of the chart's 50Mi request without a limit.
- **Its chart's RBAC is not used.** Alloy's chart role reads every Secret and ConfigMap, so
  `rbac.create: false` and `clusterrole.yaml` grants only pods, namespaces and events.
- **Events are collected once**: the Alloy instances form a cluster, and a
  `loki.source.kubernetes_events` watching all namespaces runs on one of them. The API keeps
  events for an hour; Loki keeps them 30 days.
- **Labels**: `namespace`, `pod`, `container`, `node`, `app` for containers; `job="node-journal"`,
  `unit`, `node` for the journal; `job="loki.source.kubernetes_events"` for events. Anything
  else is searched in the line, not labelled - Loki stays fast with few labels.
- **Traefik's upload scan lines also become a counter**,
  `loki_process_custom_upload_scan_total` by `verdict` and `refused` (a `stage.match` in
  `loki.process "pods"`), for the scan's alerts
  ([`../traefik/README.md`](../traefik/README.md), "Upload scan"). A metric takes the entry's
  labels, so the two are set for `stage.metrics` and dropped again: Loki's streams keep their
  labels. Alerts on log lines go this way, through Prometheus, rather than through Loki's ruler.
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
  or with its queue full - they are missing in Tempo. A refusal with `failed to execute statement`
  in Alloy's log is a redaction statement that failed on a span's values (Design above): fix
  it and test it as described there. A statement Alloy can no longer read at all - an upgrade
  removed or renamed a function - stops Alloy from starting: then `AlloyDown` and
  kube-prometheus-stack's `KubeDaemonSet*` fire instead, logs stop too, and the pod's log says
  `could not perform the initial load successfully`.
