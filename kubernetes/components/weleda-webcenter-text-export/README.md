# weleda-webcenter-text-export

Back to [`kubernetes/README.md`](../../README.md).

Turns Esko Web Center's GS1 XML exports into readable plain-text leaflets, at
`https://weleda-webcenter-text-export.d3strukt0r.dev` (`kubernetes/clusters/prod/weleda-webcenter-text-export.yaml`,
namespace `weleda-webcenter-text-export`). The app is its own repository,
`Team-MaRo/weleda-webcenter-text-export`, which builds and publishes the image; here is only how
it runs.

It ran as a Cloudflare Worker until 2026-11-24 and runs on the cluster since, like every other web
name. Its former name `webcenter-text-export.manuele-vaccari.ch` (a GitHub Pages site until then,
which no longer served it) redirects here (`redirect.yaml`, a temporary 302 with the path kept).

## What runs

- **One Deployment `web`**: the image's React Router server (`react-router-serve`) on port 3000,
  as the image's own user 65532, with a read-only root filesystem. With JavaScript on, the
  conversion happens in the visitor's browser and no file reaches the server; with JavaScript off,
  the server converts the one file sent to it and keeps nothing. No database, no secrets, no
  volume.
- **The image `d3strukt0r/weleda-webcenter-text-export:3.2`** from Docker Hub, rolled out by Keel
  (`components/keel`): it follows the 3.2 patch releases by itself; a new minor or major is a
  deliberate tag change here, after reading its changelog. On 2026-11-24 `3.2` was 3.2.3
  (2026-06-24, the repository's `master`). Its `develop` was 133 commits ahead and its image
  pipeline broken (`docker.yml`: `imagetools create` refuses `manifest:` annotations, and CI's
  Trivy scan fails), so no newer release reaches the cluster until the repository fixes that and
  releases again.
- **Memory from Kyverno's defaults** (32Mi request, 128Mi limit) until a few days of use are
  measured (the `resources` skill).
- **The Ingress** with `cloudflare-only` first, like every proxied name (charts/cloudflare-only);
  its certificate from cert-manager.

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin -n weleda-webcenter-text-export get deploy,pod,ingress,certificate
curl -sI https://weleda-webcenter-text-export.d3strukt0r.dev/    # 200
```

Gatus checks the page every five minutes ("Weleda WebCenter Text Export").
