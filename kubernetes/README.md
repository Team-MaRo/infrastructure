# kubernetes

What runs on the cluster, deployed by Argo CD from this directory on `master`.

**Pushing to `master` is deploying.** Argo CD watches this directory and applies what it
finds, pruning anything removed. There is no separate release step, which makes branch
protection on the repository a security control rather than a formality.

Deleting a file from `clusters/<name>/` removes what that Application deployed only if it
carries the finalizer `resources-finalizer.argocd.argoproj.io` - apps whose data lives elsewhere
(Gatus, Zitadel, oauth2-proxy, ...), with their own `namespace.yaml`. Apps with CRDs or volumes
(the operators, OpenBao, the databases, Loki, Prometheus) deliberately have none: deleting their
file removes only the Application, and what it deployed keeps running untracked. Which app has
it, and why, is in `CLAUDE.md`.

## Layout

```
kubernetes/
├── clusters/                  # what runs where - one directory per cluster
│   └── prod/
│       ├── root.yaml          # syncs this directory - itself and every sibling
│       ├── argocd.yaml        # Argo CD managing its own installation
│       ├── argocd-integrations.yaml  # its certificate, webhook secret and metrics monitors
│       ├── hcloud-csi.yaml    # persistent volumes
│       ├── openbao.yaml       # the secret store: Helm chart pinned here, values in components/
│       ├── external-secrets.yaml  # delivers OpenBao values as Kubernetes Secrets
│       ├── system-upgrade-controller.yaml  # upgrades k3s on the nodes
│       ├── kured.yaml             # reboots nodes after kernel updates
│       ├── kyverno.yaml           # policies: chart pinned here, values and policies in components/
│       ├── cert-manager.yaml      # certificates from Let's Encrypt
│       ├── traefik.yaml           # k3s's Traefik: DaemonSet on the ingress nodes' host network
│       ├── gatus.yaml             # the public status page and the alerting heartbeat
│       ├── kube-prometheus-stack.yaml  # metrics and alerts: chart pinned here, values in components/
│       ├── loki.yaml              # log storage: chart pinned here, values in components/
│       ├── alloy.yaml             # log collection on every node: chart pinned here
│       ├── mariadb-operator-crds.yaml  # the operator's CRDs - never pruned
│       ├── mariadb-operator.yaml  # the operator running the app database
│       ├── mariadb.yaml           # the app database itself
│       ├── cloudnative-pg.yaml    # the PostgreSQL operator and its backup plugin: charts pinned here
│       ├── postgres.yaml          # the shared PostgreSQL itself
│       ├── zitadel.yaml           # the identity provider (SSO)
│       ├── oauth2-proxy.yaml      # the Zitadel login gate for UIs without a login of their own
│       ├── kubeelasti.yaml        # scale to zero: chart pinned here, values in components/
│       ├── cluster-rbac.yaml      # cluster-wide rights for kubectl logins through Zitadel
│       ├── etcd-snapshots.yaml    # syncs the subdirectory below
│       └── etcd-snapshots/        # prod-only: the S3 settings k3s uploads snapshots with
└── components/                # how each component is deployed, shared by clusters
    ├── argocd/                # pinned upstream install.yaml plus patches
    │   ├── kustomization.yaml
    │   ├── namespace.yaml
    │   ├── ingress.yaml       # UI and API behind Traefik
    │   └── patches/           # argocd-cm (Zitadel login), RBAC, plain HTTP, Dex removed, memory, webhook secret
    ├── argocd-integrations/   # what Argo CD needs from other components' CRDs
    │   ├── kustomization.yaml
    │   ├── certificate.yaml   # argocd.d3strukt0r.dev
    │   └── external-secrets.yaml  # the GitHub webhook secret
    ├── hcloud-csi/            # Hetzner's CSI driver, pinned
    │   ├── kustomization.yaml
    │   └── patches/           # reclaimPolicy Retain, memory per container
    ├── openbao/
    │   ├── values.yaml        # Helm values: one replica, Raft storage, static seal, Ingress
    │   ├── kustomization.yaml
    │   └── certificates.yaml  # openbao.d3strukt0r.dev
    ├── external-secrets/
    │   ├── values.yaml            # Helm values: memory
    │   ├── kustomization.yaml
    │   └── cluster-secret-store.yaml  # the store named openbao
    ├── traefik/
    │   ├── kustomization.yaml
    │   ├── helmchartconfig.yaml   # values merged into k3s's bundled Traefik, the dashboard
    │   ├── certificate.yaml       # traefik.d3strukt0r.dev
    │   └── middleware-oauth2-proxy.yaml  # the Zitadel gate for the dashboard
    ├── cert-manager/
    │   ├── kustomization.yaml     # pinned release manifest
    │   ├── external-secret.yaml   # the two Cloudflare tokens from OpenBao
    │   └── cluster-issuers.yaml   # letsencrypt-staging and letsencrypt
    ├── kured/
    │   └── kustomization.yaml     # pinned release manifest plus the reboot window
    ├── kyverno/
    │   ├── values.yaml            # Helm values
    │   ├── kustomization.yaml
    │   ├── default-resources.yaml # LimitRange for every app namespace
    │   ├── allowed-registries.yaml # images only from known registries
    │   └── rbac-limitranges.yaml  # lets Kyverno create LimitRanges
    ├── system-upgrade-controller/
    │   ├── kustomization.yaml     # pinned release manifests
    │   └── plan.yaml              # which k3s version, when, one node at a time
    ├── kube-prometheus-stack/
    │   ├── values.yaml            # Helm values: k3s scrape targets, retention, ntfy routing
    │   ├── kustomization.yaml
    │   ├── external-secrets.yaml  # ntfy URLs and token, Grafana's admin, from OpenBao
    │   ├── scrape-etcd.yaml       # etcd's metrics on the servers' private IPs
    │   ├── rules.yaml             # this cluster's own alerts
    │   └── certificates.yaml      # grafana.d3strukt0r.dev
    ├── loki/
    │   ├── values.yaml            # Helm values: one instance, S3, 30 days
    │   ├── kustomization.yaml
    │   └── external-secret.yaml   # its S3 key from OpenBao
    ├── mariadb-operator/
    │   └── values.yaml            # Helm values: securityContexts, cert-manager, images
    ├── mariadb/
    │   ├── kustomization.yaml
    │   ├── mariadb.yaml           # primary + replica, the failover settings
    │   ├── backups.yaml           # daily physical backup, binary log archiving, replica rebuild
    │   ├── flush-binlogs.yaml     # closes the active binary log every 10 min, for archiving
    │   ├── backup-after-switchover.yaml  # a backup after every switch of the primary
    │   ├── external-secrets.yaml  # root/replication passwords and the S3 key from OpenBao
    │   ├── s3-ca.yaml             # the roots Hetzner's S3 certificate chains to
    │   └── rules.yaml             # alerts: no ready primary, broken or lagging replication
    ├── cloudnative-pg/
    │   ├── values.yaml            # Helm values: two replicas, memory, monitoring
    │   └── plugin-values.yaml     # the Barman Cloud plugin: quick move off a dead node
    ├── postgres/
    │   ├── kustomization.yaml
    │   ├── cluster.yaml           # primary + replica, synchronous replication
    │   ├── backups.yaml           # the object store, WAL archiving, the nightly base backup
    │   ├── external-secrets.yaml  # the S3 key from OpenBao
    │   ├── pod-monitor.yaml       # the instances' metrics
    │   └── rules.yaml             # alerts: no ready primary, replication, archiving, backups
    ├── zitadel/
    │   ├── values.yaml            # Helm values: domain, database, first admin, securityContexts
    │   ├── kustomization.yaml
    │   ├── database.yaml          # its role and database, in the postgres namespace
    │   ├── external-secrets.yaml  # masterkey, first admin password, database password
    │   ├── postgres-ca.yaml       # copies CloudNativePG's CA certificate for verify-full
    │   ├── certificates.yaml      # auth.d3strukt0r.dev, and the login page's key pair
    │   └── groups-webhook.yaml    # adds the flat `groups` claim to Zitadel's tokens
    ├── oauth2-proxy/
    │   ├── values.yaml            # Helm values: Zitadel, allowed group, cookie domain
    │   ├── kustomization.yaml
    │   ├── external-secrets.yaml  # client and cookie secret from OpenBao
    │   └── certificates.yaml      # oauth2-proxy.d3strukt0r.dev
    ├── kubeelasti/
    │   └── values.yaml            # Helm values: Prometheus address, queue, timeout
    ├── cluster-rbac/
    │   ├── kustomization.yaml
    │   └── infra-admin.yaml       # Zitadel's infra-admin group is cluster-admin
    ├── alloy/
    │   ├── values.yaml            # Helm values: the collection pipeline
    │   ├── kustomization.yaml
    │   └── clusterrole.yaml       # only what the pipeline reads
    └── gatus/
        ├── kustomization.yaml     # config.yaml becomes the ConfigMap gatus
        ├── config.yaml            # what is checked, the Watchdog heartbeat, ntfy
        ├── namespace.yaml
        ├── deployment.yaml        # one replica, pinned image, read-only
        ├── database.yaml          # its role and database, in the postgres namespace
        ├── postgres-ca.yaml       # copies CloudNativePG's CA certificate for verify-full
        ├── external-secrets.yaml  # ntfy, the heartbeat token, the database password
        ├── service.yaml
        ├── certificate.yaml       # status.d3strukt0r.dev
        └── ingress.yaml           # public, no login
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

## kubectl

Every day through Zitadel, context `d3strukt0r-prod` (written by `ansible/kubeconfig.yml`, with
kubelogin installed): the first command opens the browser, then the ID token lasts an hour and
renews itself without the browser - for 90 days after the login, or until 30 days unused.

```shell
kubectl --context d3strukt0r-prod auth whoami   # zitadel:<username>, group zitadel:infra-admin
```

The role `infra-admin` in Zitadel's project `Infrastructure` makes that user cluster admin
(`components/cluster-rbac/`). Break-glass, with Zitadel down, is context
`d3strukt0r-prod-admin`, the k3s admin certificate. The commands in this README use it; the
Zitadel context works for all of them as well, except where Zitadel itself is broken.

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

**Grafana** is at `https://grafana.d3strukt0r.dev` - "Sign in with Zitadel", the same way; its
local `admin` (password in 1Password [`Grafana | Prod | Admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=25tqaghtajnc3efoop2ve6jfze&h=my.1password.com)) and the port-forward below stay
as break-glass.

The **Traefik dashboard** is at `https://traefik.d3strukt0r.dev`, behind the Zitadel gate
(oauth2-proxy, see "The login gate"). The **status page** is public at
`https://status.d3strukt0r.dev` (see "Status page"). Grafana, Prometheus and Alertmanager by
port-forward:

```shell
kubectl --context d3strukt0r-prod-admin -n monitoring port-forward svc/kube-prometheus-stack-grafana 3000:80
kubectl --context d3strukt0r-prod-admin -n monitoring port-forward svc/kube-prometheus-stack-prometheus 9090:9090
kubectl --context d3strukt0r-prod-admin -n monitoring port-forward svc/kube-prometheus-stack-alertmanager 9093:9093
```

then `http://localhost:3000` (break-glass: user `admin`, password in 1Password [`Grafana | Prod | Admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=25tqaghtajnc3efoop2ve6jfze&h=my.1password.com)),
`http://localhost:9090` (Status → Targets shows every scrape) and `http://localhost:9093`
(firing alerts, silences).

**OpenBao** is at `https://openbao.d3strukt0r.dev` - method OIDC, role empty (the default),
"Sign in with OIDC Provider". The CLI - `-no-print`, or `bao login` prints the new token, a
valid admin token, to the terminal:

```shell
BAO_ADDR=https://openbao.d3strukt0r.dev bao login -method=oidc -no-print
```

Break-glass: the root token (1Password [`OpenBao | Prod | Recovery keys & root token`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=6ftev2p3fo3dc457whshzgn6jy&h=my.1password.com)), in the
UI's Token method or through the port-forward in "OpenBao".

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

### Backups

Every 6 hours the CronJob `openbao-snapshot` saves a Raft snapshot to
`d3strukt0r-prod-openbao-snapshots` and deletes those older than 30 days (the bucket keeps a
deleted one 7 more days). Its S3 key, before the first run:

1. Hetzner console → Object Storage → S3 credentials, label `prod openbao snapshots`. Copy the
   secret key and store both in 1Password (the access key is not secret):

   ```shell
   op item create --account my.1password.com --vault Private --category 'API Credential' \
     --title 'Hetzner | S3 | prod openbao snapshots' \
     "username=<access key>" "credential=$(pbpaste)" >/dev/null
   ```

2. Into OpenBao, and the access key ID into `tofu/objectstorage/terraform.tfvars`
   (`openbao_snapshots_access_key_id`):

   ```shell
   jq -n --arg a "$(op item get 'Hetzner | S3 | prod openbao snapshots' --account my.1password.com --vault Private --fields username)" \
         --arg s "$(op item get 'Hetzner | S3 | prod openbao snapshots' --account my.1password.com --vault Private --fields credential --reveal)" \
     'if ($a|length)!=20 or ($s|length)==0 then error("access key must be 20 characters, secret non-empty - 1Password lookup failed?") else {"access-key":$a,"secret-key":$s} end' \
   | bao kv put secret/openbao-snapshot-s3 -
   ```

A snapshot by hand, and what is there:

```shell
kubectl --context d3strukt0r-prod-admin -n openbao create job --from=cronjob/openbao-snapshot openbao-snapshot-manual
kubectl --context d3strukt0r-prod-admin -n openbao logs job/openbao-snapshot-manual
aws --profile d3strukt0r-hetzner s3 ls s3://d3strukt0r-prod-openbao-snapshots/
kubectl --context d3strukt0r-prod-admin -n openbao delete job openbao-snapshot-manual
```

### Restoring

A restore needs the static seal key and its ID `1` (1Password [`OpenBao | Prod | Seal key`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=n3blscroul4eancakippscoyhq&h=my.1password.com)), a
snapshot, and an initialised, unsealed OpenBao to restore into - on a lost volume, a fresh one:
push, `ansible-playbook secrets.yml`, `bao operator init` as in "First install" (keep its output
only until the restore is done). Then, with the new root token:

```shell
aws --profile d3strukt0r-hetzner s3 cp s3://d3strukt0r-prod-openbao-snapshots/<newest>.snapshot /tmp/
kubectl --context d3strukt0r-prod-admin -n openbao cp /tmp/<newest>.snapshot openbao-0:/tmp/restore.snapshot
kubectl --context d3strukt0r-prod-admin -n openbao exec openbao-0 -- \
  env BAO_TOKEN=<new root token> bao operator raft snapshot restore /tmp/restore.snapshot
```

The restore replaces everything, the new root token and recovery keys included: from then on
the original root token and recovery keys in 1Password apply again. Check with
`bao kv list secret/` and `bao auth list`, logged in with the original root token.

**The drill** (done 2026-09-30) runs the same restore in Docker on this machine, without
touching the cluster: download a snapshot, write the seal key to a file with `printf '%s'`,
start `quay.io/openbao/openbao:2.6.3` with a config of `storage "raft"` and the same
`seal "static"` block (`current_key_id = "1"`), `bao operator init`, restore with the new root
token, then list secrets with the original one. Delete the container and the key file
afterwards.

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
  data:
    - secretKey: password    # key in the Kubernetes Secret
      remoteRef:
        key: example         # path under secret/, i.e. secret/example
        property: password   # field of that OpenBao secret
```

The Kubernetes Secret it creates gets the ExternalSecret's name; `target.name` is set only
where the two must differ.

The store may read all of `secret/`, so every namespace that references it reaches every
value - fine while one admin runs the cluster.

## etcd snapshots

k3s uploads its twice-daily etcd snapshots to the bucket `d3strukt0r-prod-etcd` with the
cluster's own S3 key. The ExternalSecret in `clusters/prod/etcd-snapshots/` builds Secret
`kube-system/k3s-etcd-snapshot-s3-config` from it; `k3s_config` in Ansible points k3s at
that Secret.

Setting it up, or replacing the key:

1. Create an S3 key in the Hetzner console, labelled `prod etcd snapshots`, and store it in
   1Password as [`Hetzner | S3 | prod etcd snapshots`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=w2po7jqmxren7y7eruj73p5w4a&h=my.1password.com) (access key as username, secret key as
   credential).
2. With the port-forward to OpenBao open and `BAO_TOKEN` set (see `tofu/README.md`), copy it
   over - the values never appear in shell history:

   ```shell
   bao kv put secret/etcd-snapshot-s3 \
     access-key="$(op item get 'Hetzner | S3 | prod etcd snapshots' --account my.1password.com --vault Private --fields username)" \
     secret-key="$(op item get 'Hetzner | S3 | prod etcd snapshots' --account my.1password.com --vault Private --fields credential --reveal)"
   ```
3. External Secrets refreshes the Secret within its interval; k3s reads it at the next
   snapshot.

Taking one by hand and listing them:

```shell
ssh prod-01 sudo k3s etcd-snapshot save
ssh prod-01 sudo k3s etcd-snapshot ls
kubectl --context d3strukt0r-prod-admin get etcdsnapshotfiles
```

The bucket lists only the newest 5 snapshots of all nodes together - about a day. Older ones,
up to 7 days, are noncurrent versions; fetch one with the admin key and restore from the file
instead of from S3 (`--cluster-reset-restore-path=<local file>`, without the `--etcd-s3*`
flags):

```shell
aws --profile d3strukt0r-hetzner s3api list-object-versions --bucket d3strukt0r-prod-etcd --prefix etcd-snapshot-prod-01
aws --profile d3strukt0r-hetzner s3api get-object --bucket d3strukt0r-prod-etcd --key <key> --version-id <id> <local file>
```

**A restore cannot read the Secret**, since the apiserver is down. It needs the S3 settings
as flags and the server token from 1Password ([`k3s | Prod | Server token`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=2eic3puykugnvm6tsdh2jf4nvy&h=my.1password.com)); see the
[k3s docs](https://docs.k3s.io/cli/etcd-snapshot) for the full procedure:

```shell
k3s server --cluster-reset --cluster-reset-restore-path=<snapshot name> \
  --token=<server token> --etcd-s3 --etcd-s3-endpoint=nbg1.your-objectstorage.com \
  --etcd-s3-region=nbg1 --etcd-s3-bucket=d3strukt0r-prod-etcd \
  --etcd-s3-access-key=<admin key> --etcd-s3-secret-key=<admin secret>
```

## k3s upgrades

The system-upgrade-controller upgrades k3s on every node by itself, following the channel
in `components/system-upgrade-controller/plan.yaml`: one node at a time, cordoned, daily
between 02:30 and 03:30 Zurich time - before the nodes' OS updates at 03:30.

```shell
kubectl --context d3strukt0r-prod-admin -n system-upgrade get plan server -o wide   # the version it aims for
kubectl --context d3strukt0r-prod-admin -n system-upgrade get jobs                 # one per node and upgrade
kubectl --context d3strukt0r-prod-admin get nodes                                  # versions, SchedulingDisabled
```

A node left `SchedulingDisabled` after a failed Job: read the Job's log, fix the cause, then
`kubectl uncordon <node>`.

The channel is `v1.37` until `stable` reaches 1.37, then `stable`:

```shell
curl -s https://update.k3s.io/v1-release/channels | jq -r '.data[] | select(.id=="stable") | .latest'
```

Until the switch, moving to the next minor is changing the channel to the next one - one
minor at a time, after checking that the Hetzner CSI driver supports it.

## Reboots

kured reboots a node when a kernel update asks for it (`/var/run/reboot-required`), one
node at a time and only between 04:30 and 06:00 Zurich time: cordon, drain, reboot,
uncordon.

```shell
kubectl --context d3strukt0r-prod-admin -n kube-system logs -l name=kured --prefix   # all three nodes
```

To test it, or to have a node rebooted in the next window without a kernel update, create
the file yourself:

```shell
ssh prod-03 sudo touch /var/run/reboot-required
```

`/var/run` is a tmpfs, so the file disappears with the reboot.

## Default resources

Kyverno gives every namespace a LimitRange `default-resources`. A container that sets no
`resources` gets requests of 50m CPU, 64Mi memory and 50Mi ephemeral storage, and limits of
128Mi memory and 1Gi ephemeral storage - no CPU limit. Setting `resources` on a container
overrides any of them; there is no maximum.

```shell
kubectl --context d3strukt0r-prod-admin get limitrange -A
kubectl --context d3strukt0r-prod-admin get generatingpolicy default-resources
```

Only `kube-system`, `system-upgrade`, `kube-public` and `kube-node-lease` are left out
(`components/kyverno/default-resources.yaml`); there, every container sets its memory
itself. `kyverno` gets none either - Kyverno ignores its own namespace - but its chart sets
all resources. A new infrastructure component needs no entry here - only its own
`resources` where the defaults do not fit, and a place in `k3s_psa_exempt_namespaces` in
Ansible if it needs more than the restricted standard below allows.

## What an app pod must look like

Every app namespace enforces the **restricted** Pod Security Standard (configured in k3s,
see `CLAUDE.md`). A pod that breaks it is rejected when it is created, and the error lists
what is missing. Each container needs at least:

```yaml
securityContext:
  runAsNonRoot: true             # the image must not run as root, or set runAsUser
  allowPrivilegeEscalation: false
  capabilities:
    drop: [ALL]
  seccompProfile:
    type: RuntimeDefault         # may also sit once in the pod's securityContext
```

Host namespaces, host paths and privileged mode are not allowed at all. Many official images
run as root by default; set `runAsUser` to a non-zero ID or use the image's rootless
variant.

Images must come from `docker.io`, `ghcr.io`, `quay.io`, `registry.k8s.io` or
`public.ecr.aws` (Kyverno policy `allowed-registries`); anything else is refused when the
Deployment - or whichever controller - is applied. A new registry is one entry in
`components/kyverno/allowed-registries.yaml`.

## Ingress

Traffic enters through Traefik, which k3s ships. It runs once on every node labelled
`node-role.kubernetes.io/ingress=true` (all three today), on the node's own network, and
listens on 80 and 443 over IPv4 and IPv6. `prod.d3strukt0r.dev` has an A and an AAAA record
per ingress node; a service on the cluster gets a CNAME to it and an `Ingress` (class
`traefik`, the default).

```shell
kubectl --context d3strukt0r-prod-admin -n kube-system get ds traefik -o wide
kubectl --context d3strukt0r-prod-admin get nodes                   # ROLES shows ingress
```

Its settings are the `HelmChartConfig` in `components/traefik/`; k3s itself installs and
upgrades Traefik.

## Certificates

cert-manager gets certificates from Let's Encrypt, proving each name through a temporary
DNS record in Cloudflare - no DNS preparation, and the name need not be reachable. Two
ClusterIssuers: `letsencrypt-staging` (untrusted certificates, for trying) and
`letsencrypt`. A certificate is requested with a `Certificate` next to the workload:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example
  namespace: example
spec:
  secretName: example-tls        # the Secret with tls.crt and tls.key
  dnsNames: [example.d3strukt0r.dev]
  issuerRef:
    kind: ClusterIssuer
    name: letsencrypt
```

```shell
kubectl --context d3strukt0r-prod-admin get certificate -A
kubectl --context d3strukt0r-prod-admin describe challenge -A   # while one hangs
```

The issuers use one Cloudflare token per account. Creating or replacing them:

1. Cloudflare dashboard → My Profile → API Tokens → Create Token, permissions
   `Zone → DNS → Edit` and `Zone → Zone → Read`, zone resources "All zones from an
   account". Once in the personal login, once in the arepazo login.
2. Store them in 1Password as [`Cloudflare | cert-manager DNS (prod cluster)`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=gb6fdq5kggtselhlpywmmlvake&h=my.1password.com) and
   [`Cloudflare | Arepazo | cert-manager DNS (prod cluster)`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=vnc4tluvl3iy35fxkkavvszkwy&h=my.1password.com) (field `credential`).
3. With the port-forward to OpenBao open and `BAO_TOKEN` set:

   ```shell
   bao kv put secret/cloudflare-dns \
     personal="$(op item get 'Cloudflare | cert-manager DNS (prod cluster)' --account my.1password.com --vault Private --fields credential --reveal)" \
     arepazo="$(op item get 'Cloudflare | Arepazo | cert-manager DNS (prod cluster)' --account my.1password.com --vault Private --fields credential --reveal)"
   ```

## Status page

Gatus, public at `https://status.d3strukt0r.dev`: it checks the services in
`components/gatus/config.yaml` and watches the alerting pipeline itself - Alertmanager sends its
always-firing `Watchdog` alert to Gatus every two minutes, and when none has come for 5 minutes,
Gatus alerts through ntfy. There is no admin UI and no login: **adding or changing a check is
editing `config.yaml` and pushing**; Gatus reloads it within about a minute, without a restart.
A check is one entry:

```yaml
endpoints:
  - name: Zitadel
    group: Cluster
    url: https://auth.d3strukt0r.dev/debug/healthz
    interval: 1m
    conditions:
      - "[STATUS] == 200"
    alerts:
      - type: ntfy
```

An invalid file makes Gatus exit and the pod crash-loop; its last log shows why. A literal `$`
in the file is written `$$`. Check what it sees:

```shell
kubectl --context d3strukt0r-prod-admin -n gatus logs deploy/gatus --previous   # after a crash
curl -s https://status.d3strukt0r.dev/api/v1/endpoints/statuses | jq -c '.[] | {key, last: [.results[-3:][] | .success]}'
```

**Before the first sync**, ntfy and two secrets of its own must be in OpenBao.

Alerts go to one ntfy.sh topic, whose name is the secret: on the free plan a topic cannot be
reserved, so anyone who knows the name could read it.

1. **ntfy**, once for all monitoring (Gatus and Alertmanager share it):
   - Create the topic name with `echo "prod-alerts-$(openssl rand -hex 16)"` - the prefix says
     what it is, the 128 random bits make it unguessable.
   - Create an access token at ntfy.sh → Account → Access tokens, named `prod cluster alerts`,
     **never expiring** (an expiring token would silently stop the alerts).
   - Store both in 1Password as [`ntfy | prod alerts`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=htj7nc5vbwet6osr3mfdm3b5yy&h=my.1password.com) (topic as `username`, token as
     `credential`), and subscribe to the topic in the ntfy Android app.
   - Copy them into OpenBao (port-forward to OpenBao open):

     ```shell
     BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN="$(op item get 'OpenBao | Prod | Recovery keys & root token' --account my.1password.com --vault Private --fields credential --reveal)" \
       bao kv put secret/ntfy \
         topic="$(op item get 'ntfy | prod alerts' --account my.1password.com --vault Private --fields username)" \
         token="$(op item get 'ntfy | prod alerts' --account my.1password.com --vault Private --fields credential --reveal)"
     ```

2. **The heartbeat token and the database password**, letters and digits only: Gatus expands
   every `$` in its config, and the password sits inside a `postgres://` URL.

   ```shell
   op item create --account my.1password.com --vault Private --category password --title 'Gatus | Prod | Watchdog token' \
     --generate-password='letters,digits,32' >/dev/null
   op item create --account my.1password.com --vault Private --category password --title 'PostgreSQL | Prod | gatus' \
     --generate-password='letters,digits,20' >/dev/null
   jq -n --arg t "$(op item get 'Gatus | Prod | Watchdog token' --account my.1password.com --vault Private --fields password --reveal)" \
     'if ($t|length)!=32 then error("watchdog token must be 32 characters - 1Password lookup failed?") else {"watchdog-token":$t} end' \
   | bao kv put secret/gatus -
   jq -n --arg p "$(op item get 'PostgreSQL | Prod | gatus' --account my.1password.com --vault Private --fields password --reveal)" \
     'if ($p|length)!=20 then error("password must be 20 characters - 1Password lookup failed?") else {"password":$p} end' \
   | bao kv put secret/postgres-apps/gatus -
   ```

   (logged in with `bao login -method=oidc -no-print`; on a fresh OpenBao, with the root token
   and port-forward as above).

On the first start the pod may restart a few times until CloudNativePG has created its
database. Until Alertmanager sends its first heartbeat, Gatus reports the Watchdog as down after
5 minutes - a free test of the alert path.

## Metrics and alerts

kube-prometheus-stack: Prometheus collects metrics for 30 days, Grafana shows them, and
Alertmanager sends alerts to the phone through the ntfy topic from "Status page" -
`critical` at priority 5, `warning` at 3, `info` only in Grafana. It needs, before its first
sync, `secret/ntfy` and `secret/gatus` (see "Status page") and Grafana's admin password:

1. Create a password item [`Grafana | Prod | Admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=25tqaghtajnc3efoop2ve6jfze&h=my.1password.com) in 1Password (username `admin`).
2. With the port-forward to OpenBao open:

   ```shell
   jq -n --arg p "$(op item get 'Grafana | Prod | Admin' --account my.1password.com --vault Private --fields password --reveal)" \
     'if ($p|length)==0 then error("empty value - 1Password lookup failed") else {"admin-password":$p} end' \
   | BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN="$(op item get 'OpenBao | Prod | Recovery keys & root token' --account my.1password.com --vault Private --fields credential --reveal)" \
     bao kv put secret/grafana -
   ```

   JSON on stdin rather than `key=value`: a generated password may start with `@`, and a
   failed lookup would otherwise store an empty value (see "OpenBao" in `tofu/README.md`).

After the first sync, the Watchdog on the status page turns green within two minutes, and
Gatus sends a "resolved" message if it had reported it down. While Gatus is unreachable,
Alertmanager reports the failed heartbeats as `AlertmanagerFailedToSendAlerts` after a few
minutes.

**When an alert arrives**, its message says what fired and where. The cluster's own alerts
(`components/kube-prometheus-stack/rules.yaml`) carry what to do in their description; the
chart's are explained in the [runbooks](https://runbooks.prometheus-operator.dev/), which
each alert links as `runbook_url`. A known cause being worked on can be silenced in
Alertmanager's UI (Silences → New Silence), for a fixed time.

**A volume alert** (80 %, 90 %, 95 %) is answered by growing the volume: raise the PVC's
storage request - in the values or manifest that defines it, then push - and Hetzner resizes
it while it stays mounted. A volume can never shrink; going smaller means a new volume and
losing its data.

**Testing the path to the phone** - an alert that resolves itself after five minutes:

```shell
kubectl --context d3strukt0r-prod-admin -n monitoring exec alertmanager-kube-prometheus-stack-alertmanager-0 -c alertmanager -- \
  amtool alert add TestAlert severity=warning --annotation=summary="Test from the README" \
  --end="$(date -u -v+5M +%Y-%m-%dT%H:%M:%SZ)" --alertmanager.url=http://localhost:9093
```

## Logs

Every container's log, every node's journal and the cluster's events end up in Loki for 30
days, searchable in Grafana → Explore → datasource **Loki**. A few queries to start from:

```
{namespace="gatus"}                                        # one namespace's containers
{namespace="monitoring", container="prometheus"} |= "error"  # lines containing "error"
{job="node-journal", unit="k3s.service", node="prod-01"}   # k3s's own log on one node
{job="node-journal", unit="ssh.service"}                   # SSH logins
{job="loki.source.kubernetes_events"}                      # events: scheduling, OOM kills, pulls
```

Logs of pods that no longer exist stay searchable - `kubectl logs` only reaches running ones.

**If Loki cannot write to Object Storage**, new logs still arrive but pile up on its volume,
and its log says so:

```shell
kubectl --context d3strukt0r-prod-admin -n loki logs loki-0 | grep -i -E 'error|denied|failed to flush'
```

A `403` right after the key was created is Hetzner's propagation delay; anything persistent
is the key in OpenBao `secret/loki-s3` or the bucket policy in `tofu/objectstorage/loki.tf`.

## MariaDB

The app database: a primary and a replica (mariadb-operator), backed up every night and with
its binary logs archived every ten minutes, so it can be restored to any moment in the last
30 days. Apps connect to **`mariadb-primary.mariadb.svc:3306`**.

**Before the first sync**, its passwords and S3 key go into OpenBao (port-forward and
`BAO_TOKEN` as in "OpenBao"):

1. A 1Password item [`MariaDB | Prod | Root & replication`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=uxnuz66uu5e2jojwk5xqq3tpgi&h=my.1password.com) with two generated password
   fields, `root-password` and `repl-password`.
2. As JSON on stdin (generated passwords may start with `@`; an empty lookup must not be
   stored - see "OpenBao" in `tofu/README.md`):

   ```shell
   jq -n --arg r "$(op item get 'MariaDB | Prod | Root & replication' --account my.1password.com --vault Private --fields root-password --reveal)" \
         --arg p "$(op item get 'MariaDB | Prod | Root & replication' --account my.1password.com --vault Private --fields repl-password --reveal)" \
     'if ($r|length)==0 or ($p|length)==0 then error("empty value - 1Password lookup failed") else {"root-password":$r,"repl-password":$p} end' \
   | BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN="$(op item get 'OpenBao | Prod | Recovery keys & root token' --account my.1password.com --vault Private --fields credential --reveal)" \
     bao kv put secret/mariadb -
   ```

   **If an empty or wrong root password reached the cluster first**, the operator keeps its
   own copy in Secret `internal-mariadb` and loops on "Access denied" trying to rotate it
   from there. Copy the real value over:
   `kubectl --context d3strukt0r-prod-admin -n mariadb patch secret internal-mariadb --type=json -p "[{\"op\":\"replace\",\"path\":\"/data/root-password\",\"value\":\"$(kubectl --context d3strukt0r-prod-admin -n mariadb get secret mariadb -o jsonpath='{.data.root-password}')\"}]"`
3. The S3 key (`secret/mariadb-backups-s3`) - see "Object Storage" in `tofu/README.md`.

Changing a password in OpenBao later does not change it in MariaDB.

```shell
kubectl --context d3strukt0r-prod-admin -n mariadb get mariadb,pods,physicalbackup,pitr
kubectl --context d3strukt0r-prod-admin -n mariadb get mariadb mariadb -o jsonpath='{.status.replication}' | jq
```

### MariaDB: failover hangs

`MariaDBNoReadyPrimary` means apps cannot write. Most failovers finish on their own within a
couple of minutes; the known case that does not is a primary whose node died hard
(mariadb-operator#1628) - the operator waits for the old primary until its node returns.

A primary that restarts once, about a minute, right after the **replica's** node came back from
a hard failure is a known MariaDB bug (CLAUDE.md, "App database: MariaDB"); it heals itself,
nothing to do.

1. Look: `kubectl --context d3strukt0r-prod-admin -n mariadb get mariadb mariadb` (status
   column) and `get pods -o wide` - which pod was primary, on which node, is that node
   `NotReady`?
2. If the node comes back soon (a reboot), wait: the failover completes, or the old primary
   simply returns.
3. If the node is gone for good, tell Kubernetes so - its pods are then deleted and their
   volumes released:
   ```shell
   kubectl --context d3strukt0r-prod-admin taint nodes <node> node.kubernetes.io/out-of-service=nodeshutdown:NoExecute
   ```
4. If the operator still does not promote the replica, name it primary yourself (`0` or `1`,
   the replica's pod number):
   ```shell
   kubectl --context d3strukt0r-prod-admin -n mariadb patch mariadb mariadb --type=merge -p '{"spec":{"replication":{"primary":{"podIndex":1}}}}'
   ```
   Argo CD does not undo this - the MariaDB in git sets no `podIndex`.
5. Remove the taint once the node is back or replaced:
   `kubectl --context d3strukt0r-prod-admin taint nodes <node> node.kubernetes.io/out-of-service-`.

**After every switch of the primary** point-in-time recovery cannot go past the old primary's
last upload until a backup from the new primary exists. The CronJob
`mariadb-backup-after-switchover` starts one within 15 minutes; its log says what it decided
(`kubectl --context d3strukt0r-prod-admin -n mariadb logs job/<latest job>`). To start one by
hand - also when `MariaDBBackupFailed` fires:

```shell
kubectl --context d3strukt0r-prod-admin -n mariadb patch physicalbackup mariadb-daily --type=merge -p "{\"spec\":{\"schedule\":{\"onDemand\":\"$(date +%s)\"}}}"
```

### MariaDB: binary log archiving stuck

`MariaDBBinlogArchivingFailing` fires, the MariaDB reports Ready=False with `Error archiving
binlogs: ... error getting binary log mariadb-bin.NNNNNN metadata`, and `kubectl get pitr`
shows no or an old `LAST RECOVERABLE TIME`. The archiver uploads the files in order and stops
at one it cannot read - after a hard crash of the primary, the file that was being written.
Writes are not affected, only point-in-time recovery.

1. Confirm which file, and that later ones exist:
   ```shell
   kubectl --context d3strukt0r-prod-admin -n mariadb logs <primary-pod> -c agent --tail=20
   kubectl --context d3strukt0r-prod-admin -n mariadb exec <primary-pod> -c mariadb -- bash -c 'mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" -e "SHOW BINARY LOGS"'
   ```
2. Remove the unreadable file from the server's list - `PURGE ... TO` removes every file
   **before** the one named, so name the one after it:
   ```shell
   kubectl --context d3strukt0r-prod-admin -n mariadb exec <primary-pod> -c mariadb -- bash -c 'mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" -e "PURGE BINARY LOGS TO '"'"'mariadb-bin.NNNNNN+1'"'"'"'
   ```
3. That leaves a gap no restore can cross, so take a fresh physical backup at once - it is
   the new starting point for everything after the gap:
   ```shell
   kubectl --context d3strukt0r-prod-admin -n mariadb patch physicalbackup mariadb-daily --type=merge -p "{\"spec\":{\"schedule\":{\"onDemand\":\"$(date +%s)\"}}}"
   ```
4. Within ten minutes the agent archives the remaining files; `kubectl get pitr` shows a
   `LAST RECOVERABLE TIME` again and the MariaDB turns Ready.

### MariaDB: restore to a point in time

A restore creates a **new** MariaDB from the backups; the running one is not touched.

1. Pick the target time (UTC). `strictMode` refuses any time the archive cannot reach, and
   an operator bug also refuses one that falls inside the newest archived binary log:
   - **As late as possible**: copy `LAST RECOVERABLE TIME` from `kubectl get pitr`
     exactly - the end of the newest file is accepted. Leaving the time out means "now",
     which is always refused.
   - **A moment before an accident**: if it is refused as "timeline did not reach target
     time" although it lies before `LAST RECOVERABLE TIME`, it is inside the newest file;
     wait for the next upload (at most twenty minutes) and retry.
2. Apply a second MariaDB - a copy of `components/mariadb/mariadb.yaml` with another name
   (e.g. `mariadb-restore`), without `pointInTimeRecoveryRef` and without
   `replication.replica.recovery`/`bootstrapFrom` (both point at the original's backups),
   and with:
   ```yaml
   bootstrapFrom:
     pointInTimeRecoveryRef:
       name: pitr
     targetRecoveryTime: 2026-10-01T18:00:00Z
   ```
   Keep `replicas: 2` - the webhook refuses replication with one.
3. Wait for it to be ready, and **check the condition `BinlogsReplayed`**:
   `kubectl --context d3strukt0r-prod-admin -n mariadb get mariadb mariadb-restore -o jsonpath='{.status.conditions}' | jq`.
   Ready without it means only the nightly backup was restored.
4. Copy what is needed back (`mariadb-dump` from the restored instance), or switch the app to
   it. Then delete the restored MariaDB, and its PVCs and Hetzner volumes by hand (Retain).

**The target time cannot be changed** once applied. A failed restore is retried by deleting
the MariaDB, its PVCs (`storage-mariadb-restore-0`/`-1`), their PVs and the Hetzner volumes
(`hcloud volume delete`, once they show no server) - a new MariaDB on the old volumes finds
data there and skips the restore.

A drill on 2026-09-28 restored the nightly backup plus ten minutes of binary logs in under
two minutes, to the millisecond.

### MariaDB: disaster - restore as production

**Untested** (it would mean deleting the production database). Only when the running MariaDB is
beyond repair - both volumes lost or corrupt; for anything less, restore next to it as above.
Apps cannot write until it is done.

The restored instance must be called `mariadb` again, so the apps' Service names and the
Application still fit. Argo CD recreates whatever git describes the moment the MariaDB is
deleted, so the restore goes through git:

1. Pick the target time as above.
2. In `components/mariadb/`, in one commit:
   - `mariadb.yaml`: add `bootstrapFrom` (`pointInTimeRecoveryRef: pitr`, `targetRecoveryTime`)
     and point `pointInTimeRecoveryRef` at a new `PointInTimeRecovery`;
   - `backups.yaml`: that new object, a copy of `pitr` named e.g. `pitr-<date>` with prefix
     `pitr-<date>`. The restored servers get the same server ids as the old ones, and their
     binary logs must not mix with the old history in `pitr`, which stays readable (for older
     restore points) until the bucket expires it after 35 days.

   Push. Argo CD may fail to apply `bootstrapFrom` to the broken instance; that is expected.
3. Delete the broken MariaDB and its PVCs (`storage-mariadb-0`/`-1`) plus their PVs and Hetzner
   volumes. Argo CD recreates it from git, and it bootstraps from the backups.
4. Check `BinlogsReplayed` as above, then take a physical backup at once (the command under
   "failover hangs").
5. `bootstrapFrom` is only read at creation; leave it or remove it in a later commit.

## PostgreSQL (CloudNativePG)

The shared PostgreSQL for every app that only supports it: a primary and a replica, run by
CloudNativePG, backed up every night and with its WAL archived continuously (a new file at least
every 5 minutes), so it can be restored to any moment in the last 30 days. Apps connect to
**`postgres-rw.postgres.svc:5432`**. The operator runs in `cnpg-system` (two replicas, one
active), next to the Barman Cloud plugin, which ships backups and WAL to
`d3strukt0r-prod-postgres-backups`.

**Before the first sync**, the S3 key goes into OpenBao (`secret/postgres-backups-s3`) - see
"Object Storage" in `tofu/README.md`.

```shell
kubectl --context d3strukt0r-prod-admin -n postgres get cluster,pods,backups,scheduledbackups
kubectl --context d3strukt0r-prod-admin -n cnpg-system get pods,lease,certificate
```

The `cnpg` kubectl plugin (`brew install kubectl-cnpg`) adds `kubectl cnpg status postgres -n
postgres`, which shows the primary, replication and the archiving state in one view.

### PostgreSQL: adding an app

1. A generated password in 1Password (`PostgreSQL | Prod | <app>`), then into OpenBao as
   `secret/postgres-apps/<app>` with a `password` field (JSON on stdin, as for MariaDB).
2. In the app's own component, `components/<app>/database.yaml` (`zitadel/` is the model),
   every object with `namespace: postgres` - CloudNativePG wants a role, its password Secret
   and a database in the Cluster's namespace:
   - an ExternalSecret building Secret `<app>-db` of type `kubernetes.io/basic-auth`
     (`username: <app>` as a literal, `password` from OpenBao) **with the label
     `cnpg.io/reload: "true"`** (through `target.template.metadata.labels`) - without it the
     role may never be created, see CLAUDE.md;
   - a `DatabaseRole` (`cluster: postgres`, `name: <app>`, `login: true`,
     `passwordSecret: <app>-db`) and a `Database` (`cluster: postgres`, `name: <app>`,
     `owner: <app>`).

   Check both with
   `kubectl -n postgres get databaseroles.postgresql.cnpg.io,databases.postgresql.cnpg.io`
   (`APPLIED` true; `status.message` says why not).
3. In the app's own component: an ExternalSecret reading the same `secret/postgres-apps/<app>`,
   host `postgres-rw.postgres.svc`, and `sslmode=verify-full` against CloudNativePG's CA. The CA
   certificate is copied into the app's namespace by `postgres-ca.yaml` - copy Zitadel's, which
   holds a ServiceAccount, a Role in `postgres` that may `get` only Secret `postgres-ca`, a
   `SecretStore` (External Secrets' Kubernetes provider) and an ExternalSecret taking just
   `ca.crt`; rename the Role and RoleBinding after the app.

### PostgreSQL: failover hangs

`PostgresNoReadyPrimary` means apps cannot write. A failover normally finishes on its own: 8-11
seconds for a pod, about a minute and a half for a dead node (tested 2026-09-29).

1. Look: `kubectl cnpg status postgres -n postgres` and the operator's log
   (`kubectl -n cnpg-system logs deploy/cloudnative-pg`) - which instance was primary, on which
   node, is that node `NotReady`?
2. If the old primary's node stays gone, its pod stays `Terminating` and its volume attached, so
   that instance cannot be recreated elsewhere. The failover itself does not need it; to get
   the second instance back, declare the node gone
   (`kubectl taint nodes <node> node.kubernetes.io/out-of-service=nodeshutdown:NoExecute`, as for
   MariaDB) and remove the taint once the node is back or replaced.
3. A primary can be chosen by hand: `kubectl cnpg promote postgres <instance> -n postgres`.

### PostgreSQL: restore to a point in time

A restore creates a **new** cluster from the backups; the running one is not touched.

1. Apply a second `Cluster` in `postgres` with its own name, the same image and storage, one
   instance, and no `plugins` (so it never archives into the production path):
   ```yaml
   bootstrap:
     recovery:
       source: origin
       recoveryTarget:
         targetTime: "2026-10-01 16:00:00+00"   # UTC; omitted = as late as the archive goes
   externalClusters:
     - name: origin
       plugin:
         name: barman-cloud.cloudnative-pg.io
         parameters:
           barmanObjectName: postgres-backups
           serverName: postgres
   ```
2. Wait for `kubectl -n postgres get cluster <name>` to report "Cluster in healthy state", then
   check the data (`kubectl -n postgres exec <name>-1 -c postgres -- psql -d <db> ...`).
3. Copy what is needed back (`pg_dump` from the restored instance), or point the app at it.
   Then delete the Cluster, its PVC, the PV and the Hetzner volume by hand (Retain).

### PostgreSQL: disaster - restore as production

**Untested** (it would mean deleting the production database). Only when the running cluster
is beyond repair - both volumes lost or corrupt; for anything less, restore next to it as above.
Apps cannot write until it is done.

The restored cluster must be called `postgres` again, so `postgres-rw` and the Application
still fit, and it must archive into a new path: a cluster that finds WAL of its own name in the
archive refuses to archive ("Expected empty archive"), which is the check protecting the old
history. Argo CD recreates whatever git describes the moment the Cluster is deleted, so the
restore goes through git:

1. In `components/postgres/cluster.yaml`, in one commit: `bootstrap.recovery` and
   `externalClusters` as in the section above (`serverName: postgres` - the old path, read
   only), and the archiving plugin's parameters get `serverName: postgres-<date>` - the new
   path. Push. Argo CD may fail to apply the change to the broken cluster; that is expected.
2. Delete the Cluster `postgres` and its PVCs (`postgres-1`, `postgres-2`) plus their PVs and
   Hetzner volumes. Argo CD recreates it from git, and it recovers from the backups.
3. Check the data, then take a base backup at once:
   `kubectl cnpg backup postgres -n postgres --method plugin --plugin-name barman-cloud.cloudnative-pg.io`.
4. The old path (`postgres/` in the bucket) is no longer pruned by anyone - it has no cluster
   archiving to it. Keep it as long as its restore points may matter, then delete it by hand.
   The `bootstrap` and `externalClusters` are only read at creation; leave them or remove them
   in a later commit.

## Zitadel

The identity provider at **`https://auth.d3strukt0r.dev`**: one login (OIDC/SAML) for the
cluster's UIs and apps. Its data is in the shared PostgreSQL (database `zitadel`).

**Before the first sync**, its secrets go into OpenBao (port-forward and `BAO_TOKEN` as in
"OpenBao"); the database password is set up as in "PostgreSQL: adding an app":

1. Two 1Password items: the first admin's login [`Zitadel | Prod | Admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=hwednr7k3ycfvmdgtbcylq5nwi&h=my.1password.com) (generated password),
   and [`Zitadel | Prod | Masterkey`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=cpgucnzccroyrl5zwzdrmcbfau&h=my.1password.com), **exactly 32 characters** (letters and digits), written
   once and never edited:

   ```shell
   op item create --account my.1password.com --vault Private --category login --title 'Zitadel | Prod | Admin' \
     --url https://auth.d3strukt0r.dev/ui/console --generate-password='letters,digits,symbols,20' \
     username=auth-admin@d3strukt0r.dev >/dev/null
   op item create --account my.1password.com --vault Private --category password --title 'Zitadel | Prod | Masterkey' \
     "password=$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 32)" >/dev/null
   ```
2. As JSON on stdin, with the empty check:

   ```shell
   jq -n --arg m "$(op item get 'Zitadel | Prod | Masterkey' --account my.1password.com --vault Private --fields password --reveal)" \
         --arg a "$(op item get 'Zitadel | Prod | Admin' --account my.1password.com --vault Private --fields password --reveal)" \
     'if ($m|length)!=32 or ($a|length)==0 then error("masterkey must be 32 characters, admin password non-empty") else {"masterkey":$m,"admin-password":$a} end' \
   | BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN="$(op item get 'OpenBao | Prod | Recovery keys & root token' --account my.1password.com --vault Private --fields credential --reveal)" \
     bao kv put secret/zitadel -
   ```

**The masterkey must never be lost or changed**: it encrypts the secrets in Zitadel's database.

The first start creates the organisation `D3strukt0r` with the admin `auth-admin@d3strukt0r.dev`
(the password from [`Zitadel | Prod | Admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=hwednr7k3ycfvmdgtbcylq5nwi&h=my.1password.com), kept as the login) at
`https://auth.d3strukt0r.dev/ui/console`, and the machine user `iam-admin`, whose key the setup
job stores as Secret `iam-admin` in `zitadel` - copy it into 1Password too, as the document
[`Zitadel | Prod | iam-admin key`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=jitp4bfd4oirrwza3vpl3fr6oi&h=my.1password.com), without printing it:

```shell
kubectl --context d3strukt0r-prod-admin -n zitadel get secret iam-admin -o jsonpath='{.data.iam-admin\.json}' | base64 -d > /tmp/iam-admin.json \
  && op document create /tmp/iam-admin.json --title 'Zitadel | Prod | iam-admin key' --account my.1password.com --vault Private \
  && rm /tmp/iam-admin.json
```

The instance's login and domain policies and the organisation's domains are in `tofu/zitadel`
(see `tofu/README.md`): MFA required for Zitadel passwords (authenticator app or security
key/passkey, no e-mail or SMS codes), organisation domains only after DNS verification, no login
name suffix - so a username is the login name and must be unique across all organisations (an
e-mail address, or a handle nobody else will take, like `D3strukt0r`). The domains
`d3strukt0r.dev` (primary) and `d3st.dev` are verified by the `_zitadel-challenge.<domain>` TXT
records in `tofu/cloudflare`, which stay: Zitadel re-checks them periodically, and a re-added
domain gets a new code for the record's content.

Still by hand:

1. `auth-admin`: Password and Security → Multifactor Authentication → Authenticator App, the QR
   code scanned into [`Zitadel | Prod | Admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=hwednr7k3ycfvmdgtbcylq5nwi&h=my.1password.com), tested by logging out and in. On a fresh
   instance this comes **before** the first `tofu apply` of `tofu/zitadel`, which requires MFA.
2. Cloudflare dashboard → `d3strukt0r.dev` → Network → **gRPC on**: `tofu/zitadel`'s provider
   speaks gRPC, which Cloudflare refuses otherwise.

A new user gets a generated first password and the e-mail marked verified (there is no mail
server yet), and sets up the authenticator app at the first login. **If that first login loops**
with `mfa required (AUTHZ-Kl3p0)` instead of offering the second factor, start a new login
attempt: the first one for `D3strukt0r` (2026-09-30) let the user in without the setup and the
console refused the session over and over, while the next attempt showed the setup - in a
private window and in the normal one alike. The cause is unknown; the login page has several
such loops open upstream.

Losing the admin's second factor is recovered with the `iam-admin` key, which holds the same
instance rights and can remove the factor through the API.

```shell
kubectl --context d3strukt0r-prod-admin -n zitadel get pods,jobs,certificates,externalsecrets
```

## The login gate

oauth2-proxy guards the UIs without a Zitadel login of their own (today the Traefik
dashboard). Its Zitadel app comes from `tofu/zitadel` (`apps_oauth2_proxy.tf`); **before the first
sync**, its secrets go into OpenBao:

1. Right after `tofu apply` created the app, regenerate its secret - the one from creation is in
   the tofu state: Zitadel console → Projects → Infrastructure → oauth2-proxy → Regenerate
   Secret. It is shown once.
2. A 1Password item with that secret and a cookie secret (exactly 32 random bytes - oauth2-proxy
   accepts only 16, 24 or 32):

   ```shell
   op item create --account my.1password.com --vault Private --category password \
     --title 'Zitadel | Prod | oauth2-proxy' \
     "password=<the regenerated client secret>" \
     "cookie-secret[password]=$(openssl rand -base64 32 | tr -- '+/' '-_')" >/dev/null
   ```
3. As JSON on stdin, with the empty check (port-forward and token as in "OpenBao"):

   ```shell
   jq -n --arg c "$(op item get 'Zitadel | Prod | oauth2-proxy' --account my.1password.com --vault Private --fields password --reveal)" \
         --arg k "$(op item get 'Zitadel | Prod | oauth2-proxy' --account my.1password.com --vault Private --fields cookie-secret --reveal)" \
     'if ($c|length)==0 or ($k|length)==0 then error("empty value - 1Password lookup failed") else {"client-secret":$c,"cookie-secret":$k} end' \
   | BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN="$(op item get 'OpenBao | Prod | Recovery keys & root token' --account my.1password.com --vault Private --fields credential --reveal)" \
     bao kv put secret/oauth2-proxy -
   ```

A new client ID (a recreated app) goes into `components/oauth2-proxy/external-secrets.yaml`.

## Scale to zero

An app that is rarely used can sleep at zero replicas and wake on its first request, which
KubeElasti holds until the app is ready (a few seconds - 3 s for a small test app). What an app
needs, all in its own component:

1. **No `replicas` in its Deployment** - KubeElasti scales it, and one replica when awake.
2. **Its Service** carries `traefik.ingress.kubernetes.io/service.nativelb: "true"`.
3. **A Middleware** telling KubeElasti which app a request is for, referenced from its Ingress
   (`traefik.ingress.kubernetes.io/router.middlewares: <ns>-kubeelasti-target@kubernetescrd`):

   ```yaml
   apiVersion: traefik.io/v1alpha1
   kind: Middleware
   metadata:
     name: kubeelasti-target
   spec:
     headers:
       customRequestHeaders:
         X-Envoy-Decorator-Operation: <svc>.<ns>.svc.cluster.local
   ```

4. **An ElastiService** (`argocd.argoproj.io/sync-options: SkipDryRunOnMissingResource=true`, the
   CRD is the kubeelasti Application's):

   ```yaml
   apiVersion: elasti.truefoundry.com/v1alpha1
   kind: ElastiService
   metadata:
     name: <app>
   spec:
     service: <svc>
     minTargetReplicas: 1
     cooldownPeriod: 900          # seconds without requests before it sleeps
     scaleTargetRef:
       apiVersion: apps/v1
       kind: Deployment
       name: <deployment>
     triggers:
       - type: prometheus
         metadata:
           query: sum(rate(traefik_service_requests_total{service="<ns>-<svc>-<port name>@kubernetes"}[2m])) or vector(0)
           threshold: "0.01"
     probeResponse:
       - method: GET
         path:
           type: Exact
           value: /health
         headers:
           - name: X-Health-Probe
             value: gatus
         response:
           status: 200
           body: '{"status":"asleep"}'
   ```

5. **Its Gatus check** goes to the Service inside the cluster, with the probe header - through
   the public address it would wake the app every minute and keep it awake:

   ```yaml
   - name: <App>
     group: Apps
     url: http://<svc>.<ns>.svc/health
     headers:
       X-Health-Probe: gatus
     conditions:
       - "[STATUS] == 200"
   ```

Whether it sleeps:

```shell
kubectl --context d3strukt0r-prod-admin -n <ns> get elastiservice <app> -o jsonpath='{.status.mode}{"\n"}'   # proxy = asleep, serve = awake
kubectl --context d3strukt0r-prod-admin -n <ns> get deployment <deployment>
```

**Removing it again**: wake the app first (one request) before deleting only the ElastiService -
KubeElasti 0.1.30 leaves a sleeping app at zero otherwise. Removing the whole app does not need
that.

## How quickly a push arrives

At once: GitHub sends every push to `https://argocd.d3strukt0r.dev/api/webhook`, signed with
the secret in 1Password [`GitHub | infrastructure | Argo CD webhook`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=gknq7z4job2lgkcpzszvucaxay&h=my.1password.com) (OpenBao
`secret/argocd-webhook`), and Argo CD refreshes every Application using this repository.
Polling at Argo CD's default (120 s plus up to 60 s of jitter) remains the fallback for a lost
delivery. The application controller and the repo server read the polling interval at start,
so after changing it in `patches/argocd-cm.yaml`, restart both once the change has synced:

```shell
kubectl --context d3strukt0r-prod-admin -n argocd rollout restart statefulset argocd-application-controller
kubectl --context d3strukt0r-prod-admin -n argocd rollout restart deployment argocd-repo-server
```

The webhook itself lives in GitHub (repository settings → Webhooks, or `gh api
repos/Team-MaRo/infrastructure/hooks`); its recent deliveries and their answers show there too.
Creating it again:

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
