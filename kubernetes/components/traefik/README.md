# Traefik

Back to [`kubernetes/README.md`](../../README.md).

Traffic enters through Traefik, which k3s ships. Traefik is k3s's own - k3s installs and
upgrades it from its bundled chart - and is configured only through the `HelmChartConfig` in
`kubernetes/components/traefik/` (`helmchartconfig.yaml`, Application `traefik`), next to the
dashboard's certificate (`certificate.yaml`), its Zitadel gate
(`middleware-oauth2-proxy.yaml`) and the PodMonitor for its metrics (`pod-monitor.yaml`). It
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
  (`k3s_node_labels`, today all three). **That label, the DNS map `local.prod_nodes` in
  `tofu/cloudflare` and later a load balancer's targets must list the same nodes** - a node in
  DNS without Traefik answers nothing. A workload-only node gets no label: no Traefik, not
  internet-facing. There is no standard ratio; keep at least two or three for redundancy and
  size them by traffic, not by node count.
- **Ports below 1024 without root**: Traefik runs as UID 65532 without capabilities;
  `net.ipv4.ip_unprivileged_port_start = 80` on the nodes (`k3s_sysctls`) lets it bind 80 and
  443. Its other entrypoints, metrics `:9100` and `:8080`, also open on the host but are closed by
  the Hetzner firewall. (node-exporter therefore listens on 9101, not its default 9100.)
- **Its metrics** are scraped through a PodMonitor (`components/traefik/pod-monitor.yaml`, port
  `metrics` 9100 on the host network), which carries `SkipDryRunOnMissingResource=true`, since
  the CRD belongs to another Application.
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
- **A Hetzner Load Balancer fits on top without changing Traefik**: targets by label over the
  private network, TCP 80 and 443 passed through (TLS stays at Traefik), health checks, PROXY
  protocol for client IPs, and one address (IPv4 and IPv6) in DNS instead of one per node. The
  firewall could then close 80/443 on the nodes' public side.

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin -n kube-system get ds traefik -o wide
kubectl --context d3strukt0r-prod-admin get nodes                   # ROLES shows ingress
```
