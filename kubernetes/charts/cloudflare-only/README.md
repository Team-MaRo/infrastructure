# cloudflare-only

Back to [`kubernetes/README.md`](../../README.md).

The Traefik Middleware `cloudflare-only` (`files/middleware.yaml`), a `chain` of two:
`cloudflare-allowlist`, an `ipAllowList` of Cloudflare's IP ranges, and `cloudflare-rate-limit`,
20 requests a second per visitor (bursts of 100, grouped by `CF-Connecting-IP`). Every route of a
name that Cloudflare proxies puts it first, so only Cloudflare reaches those sites - anyone
talking to a node directly gets 403 from Traefik - and a single visitor cannot flood one. The
rate limit was on Traefik's entry point before (2026-10-07, a scanner's burst); it moved here
because DNS-only routes share that port without a `CF-Connecting-IP`, so their clients would all
have counted as one visitor.

## Why

DNS-only names - the registry, later Forgejo's web and SSH, a game server - point at the nodes'
own addresses, on their default ports. Once those addresses are public, every proxied site could
be reached around Cloudflare, and the headers the cluster trusts from Cloudflare
(`CF-Connecting-IP` in Traefik's rate limit, the sites' nginx and the xmlrpc allowlist) could be
set by anyone. The Hetzner firewall cannot tell the two kinds apart - it sees ports, not
hostnames, and the registry shares 443 - so Traefik does it per route
([`../../components/traefik/README.md`](../../components/traefik/README.md), "Only through
Cloudflare").

It proves "from Cloudflare", not "from our account": another Cloudflare customer could point a
zone at a node. Cloudflare's per-zone client certificates (authenticated origin pulls) would
close that; not needed for these sites.

## How it is used

Traefik takes a route's middlewares only from the route's own namespace (`allowCrossNamespace`
stays off, [`../../components/oauth2-proxy/README.md`](../../components/oauth2-proxy/README.md)),
so every app with a proxied route gets its own copy, all from the one file `files/middleware.yaml`
(no namespace; Argo CD puts each copy into the app's namespace):

- **A chart of ours** imports this chart as a subchart (`dependencies` in its `Chart.yaml`,
  `repository: file://../cloudflare-only`), so its objects come with every release - the
  WordPress chart does, switched by `ingress.cloudflareOnly`
  ([`../wordpress/README.md`](../wordpress/README.md)). `templates/middleware.yaml` only reads
  the file (`.Files.Get`). Argo CD finds no `charts/` directory and runs `helm dependency build`
  itself (it registers only `https`/`oci` repositories; a `file://` one is left to Helm); locally,
  `helm dependency build kubernetes/charts/<chart>` first.
- **A Kustomize directory** lists `../../charts/cloudflare-only/files` under `resources:` (its
  `kustomization.yaml` names the same file): traefik, argocd, gatus, matomo, pgadmin, phpmyadmin,
  wedding-manuele-robine, and the Kustomize sources of zitadel, openbao, oauth2-proxy,
  kube-prometheus-stack.
- **The route** names it first: `traefik.ingress.kubernetes.io/router.middlewares:
  <namespace>_cloudflare-only@kubernetescrd[,<its others>]` on an Ingress (a chart's ingress
  annotations in its values), `middlewares: [{name: cloudflare-only}]` on an IngressRoute.
- **A DNS-only route** leaves it out, carries the label `d3strukt0r.dev/dns-only: "true"` and
  brings a rate limit of its own, grouped by the connection's address (there it is the real
  client) - without one it has no cap at all;
  Kyverno refuses any other route without the middleware first
  ([`../../components/kyverno/README.md`](../../components/kyverno/README.md), "Routes only
  through Cloudflare").

A route that names it in a namespace without the copy is switched off by Traefik (404, "middleware
... does not exist" in its log): it fails closed.

## When Cloudflare's ranges change

Gatus checks `https://api.cloudflare.com/client/v4/ips` every hour against the etag the list in
`files/middleware.yaml` was taken from (check "Cloudflare IP ranges", group Cluster,
[`../../components/gatus/config.yaml`](../../components/gatus/config.yaml)). Red means new or
removed ranges - visitors arriving from a new one get 403:

```shell
curl -fsS https://api.cloudflare.com/client/v4/ips | jq '.result | {etag, ipv4_cidrs, ipv6_cidrs}'
```

Update `sourceRange` in `files/middleware.yaml`, the date and etag in its comment, and the etag in the
Gatus check, in one commit.

## Checking it

Through Cloudflare a site answers as usual; straight to a node it answers 403:

```shell
curl -sS -o /dev/null -w '%{http_code}\n' https://www.arepazo.ch/
curl -sS -o /dev/null -w '%{http_code}\n' --resolve www.arepazo.ch:443:<node IPv4> https://www.arepazo.ch/
```

Which copies exist: `kubectl --context d3strukt0r-prod-admin get middlewares.traefik.io -A | grep cloudflare-only`.
