# Traefik

Back to [`kubernetes/README.md`](../../README.md).

Traffic enters through Traefik, which k3s ships. Traefik is k3s's own - k3s installs and
upgrades it from its bundled chart - and is configured only through the `HelmChartConfig` in
`kubernetes/components/traefik/` (`helmchartconfig.yaml`, Application `traefik`), next to the
dashboard's certificate (`certificate.yaml`), its Zitadel gate
(`middleware-oauth2-proxy.yaml`) and
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
- **Memory: 128Mi requested, 512Mi limit** (`resources` in `helmchartconfig.yaml`). It uses
  70-160 MiB normally; a scanner's burst of requests (1249 in a minute, 2026-10-07) pushed the
  Traefik on prod-02 past the former 256Mi and got it OOM-killed. Every ingress node needs the
  headroom, since any of them can take such a burst.
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
- **No Traefik plugins today.** If one is added, load it in local plugin mode (the plugin's
  source in the image or a volume) rather than downloaded at start: a failed download breaks
  every route that uses its middleware.
- **A Hetzner Load Balancer fits on top without changing Traefik**: targets by label over the
  private network, TCP 80 and 443 passed through (TLS stays at Traefik), health checks, PROXY
  protocol for client IPs, and one address (IPv4 and IPv6) in DNS instead of one per node. The
  firewall could then close 80/443 on the nodes' public side. With PROXY protocol trusted from
  the balancer (`proxyProtocol.trustedIPs` on the entry points), Traefik takes the original
  address as the client again, so `cloudflare-only` works unchanged.

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin -n kube-system get ds traefik -o wide
kubectl --context d3strukt0r-prod-admin get nodes                   # ROLES shows ingress
```
