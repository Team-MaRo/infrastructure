# Domain redirects

Back to [`kubernetes/README.md`](../../README.md).

Whole domains that send their visitors to another one (`kubernetes/clusters/prod/domain-redirects.yaml`,
plain manifests in this directory, namespace `domain-redirects`). Traefik answers the redirect
itself, so there are no pods.

| From | To |
|---|---|
| `d3st.dev`, `d3st.org`, `d3strukt0r.me` | `https://d3strukt0r.dev`, path and query kept |

Only the apexes: their wildcards (`*.d3st.dev`, ...) reach the cluster too and still get Traefik's
404 ([`tofu/cloudflare/README.md`](../../../tofu/cloudflare/README.md), "Names and what serves them").

## Design

- **Temporary (302)**, not permanent: browsers keep a 301 forever, and `d3st.dev` may serve
  something of its own later (`middleware-redirect.yaml`).
- **A route per entry point** (`ingress-route.yaml`): an IngressRoute's `tls` covers all its entry
  points, so plain HTTP needs its own. Both are needed while a zone's SSL mode is `flexible`, where
  Cloudflare sends even HTTPS visitors to the origin over HTTP.
- **A certificate of its own** (`certificate.yaml`, cert-manager, DNS-01 with the personal
  account's token): before, these names got Traefik's self-signed default certificate, which kept
  Cloudflare's automatic SSL/TLS from moving their zones off `flexible`. With a valid certificate
  its next scan (about monthly) should pick `full` or `strict`; `tofu output ssl_modes` in
  `tofu/cloudflare` shows the current mode.
- **The route's service is `noop@internal`**, Traefik's own empty service: the redirect answers
  before it is reached.
- **`cloudflare-only` first**, like every proxied route (charts/cloudflare-only).

## Adding a domain

Point its apex at the cluster (`tofu/cloudflare`, a proxied CNAME to `prod.d3strukt0r.dev`), add
the name to `dnsNames` in `certificate.yaml` and to the `match` of both routes - or, for another
target, a middleware and routes of its own.

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin -n domain-redirects get certificate,ingressroute,middleware
curl -sI https://d3st.dev/some/path?x=1    # 302, location: https://d3strukt0r.dev/some/path?x=1
curl -sI http://d3strukt0r.me/             # 302 to https://d3strukt0r.dev/
```
