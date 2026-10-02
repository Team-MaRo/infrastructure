# Argo CD

Back to [`kubernetes/README.md`](../../README.md).

Argo CD deploys everything under `kubernetes/` from `master`, itself included. This component
is Kustomize over the pinned upstream `install.yaml` plus patches (`patches/`: `argocd-cm`
with the Zitadel login, RBAC, plain HTTP, Dex removed, memory, the webhook secret) and the
Ingress for UI and API (`ingress.yaml`). What it needs from other components' CRDs is
[`../argocd-integrations/`](../argocd-integrations/README.md).

`ansible/argocd.yml` installs Argo CD **once**, from the admin's machine - the bootstrap is in
[`ansible/roles/argocd/README.md`](../../../ansible/roles/argocd/README.md). After that Argo CD
manages itself and every other component from `kubernetes/` on `master`. Never re-apply those
manifests from Ansible: a working copy that differs from `master` would fight Argo CD's
self-heal.

## Pushing to `master` is deploying

**Pushing to `master` is deploying**, pruning included. Argo CD watches this directory and
applies what it finds, pruning anything removed. There is no separate release step, which makes
branch protection on the repository a security control rather than a formality. The files must
also be pushed *before* the first bootstrap, because `root` syncs from GitHub, not from the
working copy.

## Pruning and the finalizer

**Pruning cascades only where an Application carries the finalizer**
`resources-finalizer.argocd.argoproj.io`. With it, deleting a file from `clusters/<name>/`
makes Argo CD delete everything that Application deployed; without it, `root` deletes only
the Application object and what it deployed keeps running, orphaned and no longer tracked.
Within one Application pruning works as usual either way: a manifest removed from a
component is deleted.

- **The finalizer is on apps whose data lives elsewhere or does not matter**: `gatus`,
  `zitadel`, `oauth2-proxy`, `phpmyadmin`, `pgadmin`, `kured`, `alloy`, `etcd-snapshots`,
  `cluster-rbac`, `argocd-integrations`. A Postgres app's database and role stay when it goes -
  CloudNativePG's `databaseReclaimPolicy` and `databaseRoleReclaimPolicy` default to `retain`;
  a MariaDB app's stay because its objects set `cleanupPolicy: Skip` (the operator's default is
  `Delete`).
- **Never on an app that brings CRDs** (deleting a CRD deletes every object of its kind -
  `cloudnative-pg` would take the Postgres `Cluster` with it): cert-manager,
  external-secrets, kyverno, cloudnative-pg, the mariadb-operator ones,
  system-upgrade-controller, kube-prometheus-stack, kubeelasti. **Never on an app with a volume**
  (openbao, mariadb, postgres, loki, kube-prometheus-stack): a mistakenly deleted file must
  not take its data along. **Never on the foundation**: root, argocd, hcloud-csi, traefik.
- **An app with the finalizer brings its own `namespace.yaml`** instead of `CreateNamespace`,
  since Argo CD never deletes a namespace it created that way; so the namespace, and
  anything a job left in it outside git, goes with the app. Apps in `kube-system` get no
  `namespace.yaml`, or removing them would delete `kube-system`.
- Helm's `helm.sh/resource-policy: keep` is honoured as `Delete=false`, so such objects
  survive even a cascading deletion. There are no Helm releases: Argo CD only renders charts
  with `helm template`; `helm list -A` shows just k3s's own Traefik.

## Clusters and components

**`kubernetes/clusters/<name>/` decides what runs where; `kubernetes/components/<app>/`
holds how**, shared by clusters. Each cluster directory is an app-of-apps: `root` syncs
the directory it lives in, so it manages itself. One Argo CD per cluster, each syncing
only its own directory, so no cluster can change another. A cluster that needs something
different gets a Kustomize overlay in its own directory, not a copy of the component.

## The installation

- **`kubernetes/components/argocd/` is Kustomize over the pinned upstream `install.yaml`**, the
  approach Argo CD's own docs use for self-management. The base is a raw single-file URL:
  the `github.com/...//manifests` form makes Kustomize shell out to `git`, which the nodes
  do not have. Upgrading is bumping that URL, **one minor at a time** - Argo CD does not
  support skipping minors.
- **Nothing in `components/argocd/` may need another component's CRDs**: the bootstrap applies
  it to a fresh cluster before cert-manager, External Secrets or the Prometheus operator exist.
  Argo CD's certificate, its webhook secret and its ServiceMonitors are therefore
  `components/argocd-integrations/`, an Application of their own (with the finalizer). The
  certificate sat in `components/argocd/` until 2026-09-30, which would have broken a rebuild.
- **Bootstrap and self-management use the same directory**, so the first self-sync is a
  no-op. Both apply server-side; `ServerSideApply=true` is required for a self-managed
  Argo CD because its CRDs are too large for client-side apply's annotation.
- **Dex is removed** by `patches/dex.yaml`; Zitadel is the OIDC provider and Argo CD talks
  to it directly (below). `argocd-server` still mounts an optional `argocd-dex-server-tls`
  secret volume from upstream - harmless, left alone rather than diverging from upstream.
- **Non-HA on purpose.** Even the HA manifest keeps the application controller - the part
  that syncs - at one replica, so HA mostly buys a UI that survives a node failure. That
  controller is a StatefulSet, which Kubernetes will not replace on an unreachable node:
  syncing stays stopped until the node returns or is tainted
  `node.kubernetes.io/out-of-service` (see "When a node dies" below).

## The UI and the Zitadel login

**The UI is at `https://argocd.d3strukt0r.dev`, logging in through Zitadel** (`oidc.config`
in `argocd-cm`, no Dex): two public clients from `tofu/zitadel/apps_argocd.tf`, the UI's and
the CLI's (`cliClientID`, redirect to `localhost:8085`), both PKCE without a client secret -
there is no secret to keep. Rights come from the `groups` claim: `infra-admin` is
`role:admin` (`argocd-rbac-cm`), everyone else gets nothing, and Zitadel's role check keeps
users without a role out anyway. TLS ends at Traefik; `server.insecure` makes argocd-server
speak plain HTTP behind it (read at start - a change needs a restart of argocd-server). The
CLI logs in with `argocd login argocd.d3strukt0r.dev --sso --grpc-web`: gRPC-web passes as
plain HTTPS, so no h2c route for native gRPC is configured. The local `admin` account and the
port-forward stay as the break-glass way in.

## Bootstrap

Argo CD cannot deploy itself the first time, so `ansible/argocd.yml` installs it once from
`components/argocd/` and applies `clusters/<cluster_name>/root.yaml`. It runs from your
machine with the admin kubeconfig, after `kubeconfig.yml`, from an address where 6443 is
open. From then on git is the only source, and the playbook finds Argo CD installed and
does nothing.

The files must be **pushed before the playbook runs**: `root` syncs its directory from
GitHub, not from your working copy.

The bootstrap applies exactly what the `argocd` Application syncs, so Argo CD's first sync
of itself is a no-op.

## Accessing the UI

**Argo CD** is at `https://argocd.d3strukt0r.dev` - "Log in via Zitadel" with the personal user
(it needs the role `infra-admin`). The CLI, once; `--grpc-web` is remembered:

```shell
argocd login argocd.d3strukt0r.dev --sso --grpc-web
```

Break-glass, with Zitadel down: the port-forward and the local `admin` (its password is in
1Password), on plain HTTP since TLS ends at Traefik:

```shell
kubectl --context d3strukt0r-prod-admin -n argocd port-forward svc/argocd-server 8080:80
```

then `http://localhost:8080`.

### After every fresh install: replace the admin password

Argo CD generates the `admin` password at install and keeps it in plain text in
`argocd-initial-admin-secret`. Replace it, in this order - deleting the secret first would
throw away the only copy before you have logged in:

1. Read the generated password:

   ```shell
   kubectl --context d3strukt0r-prod-admin -n argocd get secret argocd-initial-admin-secret \
     -o jsonpath='{.data.password}' | base64 -d; echo
   ```

2. Port-forward as above, log in as `admin`, and go to **User Info → Update Password**. (With
   the `argocd` CLI instead: `argocd login localhost:8080 --plaintext`, then
   `argocd account update-password`.)
3. Store the new password in 1Password.
4. Only now delete the secret that held the first one:

   ```shell
   kubectl --context d3strukt0r-prod-admin -n argocd delete secret argocd-initial-admin-secret
   ```

   It is not part of `install.yaml`, so Argo CD will not recreate it.

### Recovering a lost admin password

The local `admin` account is the break-glass login - it keeps working when SSO does not.
If its password is lost, have Argo CD generate a new one: remove the stored hash and
restart the server, which then writes a fresh `argocd-initial-admin-secret`.

```shell
kubectl --context d3strukt0r-prod-admin -n argocd patch secret argocd-secret --type json \
  -p '[{"op":"remove","path":"/data/admin.password"},{"op":"remove","path":"/data/admin.passwordMtime"}]'
kubectl --context d3strukt0r-prod-admin -n argocd rollout restart deployment argocd-server
```

Then continue with step 1 above. Setting a chosen password directly is also possible - put a
bcrypt hash (`argocd account bcrypt --password <password>`) into `admin.password` in
`argocd-secret` - but regenerating needs no extra tools.

Once SSO is set up and `admin` is disabled (`admin.enabled: "false"` in `argocd-cm`),
re-enable it before logging in - with a commit, not by editing `argocd-cm` by hand:
`argocd-cm` is part of the installation Argo CD manages itself, and self-heal would revert
a manual edit within minutes. Only if Argo CD is too broken to sync is a manual edit
enough, because then nothing reverts it.

## Upgrading Argo CD

Bump the version in the `install.yaml` URL in `components/argocd/kustomization.yaml` and
push. It upgrades every cluster that runs the shared component. **One minor version at a
time** - Argo CD does not support skipping minors, and each one's upgrade notes may require
a step. Patch releases within a minor need nothing special.

## How quickly a push arrives

At once: **a push arrives through a GitHub webhook** - GitHub sends every push to
`https://argocd.d3strukt0r.dev/api/webhook`, signed with the secret in 1Password
[`GitHub | infrastructure | Argo CD webhook`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=gknq7z4job2lgkcpzszvucaxay&h=my.1password.com) (OpenBao
`secret/argocd-webhook`), and Argo CD refreshes every Application using this repository.
`argocd-secret` carries only the reference `$argocd-webhook:github-secret`, and the Secret it
names needs the label `app.kubernetes.io/part-of: argocd` (it comes from
[`../argocd-integrations/`](../argocd-integrations/README.md)).
Polling at Argo CD's default (120 s plus up to 60 s of jitter) remains the fallback for a lost
delivery. The application controller and the repo server read the polling interval at start,
so after changing it in `patches/argocd-cm.yaml`, restart both once the change has synced:

```shell
kubectl --context d3strukt0r-prod-admin -n argocd rollout restart statefulset argocd-application-controller
kubectl --context d3strukt0r-prod-admin -n argocd rollout restart deployment argocd-repo-server
```

The webhook itself lives in GitHub, not in this repo (repository settings → Webhooks, or
`gh api repos/Team-MaRo/infrastructure/hooks`); its recent deliveries and their answers show
there too. Creating it again:

```shell
gh api repos/Team-MaRo/infrastructure/hooks --method POST -f name=web -F active=true -f 'events[]=push' \
  -f 'config[url]=https://argocd.d3strukt0r.dev/api/webhook' -f 'config[content_type]=json' -f 'config[insecure_ssl]=0' \
  -f "config[secret]=$(op item get 'GitHub | infrastructure | Argo CD webhook' --account my.1password.com --vault Private --fields password --reveal)" \
  --jq '.id'
```

## When Argo CD breaks itself

A bad commit to `components/argocd/` can break Argo CD, which is the thing that would
otherwise roll it back. Fix the commit, then apply the directory by hand from the admin
context:

```shell
kubectl --context d3strukt0r-prod-admin apply --server-side -k kubernetes/components/argocd
```

## When a sync is stuck

- **Automated sync retries a failed sync five times on the same commit**, so a fix pushed in
  between is not picked up until those retries are over. Terminate the running operation in
  the UI (or `argocd app terminate-op <app>`) and sync again.
- **Terminating an operation can leave hook Jobs behind** that still carry the finalizer
  `argocd.argoproj.io/hook-finalizer` and never go away. Remove it by hand:

```shell
kubectl --context d3strukt0r-prod-admin -n <namespace> patch job <job> --type=json -p '[{"op":"remove","path":"/metadata/finalizers"}]'
```

## When a node dies

The install is not HA. If a node disappears, Argo CD's Deployments move after about five
minutes, but the application controller is a StatefulSet, and Kubernetes does not replace
a StatefulSet pod on a node it cannot reach. Syncing stays stopped until the node comes
back, or until you declare it gone:

```shell
kubectl --context d3strukt0r-prod-admin taint node <node> node.kubernetes.io/out-of-service=nodeshutdown:NoExecute
```

Running workloads are unaffected either way; only deploying changes stops.
