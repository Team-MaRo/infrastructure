# kubernetes

What runs on the cluster, deployed by Argo CD from this directory on `master`.

**Pushing to `master` is deploying.** Argo CD watches this directory and applies what it
finds, pruning anything removed. There is no separate release step, which makes branch
protection on the repository a security control rather than a formality.

## Layout

```
kubernetes/
├── clusters/                  # what runs where - one directory per cluster
│   └── prod/
│       ├── root.yaml          # syncs this directory - itself and every sibling
│       ├── argocd.yaml        # Argo CD managing its own installation
│       ├── hcloud-csi.yaml    # persistent volumes
│       ├── openbao.yaml       # the secret store: Helm chart pinned here, values in components/
│       └── external-secrets.yaml  # delivers OpenBao values as Kubernetes Secrets
└── components/                # how each component is deployed, shared by clusters
    ├── argocd/                # pinned upstream install.yaml plus patches
    │   ├── kustomization.yaml
    │   ├── namespace.yaml
    │   └── patches/dex.yaml   # removes the bundled Dex
    ├── hcloud-csi/            # Hetzner's CSI driver, pinned
    │   ├── kustomization.yaml
    │   └── patches/reclaim-retain.yaml
    ├── openbao/
    │   └── values.yaml        # Helm values: one replica, Raft storage, static seal
    └── external-secrets/
        ├── kustomization.yaml
        └── cluster-secret-store.yaml  # the store named openbao
```

Components are Kustomize over a pinned upstream manifest where upstream publishes one.
Where upstream ships only a Helm chart (OpenBao), the cluster's Application pins the chart
and reads its values from `components/<app>/values.yaml` through a second source.

The split is **what** against **how**. `clusters/<name>/` decides which components a
cluster runs; `components/<app>/` holds the manifests, written once and shared. Each
cluster runs its own Argo CD, which syncs only its own directory, so no cluster can change
another.

Each cluster directory is an app-of-apps: `root` syncs the directory it lives in, so it
manages itself as well as its siblings.

- **Adding a component** means a directory under `components/` and one Application file
  in each cluster directory that should run it.
- **When one cluster needs something different**, give it a small Kustomize overlay under
  its own directory that references the component, rather than copying the component.
- **Adding a cluster** means a new `clusters/<name>/` with its own `root.yaml` and
  `argocd.yaml`, and `cluster_name` set for its inventory group. See `CLAUDE.md` for what
  else in Ansible and OpenTofu still assumes a single cluster.

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

Not exposed yet. Exposing it needs TLS, which needs cert-manager. Until then:

```shell
kubectl --context d3strukt0r-prod-admin -n argocd port-forward svc/argocd-server 8080:443
```

then `https://localhost:8080`; expect a self-signed certificate warning.

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
   the `argocd` CLI instead: `argocd login localhost:8080 --insecure`, then
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

## OpenBao

The secret store. One replica with Raft storage on a Hetzner Volume, unsealed on every start
by a static seal key from Secret `openbao/openbao-seal` - not in git; `ansible/secrets.yml`
writes it from 1Password. OpenBao's own configuration (secrets engines, auth, policies) is
`tofu/openbao` - see [`../tofu/README.md`](../tofu/README.md).

### First install

1. Push; Argo CD creates the namespace and the StatefulSet. The pod waits for its Secret.
2. `cd ansible && ansible-playbook secrets.yml` writes `openbao-seal`; the pod starts.
3. Initialise it, **once, ever** for this storage:

   ```shell
   kubectl --context d3strukt0r-prod-admin -n openbao exec -ti openbao-0 -- bao operator init
   ```

   It prints recovery keys and the initial root token, **once**. Store all of them in
   1Password before closing the terminal. With an auto-unseal like the static seal there are
   no unseal keys; the recovery keys are for break-glass operations such as generating a new
   root token.
4. `kubectl ... -n openbao exec openbao-0 -- bao status` shows `Initialized true`,
   `Sealed false`. Deleting the pod proves the seal: it comes back unsealed by itself.

**The seal key is the one thing that must never be lost.** Without it, the data on the
volume - and every snapshot - is unreadable. It lives in 1Password; rotating it means adding
the new key alongside the old one (`previous_key`), never replacing it.

### Reaching it from this machine

```shell
kubectl --context d3strukt0r-prod-admin -n openbao port-forward svc/openbao 8200:8200
export BAO_ADDR=http://127.0.0.1:8200   # in the shell that runs bao
bao status
```

The UI is then at `http://127.0.0.1:8200/ui`.

## External Secrets

Workloads never talk to OpenBao. External Secrets reads values from it and writes ordinary
Kubernetes Secrets, through one `ClusterSecretStore` named `openbao` - OpenBao's `secret/`
engine, logged into with the operator's own service account (role `external-secrets` in
`tofu/openbao`).

A value goes into OpenBao by hand (see `tofu/README.md` for the port-forward and token), and
an `ExternalSecret` next to the workload pulls it in:

```yaml
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: example
  namespace: example
spec:
  secretStoreRef:
    kind: ClusterSecretStore
    name: openbao
  target:
    name: example            # the Kubernetes Secret that gets created
  data:
    - secretKey: password    # key in the Kubernetes Secret
      remoteRef:
        key: example         # path under secret/, i.e. secret/example
        property: password   # field of that OpenBao secret
```

The store may read all of `secret/`, so every namespace that references it reaches every
value - fine while one admin runs the cluster.

## How quickly a push arrives

Argo CD checks git every 60 seconds (`patches/argocd-cm.yaml`; upstream is 120 s plus up to
60 s of jitter). The application controller and the repo server read that interval at start,
so after changing it, restart both once the change has synced:

```shell
kubectl --context d3strukt0r-prod-admin -n argocd rollout restart statefulset argocd-application-controller
kubectl --context d3strukt0r-prod-admin -n argocd rollout restart deployment argocd-repo-server
```

A GitHub webhook would make syncs immediate, once `argocd-server` is reachable from the
internet.

## When Argo CD breaks itself

A bad commit to `components/argocd/` can break Argo CD, which is the thing that would
otherwise roll it back. Fix the commit, then apply the directory by hand from the admin
context:

```shell
kubectl --context d3strukt0r-prod-admin apply --server-side -k kubernetes/components/argocd
```

## Storage

A PersistentVolumeClaim becomes a Hetzner Cloud Volume through Hetzner's CSI driver
(`components/hcloud-csi/`), with `hcloud-volumes` as the default StorageClass. There is no
node-local storage: k3s's local-path provisioner is disabled.

A volume is not on a node. Hetzner keeps it on three physical servers and attaches it over
the network to one node at a time; when a pod moves, the driver moves the volume with it.
So a volume is `ReadWriteOnce` - it cannot be shared by pods on different nodes - and a
server holds at most 16. Each is at least 10 GB and stays in nbg1.

**Deleting a PVC does not delete the volume.** The class uses `reclaimPolicy: Retain`,
because Hetzner keeps no backups of volumes and Argo CD prunes whatever leaves git. The PV
turns `Released`, and the volume stays - and is billed - until removed by hand:

```shell
kubectl --context d3strukt0r-prod-admin get pv                     # STATUS Released
kubectl --context d3strukt0r-prod-admin get pv <pv> -o jsonpath='{.spec.csi.volumeHandle}'; echo
kubectl --context d3strukt0r-prod-admin delete pv <pv>
hcloud volume delete <volume id>
```

The driver needs Secret `kube-system/hcloud`, which is not in git: `ansible/secrets.yml`
writes it from 1Password. Upgrading is bumping the version in the URL in
`components/hcloud-csi/kustomization.yaml`.

## When a node dies

The install is not HA. If a node disappears, Argo CD's Deployments move after about five
minutes, but the application controller is a StatefulSet, and Kubernetes does not replace
a StatefulSet pod on a node it cannot reach. Syncing stays stopped until the node comes
back, or until you declare it gone:

```shell
kubectl --context d3strukt0r-prod-admin taint node <node> node.kubernetes.io/out-of-service=nodeshutdown:NoExecute
```

Running workloads are unaffected either way; only deploying changes stops.
