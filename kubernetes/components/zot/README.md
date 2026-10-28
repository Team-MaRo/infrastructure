# zot

Back to [`kubernetes/README.md`](../../README.md).

The container registry at `registry.d3strukt0r.dev` (`kubernetes/clusters/prod/zot.yaml`, the zot
Helm chart pinned there with `values.yaml` and this directory's manifests, namespace `zot`). It
holds our own images - only here, never pushed to ghcr.io or Docker Hub - and mirrors the
registries Kyverno allows (`docker.io`, `ghcr.io`, `quay.io`, `registry.k8s.io`,
`public.ecr.aws`).

## Why zot

One Go program with logins, per-repository rights, a mirror of several registries and its own
garbage collection, no database. Distribution (`registry:2`) has no real login model, needs one
instance per mirrored registry and collects garbage only offline; Harbor needs about ten
containers, PostgreSQL and Redis - too much for four small nodes (research 2026-10-28).

## Design

- **One replica, nothing on the pod**: images in the bucket `d3strukt0r-prod-zot`
  (`tofu/objectstorage`), users and API keys in [`valkey-state`](../valkey-state/README.md)
  database 4 (`remoteCache` + `cacheDriver` redis), so a new pod anywhere has everything. zot's
  scale-out mode gives each repository one owner instance, so more replicas would not make it
  highly available; the nodes' embedded registry is the plan for that (Ansible, k3s role).
- **No deduplication** (`dedupe: false`): with it on S3, garbage collection deleted freshly pushed
  layers (zot#4550, fixed after v2.1.22). The bucket is versioned for 14 days, so a layer that
  garbage collection, a bug or a mistake deletes can be restored ("Restoring a deleted layer").
- **Docker-format manifests are accepted** (`http.compat: ["docker2s2"]`): without it a push from
  Docker's classic builder gets 415, and the mirror would convert Docker Hub's images to OCI and
  change their digests, so images pinned by digest would not be found.
- **DNS-only**: `registry.d3strukt0r.dev` is a CNAME to `direct.prod.d3strukt0r.dev`, the nodes'
  own addresses (`tofu/cloudflare`). Through Cloudflare's proxy a request may carry at most
  100 MB and a push sends each layer in one request - the official PHP image's base layer is
  118 MB. So the Ingress (`ingress.yaml`) has no `cloudflare-only`, carries the label
  `d3strukt0r.dev/dns-only: "true"` for Kyverno's guard, and a rate limit of its own
  (`middleware-rate-limit.yaml`: 100 requests a second per connection address, bursts of 500 -
  a pull asks for every layer in parallel). Only `/v2` and `/zot` are routed; `/metrics` stays
  inside the cluster.
- **Restricted Pod Security**: the image runs as root; here as 10001 with a read-only root, `/tmp`
  and its root directory as emptyDirs (tested 2026-10-28).
- **Configuration in git**: `external-secrets.yaml` renders `config.json` from a template, with
  only zot's Valkey password and the htpasswd file from OpenBao; the S3 key reaches zot's driver
  as `AWS_ACCESS_KEY_ID`/`AWS_SECRET_ACCESS_KEY`. Reloader restarts zot when either changes.

## Logins

Every request needs one; nothing is anonymous except `/metrics` (in-cluster only). zot answers
every client with a Bearer challenge, so a client exchanges its credential at
`/zot/auth/token`:

| Who | Credential | May |
|---|---|---|
| GitHub Actions of `Team-MaRo` and `D3strukt0r` | the workflow's OIDC token (audience `registry.d3strukt0r.dev`, `permissions: id-token: write`) - no secret stored at GitHub | read and push `team-maro/**`, `d3strukt0r/**` |
| the nodes (user `cluster`) | an API key | read everything, the mirror included |
| `admin` | an API key | everything, deleting included |

`admin` and `cluster` are htpasswd users (hashes in `secret/zot`), but zot refuses their
passwords at the token endpoint once GitHub's tokens are on - tested 2026-10-28. The password
only creates API keys ("Making an API key"). Keys live in Valkey and survive restarts; if
valkey-state's data is lost, they are gone and must be made again.

## Mirror

zot fetches from the five registries on demand, under a prefix per registry:
`registry.d3strukt0r.dev/docker.io/library/busybox` is `docker.io/library/busybox`. The nodes are
pointed at it in `/etc/rancher/k3s/registries.yaml` (Ansible, k3s role) and fall back to the
origin registry when zot fails. Docker Hub limits anonymous pulls per address; zot's cache keeps
that low.

## Secrets

| OpenBao | Fields | 1Password |
|---|---|---|
| `secret/zot` | `htpasswd` (two lines, bcrypt) | [`Zot \| Prod \| admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=oflhq5kcafpo5np4aj2csrvfqq&h=my.1password.com), [`Zot \| Prod \| cluster`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=od76biljijf6a7umrlojyd45ya&h=my.1password.com) (the passwords) |
| `secret/zot-s3` | `access-key`, `secret-key` | [`Hetzner \| S3 \| prod zot`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=iiiac6kigz57va4kxpfk2c2tc4&h=my.1password.com) |
| `secret/valkey-apps/zot` | `password` | [`Valkey \| Prod \| zot`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=233swwvse6eihstd2sxh222xsu&h=my.1password.com) |

## Runbooks

### Making an API key

With the user's password, once; the key is shown only in the answer:

```shell
op item get 'Zot | Prod | cluster' --account my.1password.com --fields password --reveal \
  | curl -fsS -u "cluster:$(cat)" -X POST -H 'Content-Type: application/json' \
      -d '{"label":"nodes"}' https://registry.d3strukt0r.dev/zot/auth/apikey | jq -r .apiKey
```

Store it in the user's 1Password item (field `api-key`). `docker login registry.d3strukt0r.dev -u
cluster` takes it as the password.

### Rotating a key

Make a new one as above, roll it out (the nodes: Ansible), then remove the old one:
`curl -u cluster:<password> -X DELETE 'https://registry.d3strukt0r.dev/zot/auth/apikey?id=<uuid>'`
(`GET /zot/auth/apikey` lists them with their ids).

### Restoring a deleted layer

The bucket keeps deleted objects for 14 days as old versions. List them for the blob's digest and
copy the last version back over the current key (admin S3 key, profile `d3strukt0r-hetzner`):

```shell
aws --profile d3strukt0r-hetzner --endpoint-url https://nbg1.your-objectstorage.com s3api list-object-versions --bucket d3strukt0r-prod-zot --prefix <path to the blob>
aws --profile d3strukt0r-hetzner --endpoint-url https://nbg1.your-objectstorage.com s3api copy-object --bucket d3strukt0r-prod-zot --key <key> --copy-source 'd3strukt0r-prod-zot/<key>?versionId=<id>'
```

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin -n zot get pods,externalsecrets,ingress
kubectl --context d3strukt0r-prod-admin -n zot logs deploy/zot
curl -sI https://registry.d3strukt0r.dev/v2/    # 401 with a Bearer challenge
```

Gatus checks `/v2/` for the 401; `ZotDown` (`rules.yaml`) fires when Prometheus cannot scrape it.
