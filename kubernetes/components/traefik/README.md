# Traefik

Back to [`kubernetes/README.md`](../../README.md).

Traffic enters through Traefik, which k3s ships. Traefik is k3s's own - k3s installs and
upgrades it from its bundled chart - and is configured only through the `HelmChartConfig` in
`kubernetes/components/traefik/` (`helmchartconfig.yaml`, Application `traefik`), next to the
dashboard's certificate (`certificate.yaml`), its Zitadel gate
(`middleware-oauth2-proxy.yaml`), the upload scan (`middleware-upload-scan.yaml`,
`plugins/upload-scan/`, "Upload scan") and
the PodMonitor for its metrics (`pod-monitor.yaml`). It
runs as a **DaemonSet on every ingress node's host network** and listens on the node's ports 80
and 443 itself, IPv4 and IPv6. `prod.d3strukt0r.dev` has an A and an AAAA record per ingress
node; a service on the cluster gets a CNAME to it and an `Ingress` (class `traefik`, the
default).

## Why the host network

**Why the host network**: the cluster is single-stack IPv4, and k3s's servicelb only
rewrites IPv4 packets to a pod (iptables DNAT; a pod without an IPv6 address has nothing to
rewrite IPv6 to). On the host network Traefik accepts IPv6 directly and talks to the pods
over the IPv4 pod network, reaching pods on other nodes through the WireGuard tunnels. Its
Service is `ClusterIP` (`service.spec.type` in this chart, not `service.type`), so servicelb
no longer claims 80/443. Pods themselves still cannot reach IPv6-only destinations; if that
is ever needed, a forward proxy on the host network is the targeted fix, dual-stack (a
rebuild) the complete one.

## How it is set up

- **Ingress nodes are the ones labelled `node-role.kubernetes.io/ingress=true`**
  (`k3s_node_labels`, today the three servers; the worker `prod-04` has none). **That label,
  the DNS map `local.prod_nodes` in `tofu/cloudflare` and later a load balancer's targets must
  list the same nodes** - a node in
  DNS without Traefik answers nothing. A workload-only node gets no label: no Traefik, not
  internet-facing. There is no standard ratio; keep at least two or three for redundancy and
  size them by traffic, not by node count.
- **Ports below 1024 without root**: Traefik runs as UID 65532 without capabilities;
  `net.ipv4.ip_unprivileged_port_start = 80` on the nodes (`k3s_sysctls`) lets it bind 80 and
  443. Its other entrypoints, metrics `:9100` and `:8080`, also open on the host but are closed by
  the Hetzner firewall. (node-exporter therefore listens on 9101, not its default 9100.)
- **Its metrics** are scraped through a PodMonitor (`components/traefik/pod-monitor.yaml`, port
  `metrics` 9100 on the host network), which gets `SkipDryRunOnMissingResource=true` from the
  shared Component `shared/monitoring-sync-options`, since the CRD belongs to another
  Application.
- **The access log records only failed and slow requests** (`accessLog` in
  `helmchartconfig.yaml`, user decision 2026-10-06): status 400-599 or over 2 s, as JSON on
  stdout, so Alloy ships it to Loki (30 days). Headers are dropped except `CF-Connecting-IP` (the
  visitor; the client address Traefik sees is Cloudflare's) and `User-Agent`; query strings are
  dropped from the path. Search it in Grafana's Explore:
  `{namespace="kube-system", container="traefik"} | json | DownstreamStatus >= 500`. Traefik's
  own log (start, configuration, backend and TLS errors) is JSON as well (`log.format`), in the
  same stream: `| json | level="error"`.
- **Alias headers are dropped** (`aliasHeadersStrategy: delete` on every entry point): a header
  such as `X_Forwarded_For` becomes `X-Forwarded-For` in PHP's `$_SERVER`, so a client could
  otherwise forge what Traefik sets. Dropped, not rejected - odd clients still get an answer.
- **Safe naming** (`providers.kubernetesCRD.safeNaming`): names Traefik generates from CRDs join
  namespace and name with `_` instead of `-`, so two objects can never collide. A Middleware is
  referenced from an Ingress as `<namespace>_<name>@kubernetescrd`
  (`traefik.ingress.kubernetes.io/router.middlewares`); an IngressRoute names it by `name` and
  `namespace` and is unaffected. A wrong reference fails closed - the router is not served (404)
  and the dashboard lists its error. The same names appear in the access log (`RouterName`,
  `ServiceName`) and in the metrics' `router`/`service` labels.
- **Memory: 112Mi requested, 1Gi limit** (`resources` in `helmchartconfig.yaml`; real-memory p95
  104 MiB; working set p95 123 MiB, peak 141 MiB over 2026-10-23..11-02). A scanner's burst of
  requests (1249 in a minute, 2026-10-07) pushed the Traefik on prod-02 past the former 256Mi and
  got it OOM-killed; the limit does not hold back scheduling. Every ingress node needs the headroom,
  since any of them can take such a burst. 1Gi since the upload scan, which holds up to 404 MiB
  of bodies plus one being joined ("Upload scan"); the chart sets Go's `GOMEMLIMIT` from the
  limit.
- **Only through Cloudflare, at most 20 requests a second per visitor** (Middleware
  `cloudflare-only`, [`../../charts/cloudflare-only/README.md`](../../charts/cloudflare-only/README.md)):
  every route of a name Cloudflare proxies puts this chain first. It admits only Cloudflare's IP
  ranges (anyone else gets 403 - Traefik on the host network sees the connecting Cloudflare
  server as the client, so that check needs no header), then limits each visitor to 20 requests
  a second on average with bursts of 100, grouped by `CF-Connecting-IP` (429 above it, logged).
  A scanner's 1249 requests in a minute (2026-10-07) had got the Traefik on prod-02 OOM-killed.
  Both are per route, not on the entry point: names that are not proxied (the registry, later
  Forgejo) share port 443, must stay reachable directly and carry no `CF-Connecting-IP` - on the
  entry point their clients would all share one bucket. Behind the chain, `CF-Connecting-IP` can
  only come from Cloudflare, which the rate limit, the sites' nginx and the xmlrpc allowlist
  rely on. A route that names the middleware without its namespace's copy existing is switched
  off (404) - it fails closed.
- **Every request is traced** (`tracing.otlp` in `helmchartconfig.yaml`, the default sample rate
  1.0): Traefik sends a span per request over OTLP gRPC to the Alloy on its node
  (`alloy.kube-system.svc:4317`, resolved through `ClusterFirstWithHostNet`), which forwards it
  to Tempo ([`../tempo/README.md`](../tempo/README.md)). The access log then carries the
  request's `TraceId` (and `trace_id`, the OpenTelemetry name Traefik writes alongside), and
  Grafana links a logged request to its trace. The apps behind Traefik show up in a trace only
  once they send their own spans (Zitadel does).
- **The dashboard is at `https://traefik.d3strukt0r.dev`** (read-only), switched on through the
  chart's `ingressRoute.dashboard` in the HelmChartConfig - an IngressRoute on `websecure` with
  certificate `traefik-tls` and the Zitadel gate (see [`../oauth2-proxy/README.md`](../oauth2-proxy/README.md)).
- **Ingresses report `prod.d3strukt0r.dev` as their address** (`ingressEndpoint.hostname`).
  k3s's values copy the address from Traefik's Service (`publishedService`), which as
  `ClusterIP` has none, and Argo CD counts an Ingress without an address as Progressing - the
  first Ingresses (Zitadel's) blocked their sync that way.
- **Updates replace one node at a time** (`maxUnavailable: 1`, `maxSurge: 0`): two Traefiks
  cannot hold the same host port. That node's share of requests fails for the seconds in
  between, as during a kured reboot - DNS keeps pointing at it.
- **Plugins only in local mode**: the source in a volume, not downloaded at start - a failed
  download would break every route that uses its middleware. The one today is our upload scan
  ("Upload scan").
- **A Hetzner Load Balancer fits on top without changing Traefik**: targets by label over the
  private network, TCP 80 and 443 passed through (TLS stays at Traefik), health checks, PROXY
  protocol for client IPs, and one address (IPv4 and IPv6) in DNS instead of one per node. The
  firewall could then close 80/443 on the nodes' public side. With PROXY protocol trusted from
  the balancer (`proxyProtocol.trustedIPs` on the entry points), Traefik takes the original
  address as the client again, so `cloudflare-only` works unchanged.

## Upload scan

Every uploaded file is to be handed to the shared clamd before the app gets it
([`clamav/README.md`](../clamav/README.md)), so nobody can upload malware to any app on the
cluster without each app setting something up. Our own plugin does it, `plugins/upload-scan`
(Go, standard library only, with tests), its settings `middleware-upload-scan.yaml`.

- **Loaded as a local plugin**: `kustomization.yaml` puts its source into the ConfigMap
  `traefik-upload-scan`, `helmchartconfig.yaml` mounts it under `/plugins-local/src/…` and
  declares it (`experimental.localPlugins`, type `localPath`). No image, no plugin registry;
  Traefik runs it in its Go interpreter (Yaegi). **Traefik reads plugins only at start**: the pod
  annotation `upload-scan/source` in `helmchartconfig.yaml` is a hash of the plugin's source, so a
  change to it changes the HelmChartConfig in the same commit, which rolls Traefik one node at a
  time. The limits that size its memory (100 MiB per body, 4 at once) are therefore in the code
  (`body.go`), not in the Middleware, whose settings Traefik applies without a restart.
  `go test` in `plugins/upload-scan` fails until the hash matches and prints the right one.
  Reloader is not used for it: it would need to read Secrets in `kube-system`.
- **What it scans**: every request with a body, whatever its method (apps parse uploads on
  DELETE and others too). A multipart body is **not parsed the way
  one parser would**: parsers differ (PHP, Node, Python, Go each split some bodies differently,
  2026-11-06), and every difference was a way around the scan. It is cut wherever any parser
  would cut: at every `--boundary` at a line start (after a CRLF or an LF, after a closing
  delimiter too), for the boundary values a parser could take from the `Content-Type` headers
  (the first `boundary` anywhere, as PHP reads it, and the first and last parameter of that
  name, in up to 4 header values). Of each piece what follows its headers (the first empty line,
  `\n\n` or `\n\r\n`) is scanned on its own, without the line break that belongs to the next
  delimiter - exactly the bytes a parser stores, which clamd's whole-file hash signatures need.
  Form fields are scanned as well - telling fields from files is where parsers disagree - so the
  log's `parts` counts them (those clamd answered). Pieces are scanned as they are found, never
  collected, each once even where two boundary values cut at the same lines, over one buffer per
  request; the time budget or the 1,000th part ends the scan (`incomplete`). A multipart body
  without its boundary, or without any part with content, is scanned whole. A multipart type
  other than `multipart/form-data` (`multipart/mixed` and the like) is scanned whole and in parts:
  parsers turn only form-data into files, the app gets any other as it is (WordPress stores such a
  body as the file when it comes with a `Content-Disposition`). Any other body as one file
  (WordPress' media upload through its REST API, SVGs).
- **Ambiguous multipart is marked, not puzzled out**: reading every body exactly like every parser
  does cannot be kept up (each review round found another quirk of PHP's parser alone,
  2026-11-06). Browsers and HTTP libraries send one plain kind - one `Content-Type` header,
  `multipart/form-data` with one clean `boundary`, no `Content-Encoding`, at most 1,000 parts, a
  part's headers under 4 KiB (PHP reads them through a 5120-byte buffer and at some line lengths
  takes the rest of a part's headers for its content, measured 2026-11-07), the body starting with
  the delimiter, delimiters only at line starts and with the same line breaks, each delimiter line
  ending right after it (only `--` or spaces between), a closing delimiter with nothing after it,
  parts Go's strict parser accepts and none with a `Content-Transfer-Encoding` (Go decodes
  quoted-printable before an app stores it, PHP does not, so the scan may have seen other bytes).
  A body that departs from that gets `ambiguous` with the reason in its log line (`ambiguous.go`),
  next to the verdict of the scan above, and is refused (422), which closes parser differences
  for every language at once. Any other body with a `Content-Encoding` is marked too: clamd saw
  the compressed bytes, and an app that inflates the body would store what it never saw.
- **Not scanned**: gRPC, gRPC-web, Connect and `application/proto` (Argo CD's and Zitadel's
  consoles and CLIs) - never read, since a stream that keeps its body open would hang; JSON (and
  `+json`) and `application/x-www-form-urlencoded` up to 64 KB of known length - the type is the
  client's word, and never a multipart one (some parsers take `multipart/form-data+json` for
  form-data); a request with any `Content-Disposition` header is scanned whatever its type
  (WordPress' raw media upload needs that header). Nothing is excluded by host or path today
  (`exclude` in the Middleware: a host, or a host with an exact path). Base64 inside small JSON
  stays unscannable (accepted).
- **Memory**: a body is read in pieces of up to 1 MiB, the first one starting at 64 KiB and
  doubled each time it is full - 128 KiB, 256 KiB, 512 KiB, 1 MiB, copying what it holds (at most
  960 KiB per body) - so a body with a known length up to 1 MiB stays one piece and is never
  joined (a chunked one too, unless it ends one byte past a piece); the memory pool counts them in
  units of 64 KiB (16 for a full piece) as they are allocated, and a piece is asked for only once
  the one before is full, so a client holds 64 KiB or about twice what it has sent, whichever is
  more: one that sends one byte and stalls holds one unit, one that stalls after 64 KiB two (one
  for a chunked body, which reads a byte ahead before it grows), after 1 MiB 32 (what it sent and
  the piece being filled). One-byte clients fill a pool after about 6,400 connections at almost no
  cost; what keeps that from one address are the limits per address. A small API call holds one
  unit, an upload that only connects none, since a body asks for memory only once its first byte
  has arrived. The pool is 6464 units (404 MiB) per Traefik, room for four bodies of 100 MiB
  (about Cloudflare's own limit per request) and the chunk that tells a chunked one is too large,
  shared by however many bodies there are; each piece is counted until Traefik has passed
  it on to the app - a joined multipart body is one piece, so its memory goes back only once it
  has been passed on completely. A body is admitted once its announced size (one unit when it is
  unknown) fits next to what the bodies being read have set aside. What a body sets aside follows
  its pace: what it would send in the next 30 s at its rate so far, all of it during its first
  3 s - a fast upload keeps its room, a slow or stalled one holds back little, and holding the
  whole pool takes about 13 MiB/s of real uploads. That is a bet on the pace: room a body held
  back may have gone to another by the time it needs it, and then both wait. A body takes only
  units not set aside for another; freed units go first to the bodies being read that wait for a
  chunk, oldest first among those they are enough for, then to the waiting bodies that now fit.
  So a steady stream of small requests can keep a large one - a body waiting to be admitted, or
  for a full 1 MiB piece while smaller ones get theirs - waiting until its time is up. Accepted:
  memory mostly comes back a full piece at a time, and then the oldest waiting body gets it; making
  later bodies wait behind it instead blocked them, and every new upload, while memory was free.
  While two or more bodies being read wait for a chunk, one that has waited a second gives up its
  scan if it is the one to give way - the newest waiting one that took more than it announced,
  such as a chunked body, else the newest waiting one - and no other gave way in the last second,
  so stuck bodies free memory one at a time whoever else holds some; its memory comes back once
  the app has it. A body waiting alone waits out its time, since memory comes back as the others
  finish. Waiting - to be admitted, for a chunk, for a clamd slot, for the join - takes at most
  10 s together; reading a slow body does not count. A body that does not get in, or gets no
  chunk in time, is not scanned (`skipped:busy`; for a chunked body `bytes` is then what was read
  so far) and refused with 503 - in report mode the app gets what was read followed by the
  rest. Cutting a multipart body
  needs it in one piece, so one with a usable boundary is joined: its pieces are copied into one
  buffer, one body at a time per Traefik, then cleared, and for a body from 16 MiB their memory is
  returned to the system at once (`debug.FreeOSMemory`), since a later large buffer could not
  reuse 1 MiB pieces and would take fresh memory while they wait for the garbage collector. Every
  other body is scanned and passed on straight from its pieces. A client that hangs up gives up
  its clamd slot, and its wait for one, at once; while its body is still being read, Go
  notices the hang-up (HTTP/1.1) only at the body's end, so one that disconnects halfway keeps its
  place until its 10 s of waiting are up. Measured 2026-11-09 in Docker under the cluster's limits
  (1 GiB, `GOMEMLIMIT` 921 MiB), four 90 MB multipart uploads at once: 535-630 MiB peak with a
  size header (it varies between runs), 480-550 MiB chunked, all scanned (reserving up front
  before needed 408 MiB with a size header but scanned only two of four chunked ones); two clients
  trickling chunked uploads no longer make a normal upload `skipped:busy`; and more uploads than
  fit at once are all scanned, the later ones waiting for room; 300 and 400 multipart uploads of
  1 MB at once, each growing its first piece through every step, peaked at 97-253 MiB, all
  scanned (the doubling's copies and garbage included). Larger bodies are not scanned
  (`skipped:too-large`) and refused with 413.
- **Per address** (2026-11-09): one address may have at most 16 uploads checked at once and hold at
  most one full-size body's memory (1616 units, a quarter of the pool) - taken, and set aside for
  what its bodies announced, so it cannot announce four bodies of 100 MiB and hold the pool's room
  with them; a body takes only what is not set aside for the address's other bodies, as in the
  pool - so neither many stalled uploads nor a few large ones from one address make every other
  upload `skipped:busy`. A 17th upload is not read at all, logged `skipped:too-many` and refused
  with 429. A body that would go over the memory waits for the address's own memory to
  come back, without counting as stuck next to other addresses' bodies (it never makes one give
  way). Only when all of the address's memory is held by its bodies waiting on its share - none of
  it with a body still being read, scanned or passed on, which gives it back by itself, or with
  one waiting on the pool, which the pool's rule settles - do they settle it among themselves like
  bodies stuck on the pool: after a second the one beyond its announcement, else the newest,
  gives way, at most one per address per second, apart from the pool's give-ways. The other may
  still not fit afterwards, or run out of time, and fail too (rare, accepted). Such a body ends
  `skipped:too-many` (refused with 429, like a 17th upload) when the pool, set-aside
  room included, had room for it, else `skipped:busy`. The address is the visitor's that
  Cloudflare passes on (`CF-Connecting-IP`), else the connection's; IPv6 counts by its `/64`,
  since one network holds more addresses than could be counted one by one. The header can be
  trusted because the scan runs only behind `cloudflare-only`, which lets nothing but Cloudflare
  reach it. Each Traefik counts for itself (Cloudflare spreads a client over them), and a shared
  address - a household, a venue's Wi-Fi, a mobile carrier - shares these limits. Several
  addresses together are not held back; memory going to whoever is actually sending would be the
  answer to that, should it matter.
- **Time**: at most 10 s waiting plus one budget of 45 s for all files of a request, clamd's
  reload wait (~9 s) included; clamd itself ends one file's scan after 30 s and answers
  `limits-exceeded`, so a single slow file does not use up the request's time unanswered. clamd
  unreachable ends the request's scan after the first attempt. A request scans within one of the
  four clamd slots per Traefik, and all its parts go to clamd over one connection, a session
  (`IDSESSION`), so a body of many small parts costs one connection and one DNS lookup instead of
  one each, and its budget goes to scanning (1000 parts against the real clamd in Docker,
  2026-11-09: 0.7-1.1 s with a session, 1.4-1.5 s with a connection each; the first run after
  clamd's start took about 7 s either way). A session clamd breaks off fails the part it was on,
  and the next part opens a new one within the same slot. clamd keeps an open session among the
  connections it polls, so its `MaxQueue` has to stay above four per Traefik
  ([`clamav/README.md`](../clamav/README.md)). Traefik's own read timeout (300 s on `web` and
  `websecure`, `helmchartconfig.yaml`; its default of 60 s cut off large uploads over slow
  connections) bounds receiving the body, counted from the request's start; the scan after it
  does not count, since Go lifts that deadline once the body is read. Waits for memory - once the
  first byte is there, and between pieces - fall inside it. Cloudflare passes an upload on as it
  arrives, apparently after collecting about its first MiB (2026-11-08: 2 MiB sent throttled took
  44 s in the browser and 21 s at the plugin), so a slow sender's time counts here. Cloudflare
  waits up to 125 s for the first bytes of the answer and then between any two (its Proxy Read
  Timeout, fixed below Enterprise), and allows 30 s for writing the request to us (its Proxy Write
  Timeout, fixed; its docs do not say whether per write or in total).
- **Code**: `config.go` (settings, defaults), `filter.go` (which requests), `body.go` (reading,
  holding and joining the body), `pool.go` (memory), `queue.go` (the time a request may wait),
  `client.go` (the limits per address), `check.go` (the request's path), `multipart.go` (cutting
  a multipart body into what is scanned), `ambiguous.go`, `clamd.go`, `log.go`, `refuse.go`
  (which verdict is refused how), `negotiate.go` (the answer's format and language), `respond.go`
  (writing it), `messages.go` (its texts); each has its
  tests in `<name>_test.go`, the fake clamd and the shared helpers in `fakeclamd_test.go` and
  `setup_test.go`. Yaegi 0.16 knows no
  built-in `min`/`max`, drops the first value of a double assignment with a call on the right,
  loops forever on `for i := 0; ; {`, and keeps a named result from the function's previous call
  when this call does not set it (2026-11-09: a body's copied first piece reappeared as an offset in
  its later pieces) - the code avoids all four; `safeScan`'s named result is set on every path.
- **Refusing** (`mode: block`, phase 3, 2026-11-10): only `clean` passes - everything else the
  scan looked at is refused and never reaches the app; a request with nothing to scan (no body, a
  type not scanned) passes as before. 422 for `infected` and `limits-exceeded` (their text names
  the signature) and for any body marked `ambiguous` whatever else its scan said (one of more than
  1,000 parts, whose scan stops at the 1,000th, would otherwise end `incomplete` with a retry);
  413 for `skipped:too-large`; 429 for `skipped:too-many`
  and 503 for `skipped:busy`, both with `Retry-After: 10`; 503 with `Retry-After: 30` for
  `incomplete`, `skipped:time`, `limits-exceeded` of `MaxScanTime` and `error` (clamd
  unreachable, a fault in the scan); 400 for `aborted`. No `Retry-After` where the same upload
  would fail again: a limit the file itself exceeds outweighs another part's `MaxScanTime`, and
  stays the verdict when time runs out afterwards. A client that already left gets 499 without a
  body, the status Traefik's proxy gives it, so the access log and the metrics count it alike;
  one that hangs up halfway through its body over HTTP/1.1 is `aborted` with 400 instead, since
  Go notices the hang-up only as a broken body - an answer it no longer gets. The
  answer follows the client's `Accept` - JSON (`code`, `message`, `retry_after`; WordPress's REST
  errors name them so, and its block editor shows `message`) when it prefers
  `application/json` or a `+json` type, a minimal HTML page (no script, no branding, the same for
  every site) when it prefers `text/html`, plain text otherwise, `*/*` included - and its
  `Accept-Language`: English, German (Swiss spelling), French, Italian or Spanish, English for any
  other (`messages.go`). An answer before the body's end (413 by its size header, busy or too-many
  partway) leaves the rest unread: Go's server reads up to 256 KiB more, then closes the
  connection. With one clamd ([`clamav/README.md`](../clamav/README.md)), scanned uploads get 503
  while it restarts - every body the scan reads, not only files: a JSON or form body over 64 KiB
  (a long post saved in the block editor) too, since its type is the client's word and an upload
  labelled JSON must not pass whenever clamd is busy. Report mode (`mode: report`, the plugin's
  default) lets every request pass, also when looking at the read body or scanning it fails, and
  only logs - "Switching back to report mode" below.
- **The log**: one JSON line per upload in Traefik's log - the mode (`upload_scan`), host, path
  without query, number of parts clamd answered, size, verdict (`clean`, `infected` with the
  signature, `limits-exceeded` for a file beyond ClamAV's own limits - `AlertExceedsMax`, not
  malware found -, `error`, `aborted` when the client stopped sending, sent less than it
  announced or left while waiting, `incomplete` when time ran out or the client left with parts
  unscanned and none of the scanned ones decided the request already, `skipped:time`
  when that happened before the first part, `skipped:busy`, `skipped:too-many` when its address
  was over one of its limits, `skipped:too-large`), why a body was scanned whole or stopped early
  (`note`), `ambiguous` with its reason, `refused` with the status of a refused request, and the
  milliseconds it took. No client address and no file names.
- **Rolled out in steps**: first loaded with no route using it - while Traefik rolls node by node,
  a node whose Traefik has not restarted yet does not know the plugin and would answer a route
  that uses it with 404, and a plugin that fails to load would take that route down everywhere.
  Once every node's Traefik has logged `Plugins loaded`, the dashboard route
  (`traefik.d3strukt0r.dev`) uses it, after the login gate, so only logged-in admins can make it
  buffer and scan. Then for every site **inside the `cloudflare-only` chain**
  (`kubernetes/charts/cloudflare-only`) rather than on the entry points: an entry-point middleware
  runs before a route's `cloudflare-only`, so a client talking to a node directly would be
  buffered and scanned before its 403, and DNS-only routes (the registry) would be scanned too.
  Traefik takes middlewares only from the route's own namespace, so each site's namespace gets a
  copy of the `upload-scan` Middleware with the same settings, the way it gets `cloudflare-only`.

The verdicts in Grafana → Explore → Loki:

```logql
{namespace="kube-system", container="traefik"} | json | upload_scan != ""
```

A test with the EICAR string (it counts only as a small file of its own), through the dashboard
route, where the scan sits behind the login: logged in at `https://traefik.d3strukt0r.dev/`, in
the browser's developer console - the string is built in memory, so it never lands on the disk,
where a virus scanner on the machine would flag it. It comes back 422 with the text in the
browser's language, and Traefik's log shows `infected` with `refused: 422`:

```js
const eicar = 'X5O!P%@AP[4\\PZX54(P^)7CC)7}$' + 'EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*';
const form = new FormData();
form.append('file', new Blob([eicar]), 'eicar.txt');
fetch('/', { method: 'POST', body: form, headers: { Accept: 'application/json' } })
  .then(async r => console.log(r.status, await r.text()));
```

Tests of the plugin itself, in `plugins/upload-scan` (a fake clamd; EICAR built at run time,
never kept in a file) - compiled Go, and under Yaegi in the version Traefik 3.7 ships, since that
is what runs it in the cluster (Yaegi 0.16 dropped a value from a double assignment that compiled
Go kept, 2026-11-06):

```shell
go test ./...
go run github.com/traefik/yaegi/cmd/yaegi@v0.16.1 test .
```

Checked against PHP itself (2026-11-06): Traefik 3.7.15 with the plugin in front of PHP 8.3 and a
stand-in clamd that records the SHA-256 of every stream it gets. The yardstick is byte-exact:
every file PHP stores must have reached clamd exactly - an extra line break or a cut file would
miss clamd's whole-file hash signatures, which EICAR, lenient about trailing bytes, does not
show. 40 crafted uploads (boundary tricks, duplicate and uppercase parameters, file names twice
or with `filename*`, LF and mixed line endings, folded headers, header lines without a colon,
preambles, parts after the closing delimiter, no closing boundary, quoted-printable, 25 files,
raw bodies), real uploads from Chrome, and 1,600 random multipart bodies (random line endings,
junk header lines, missing or extra delimiters, epilogues, random file bytes): no stored file
was missed. The random bodies found two cases the crafted ones had not (a part whose content
starts with a `--boundary` line - PHP treats it as content when the part has a
`Content-Disposition`, as a delimiter when not); both are unit tests now. The only bodies not
scanned are the small form and JSON bodies without `Content-Disposition` skipped on purpose,
which PHP does not store as files. Repeat it after every change to how the plugin cuts multipart
bodies - it runs the plugin in report mode, so every body reaches PHP:
`plugins/upload-scan/compare/run.sh` ([`compare/README.md`](plugins/upload-scan/compare/README.md)).

The repo's pre-commit hook (`.githooks/pre-commit`) runs both on the staged versions whenever a
commit touches the plugin, `helmchartconfig.yaml`, `kustomization.yaml` or
`middleware-upload-scan.yaml` (a test hands its settings to the plugin), so neither the source
hash, a half-staged plugin nor a setting the plugin refuses goes in unnoticed; a clone turns it
on once with `git config core.hooksPath .githooks` (`AGENTS.md`). GitHub runs the same two on
every push and pull request touching those files (`.github/workflows/upload-scan.yml`, the tests
under `-race`), also for a clone without the hook; its actions follow their major version (`@v7`).
Making the job a required check of `master`'s branch protection is a
setting on GitHub, not in the repo.

### Switching back to report mode

When the scan refuses uploads it should not - a false positive on a real file, or clamd down for
longer than a restart (`ClamavDown`) - report mode lets every upload pass again while the verdicts
are still logged. In `middleware-upload-scan.yaml` set `mode: report`, push to `master`; Argo CD
syncs the Middleware and Traefik applies it without a restart. The next upload's log line says
`"upload_scan":"report"`:

```logql
{namespace="kube-system", container="traefik"} | json | upload_scan != ""
```

Only one host or path affected (an app whose uploads clamd misreads): put it on `exclude` in the
same file instead, and block mode stays for every other. Set `mode: block` again once the cause
is gone.

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin -n kube-system get ds traefik -o wide
kubectl --context d3strukt0r-prod-admin get nodes                   # ROLES shows ingress
```
