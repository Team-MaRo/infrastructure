# Spegel

Back to [`kubernetes/README.md`](../../README.md).

Spegel lets a node fetch image layers from the other nodes instead of from zot or the internet:
a DaemonSet that reads each node's containerd content store and answers registry requests for
what it holds. Its Helm chart comes from `oci://ghcr.io/spegel-org/helm-charts/spegel` (the
only place Spegel publishes it; an `oci://` source needs no repository Secret while the registry
is public) with a patch range in `kubernetes/clusters/prod/spegel.yaml`; values here in
`values.yaml`. Since 2026-11-19, replacing k3s's built-in Spegel, which froze floating tags
(`ansible/roles/k3s/README.md`, "Pulling through zot").

## How a pull goes

containerd tries each mirror endpoint in turn and moves on at a refused connection, a 404 or any
other error. For the six registries Kyverno allows (our own: below), `registries.yaml`
(`k3s_registries` in `ansible/roles/k3s/defaults/main.yml`) lists:

1. **The node's own Spegel**, `http://<the node's private IP>:30020` - answers manifests and
   layers **by digest** when another node has them (a miss costs about 20 ms,
   `mirror-resolve-timeout`).
2. **zot**, `registry.d3strukt0r.dev` - every tag, checked against upstream on each request.
3. **The origin registry**, which k3s always adds last.

So a pull of `wordpress:7-php8.5-fpm` asks Spegel for the tag (404), zot for the tag (the current
digest from Docker Hub), then Spegel for each layer by digest - from a neighbour if one has it,
otherwise from zot.

**The endpoint lives in k3s's file, not Spegel's.** The chart's init container would write
containerd's `certs.d` itself (`containerdMirrorAdd`), but k3s regenerates that directory from
`registries.yaml` on every start and would drop Spegel's entries until its pod restarts. So it is
off, and the k3s role lists Spegel. k3s applies each mirror's `rewrite` to every endpoint, so
Spegel sees `docker.io/library/...` paths - harmless, it looks content up by digest alone.

The address is the node's private IP on NodePort 30020, the only way the chart exposes the
registry (no hostPort), as Spegel does itself; `localhost` NodePorts are a legacy iptables-only
feature of kube-proxy. The public firewall does not open 30020.

## Our own images

`registry.d3strukt0r.dev` takes part too: its mirror in `registries.yaml` lists only Spegel, and
k3s adds zot itself last. Tags come from zot as for every registry; while zot is down, a node
still gets a layer by digest from a neighbour, but cannot resolve a tag it has not got.

**Spegel asks for no login.** It serves whatever a node's containerd holds to anyone who reaches
it, so any pod in the cluster can read our images by digest at `<node IP>:30020` without zot's
credentials (it needs the digest, it cannot list images). Accepted while our images hold no
secrets; an image that does must not run on the cluster, or this needs a NetworkPolicy first.

## Tags never

**Every tag reference gets a 404** (`registryFilters: [":[^/]+$"]`), so tags always come from zot
and a floating tag always resolves to the image it points to now. `resolveTags: false` alone does
not do that in v0.8.1: only the init container (off, above) reads it; the registry itself still
answers tags from whichever node has one (`pkg/registry/registry.go`). The filter runs before
any lookup and is matched against `<registry>/<repository>[:<tag>]` - the digest is never part
of that string (`pkg/oci/filter.go`), so `:[^/]+$` catches exactly the requests by tag, while
manifests and layers by digest pass. `resolveTags: false` stays set so the intent is visible.

## Values that differ from the chart

| Value | Why |
|---|---|
| `containerdMirrorAdd: false` | k3s writes the endpoint (above) |
| `containerdSock`, `containerdContentPath`, `containerdRegistryConfigPath` | k3s's containerd lives under `/run/k3s` and `/var/lib/rancher/k3s`, not the stock paths |
| `mirroredRegistries` | the six registries Kyverno allows and our own, the same as `k3s_registries` |
| `registryFilters`, `resolveTags: false` | tags never (above) |
| `debugWebEnabled: false` | its debug pages are on by default |

Memory stays at the chart's 128Mi request and limit until measured (the sizing rule in
`AGENTS.md`); Spegel's `GOMEMLIMIT` follows the limit.

## What it needs

- **Root and host paths** (containerd's socket and content store): the namespace is on
  `k3s_psa_exempt_namespaces` (`ansible/roles/k3s/defaults/main.yml`).
- **`system-node-critical`** (the chart's priority class), so it is the last thing evicted from a
  node under pressure.
- **The finalizer** on its Application: nothing it holds matters - removed, containerd finds the
  endpoint refused and pulls from zot.

## Order for a fresh cluster or a removal

The endpoint and Spegel can come in either order: before Spegel runs, a refused connection sends
containerd straight to zot. The PSA exemption must be in place first, or the DaemonSet's pods are
refused - so `prod.yml` runs before Argo CD deploys it. Removing it: delete
`kubernetes/clusters/prod/spegel.yaml`, then drop `k3s_spegel_url` from the mirrors and `spegel`
from the exemptions and run `prod.yml`.

## Checking it

On a node (`ansible prod -b -m shell -a '...'` from `ansible/`):

```sh
# Spegel first, then zot
cat /var/lib/rancher/k3s/agent/etc/containerd/certs.d/docker.io/hosts.toml
# a tag: 404
curl -s -o /dev/null -w '%{http_code}\n' -H 'Accept: application/vnd.oci.image.index.v1+json' \
  "http://$(hostname -I | tr ' ' '\n' | grep '^10\.0\.0\.'):30020/v2/library/wordpress/manifests/7-php8.5-fpm?ns=docker.io"
# a digest another node has: 200
curl -s -o /dev/null -w '%{http_code}\n' -H 'Accept: application/vnd.oci.image.index.v1+json' \
  "http://$(hostname -I | tr ' ' '\n' | grep '^10\.0\.0\.'):30020/v2/library/wordpress/manifests/sha256:<digest>?ns=docker.io"
```

Its metrics (PodMonitor `spegel`, per `node`): `spegel_mirror_requests_total` counts what a node
asked its peers for and whether one answered.

## Open bugs

- **#1568** - Spegel exits when its first peer does not answer within 30 s; the DaemonSet restarts
  it, and meanwhile pulls go to zot.
- **#735** - a pull by digest is answered regardless of the repository named; harmless, the
  content is verified by its digest.
- **#1530** - registry filters do not limit what a node advertises to its peers, only what it
  answers; harmless here, the filter is about answering.
