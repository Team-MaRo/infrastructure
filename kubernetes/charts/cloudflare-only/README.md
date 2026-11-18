# cloudflare-only

Back to [`kubernetes/README.md`](../../README.md).

The Traefik Middleware `cloudflare-only` (`files/middleware.yaml`), a `chain` of three:
`cloudflare-allowlist`, an `ipAllowList` of Cloudflare's IP ranges, `cloudflare-rate-limit`, 20
requests a second per visitor by default (bursts of 100, grouped by `CF-Connecting-IP`; an app
whose genuine use needs more raises its own, "Raising the limit for one app"), and `upload-scan`,
which hands every uploaded file to clamd and refuses what it does not call clean. Every route of a
name that Cloudflare proxies puts it first, so only Cloudflare reaches those sites - anyone
talking to a node directly gets 403 from Traefik -, a single visitor cannot flood one, and no app
gets a file clamd flags - without setting anything up itself. The
rate limit was on Traefik's entry point before (2026-10-07, a scanner's burst); it moved here
because DNS-only routes share that port without a `CF-Connecting-IP`, so their clients would all
have counted as one visitor.

The upload scan's Middleware sits in the same file (Traefik's plugin, its settings:
[`../../components/traefik/README.md`](../../components/traefik/README.md), "Upload scan"), so
every app gets a copy with the same settings - the scan's memory is sized by the first copy
Traefik builds, and switching it to report mode is one line for all. It comes last in the chain:
a client that is not Cloudflare, or is over its rate, costs no buffering. On the routes behind
the Zitadel gate (oauth2-proxy) it runs before the login, like on any public form; the
per-address limits and the scan's memory bound what an anonymous client can make it hold.

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
  ([`../wordpress/README.md`](../wordpress/README.md)). `templates/middleware.yaml` reads the
  file (`.Files.Get`) document by document, so `rateLimit` (`values.yaml`) can change the rate
  limit's numbers; everything else comes out as the file has it, without its comments. Argo CD finds no `charts/` directory and runs `helm dependency build`
  itself (it registers only `https`/`oci` repositories; a `file://` one is left to Helm); locally,
  `helm dependency build kubernetes/charts/<chart>` first.
- **A Kustomize directory** lists `../../charts/cloudflare-only/files` under `resources:` (its
  `kustomization.yaml` names the same file): traefik, argocd, gatus, matomo, pgadmin, phpmyadmin,
  wedding-manuele-robine, and the Kustomize sources of zitadel, openbao, oauth2-proxy,
  kube-prometheus-stack, goldilocks.
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

## Raising the limit for one app

The rate limit is there to stop traffic that is not genuine - a scanner's hundreds to thousands
of requests a minute -, never normal use, heavy use included. The numbers in
`files/middleware.yaml` are the default; an app whose genuine use they refuse raises its own
copy, and only its own: each namespace's copy is its own bucket. Today `monitoring` (Grafana:
average 50, burst 300) and `wedding-manuele-robine` (the gallery's thumbnails: average 50, burst
300). Both are sized so the heaviest measured genuine use takes about 80 % of the burst.

- **When**: people using the app get 429 ("Too Many Requests"). Traefik's access log keeps every
  429 with the visitor's `CF-Connecting-IP`; one from a real user's address is the sign, a
  scanner's (probing `.env`, `wp-config.txt`, ...) is the limit working.
- **How much**: the burst above the largest genuine spike (everything a page or dashboard sends
  at once), the average above sustained use, a few times over. The access log keeps only errors
  and slow requests, and Prometheus' 30 s scrapes hide what happens within a second; Traefik's
  spans in Tempo have every request to the microsecond. Take a heavy session of real use, fetch
  its spans (Tempo's search API, TraceQL `{ resource.service.name = "traefik" &&
  span.server.address = "<host>" }`, `limit=5000`, `spss=1`), count them per second and replay
  their start times against a token bucket (`average` tokens a second, at most `burst`, one per
  request): the chosen numbers refuse nothing and leave the bucket well above empty. The skill
  `rate-limits` (`.claude/skills/rate-limits/`) does all of it, for one namespace or every one.
- **Where**: a Kustomize directory patches its copy in its `kustomization.yaml`, with the
  measurement next to it (`components/kube-prometheus-stack`):

  ```yaml
  patches:
    - target:
        kind: Middleware
        name: cloudflare-rate-limit
      patch: |-
        - op: replace
          path: /spec/rateLimit/average
          value: 100
        - op: replace
          path: /spec/rateLimit/burst
          value: 500
  ```

  A chart with the subchart sets `cloudflare-only.rateLimit` (`average`, `burst`) in the app's
  values - for a WordPress site in its `components/<site>/values.yaml`.
- It covers every route of the namespace: an app sharing its namespace with a public one raises
  the public one's too.

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
