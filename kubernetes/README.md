# kubernetes

What runs on the cluster, deployed by Argo CD from this directory on `master`.

**Pushing to `master` is deploying.** Argo CD watches this directory and applies what it
finds, pruning anything removed. There is no separate release step, which makes branch
protection on the repository a security control rather than a formality.

Pruning stops at the Application, though: deleting a file from `clusters/<name>/` removes
only the Application, and what it deployed keeps running untracked (no Application has the
`resources-finalizer.argocd.argoproj.io` finalizer). Removing a component for real means
deleting its objects by hand as well.

## Layout

```
kubernetes/
├── clusters/                  # what runs where - one directory per cluster
│   └── prod/
│       ├── root.yaml          # syncs this directory - itself and every sibling
│       ├── argocd.yaml        # Argo CD managing its own installation
│       ├── hcloud-csi.yaml    # persistent volumes
│       ├── openbao.yaml       # the secret store: Helm chart pinned here, values in components/
│       ├── external-secrets.yaml  # delivers OpenBao values as Kubernetes Secrets
│       ├── system-upgrade-controller.yaml  # upgrades k3s on the nodes
│       ├── kured.yaml             # reboots nodes after kernel updates
│       ├── kyverno.yaml           # policies: chart pinned here, values and policies in components/
│       ├── cert-manager.yaml      # certificates from Let's Encrypt
│       ├── traefik.yaml           # k3s's Traefik: DaemonSet on the ingress nodes' host network
│       ├── uptime-kuma.yaml       # website checks and the alerting heartbeat
│       ├── kube-prometheus-stack.yaml  # metrics and alerts: chart pinned here, values in components/
│       ├── loki.yaml              # log storage: chart pinned here, values in components/
│       ├── alloy.yaml             # log collection on every node: chart pinned here
│       ├── mariadb-operator-crds.yaml  # the operator's CRDs - never pruned
│       ├── mariadb-operator.yaml  # the operator running the app database
│       ├── mariadb.yaml           # the app database itself
│       ├── etcd-snapshots.yaml    # syncs the subdirectory below
│       └── etcd-snapshots/        # prod-only: the S3 settings k3s uploads snapshots with
└── components/                # how each component is deployed, shared by clusters
    ├── argocd/                # pinned upstream install.yaml plus patches
    │   ├── kustomization.yaml
    │   ├── namespace.yaml
    │   └── patches/           # argocd-cm, Dex removed, memory per container
    ├── hcloud-csi/            # Hetzner's CSI driver, pinned
    │   ├── kustomization.yaml
    │   └── patches/           # reclaimPolicy Retain, memory per container
    ├── openbao/
    │   └── values.yaml        # Helm values: one replica, Raft storage, static seal
    ├── external-secrets/
    │   ├── values.yaml            # Helm values: memory
    │   ├── kustomization.yaml
    │   └── cluster-secret-store.yaml  # the store named openbao
    ├── traefik/
    │   ├── kustomization.yaml
    │   └── helmchartconfig.yaml   # values merged into k3s's bundled Traefik
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
    │   └── rules.yaml             # this cluster's own alerts
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
    │   ├── external-secrets.yaml  # root/replication passwords and the S3 key from OpenBao
    │   ├── s3-ca.yaml             # the roots Hetzner's S3 certificate chains to
    │   └── rules.yaml             # alerts: no ready primary, broken or lagging replication
    ├── alloy/
    │   ├── values.yaml            # Helm values: the collection pipeline
    │   ├── kustomization.yaml
    │   └── clusterrole.yaml       # only what the pipeline reads
    └── uptime-kuma/
        ├── kustomization.yaml
        ├── deployment.yaml        # one replica, pinned rootless image, SQLite
        ├── pvc.yaml               # its 10 GB volume
        └── service.yaml
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

Not exposed yet: the UIs open once Zitadel provides SSO (TLS via cert-manager and Traefik is
already in place). Until then:

```shell
kubectl --context d3strukt0r-prod-admin -n argocd port-forward svc/argocd-server 8080:443
```

then `https://localhost:8080`; expect a self-signed certificate warning. Uptime Kuma the same
way:

```shell
kubectl --context d3strukt0r-prod-admin -n uptime-kuma port-forward svc/uptime-kuma 3001:3001
```

then `http://localhost:3001`. Grafana, Prometheus and Alertmanager:

```shell
kubectl --context d3strukt0r-prod-admin -n monitoring port-forward svc/kube-prometheus-stack-grafana 3000:80
kubectl --context d3strukt0r-prod-admin -n monitoring port-forward svc/kube-prometheus-stack-prometheus 9090:9090
kubectl --context d3strukt0r-prod-admin -n monitoring port-forward svc/kube-prometheus-stack-alertmanager 9093:9093
```

then `http://localhost:3000` (user `admin`, password in 1Password `Grafana | Prod | Admin`),
`http://localhost:9090` (Status → Targets shows every scrape) and `http://localhost:9093`
(firing alerts, silences).

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

## etcd snapshots

k3s uploads its twice-daily etcd snapshots to the bucket `d3strukt0r-prod-etcd` with the
cluster's own S3 key. The ExternalSecret in `clusters/prod/etcd-snapshots/` builds Secret
`kube-system/k3s-etcd-snapshot-s3-config` from it; `k3s_config` in Ansible points k3s at
that Secret.

Setting it up, or replacing the key:

1. Create an S3 key in the Hetzner console, labelled `prod etcd snapshots`, and store it in
   1Password as `Hetzner | S3 | prod etcd snapshots` (access key as username, secret key as
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

**A restore cannot read the Secret**, since the apiserver is down. It needs the S3 settings
as flags and the server token from 1Password (`k3s | Prod | Server token`); see the
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
2. Store them in 1Password as `Cloudflare | cert-manager DNS (prod cluster)` and
   `Cloudflare | Arepazo | cert-manager DNS (prod cluster)` (field `credential`).
3. With the port-forward to OpenBao open and `BAO_TOKEN` set:

   ```shell
   bao kv put secret/cloudflare-dns \
     personal="$(op item get 'Cloudflare | cert-manager DNS (prod cluster)' --account my.1password.com --vault Private --fields credential --reveal)" \
     arepazo="$(op item get 'Cloudflare | Arepazo | cert-manager DNS (prod cluster)' --account my.1password.com --vault Private --fields credential --reveal)"
   ```

## Uptime Kuma

Checks the websites and watches the alerting pipeline itself: Alertmanager sends its
always-firing `Watchdog` alert to an Uptime Kuma push monitor every minute, and when that
stops, Uptime Kuma alerts through ntfy. Its configuration lives in its database, not in git,
so it is set up by hand once - and again only if its volume is lost. The list below is what to
recreate.

Alerts go to one ntfy.sh topic, whose name is the secret: on the free plan a topic cannot be
reserved, so anyone who knows the name could read it.

1. **ntfy**, once for all monitoring:
   - Create the topic name with `echo "prod-alerts-$(openssl rand -hex 16)"` - the prefix says
     what it is, the 128 random bits make it unguessable.
   - Create an access token at ntfy.sh → Account → Access tokens, named `prod cluster alerts`,
     **never expiring** (an expiring token would silently stop the alerts).
   - Store both in 1Password as `ntfy | prod alerts` (topic as `username`, token as
     `credential`), and subscribe to the topic in the ntfy Android app.
   - Copy them into OpenBao for Alertmanager (port-forward to OpenBao open):

     ```shell
     BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN="$(op item get 'OpenBao | Prod | Recovery keys & root token' --account my.1password.com --vault Private --fields credential --reveal)" \
       bao kv put secret/ntfy \
         topic="$(op item get 'ntfy | prod alerts' --account my.1password.com --vault Private --fields username)" \
         token="$(op item get 'ntfy | prod alerts' --account my.1password.com --vault Private --fields credential --reveal)"
     ```

2. Port-forward (see "Accessing the UI"), open `http://localhost:3001`, create the admin
   account and store it in 1Password as `Uptime Kuma | Prod | Admin`. Enable 2FA under
   Settings → Security → Two Factor Authentication, scanning the QR code into the same
   1Password item as a one-time password.
3. Settings → Notifications → Setup Notification: type **ntfy**, display name
   `ntfy prod alerts`, server `https://ntfy.sh`, the topic, authentication **Access token**
   with the token, priority 4 and DOWN priority 5, **Default enabled**. **Test** must reach the
   phone before saving.
4. Add New Monitor → **Push**, name `Alertmanager Watchdog`, heartbeat interval 300 s,
   retries 1. **Pause it** until Alertmanager is deployed (see "Metrics and alerts"), or it
   goes DOWN after five minutes.
   Its push URL, as shown, is `http://localhost:3001/api/push/<token>?status=up&msg=OK&ping=`;
   put it into OpenBao with the cluster-internal host instead and without `&ping=`:

   ```shell
   BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN="$(op item get 'OpenBao | Prod | Recovery keys & root token' --account my.1password.com --vault Private --fields credential --reveal)" \
     bao kv put secret/uptime-kuma \
       watchdog-push-url='http://uptime-kuma.uptime-kuma.svc:3001/api/push/<token>?status=up&msg=OK'
   ```

5. One **HTTP(s)** monitor per entry in the list below, interval 60 s, retries 2.

| Monitor | Type | Target |
|---|---|---|
| Alertmanager Watchdog | Push | Alertmanager, every minute |

**Losing the volume** loses this configuration and the check history, nothing else: repeat
the steps, and replace the push URL in OpenBao, since a new monitor gets a new token.

## Metrics and alerts

kube-prometheus-stack: Prometheus collects metrics for 30 days, Grafana shows them, and
Alertmanager sends alerts to the phone through the ntfy topic from "Uptime Kuma" -
`critical` at priority 5, `warning` at 3, `info` only in Grafana. It needs, before its first
sync, `secret/ntfy` and `secret/uptime-kuma` (see "Uptime Kuma") and Grafana's admin
password:

1. Create a password item `Grafana | Prod | Admin` in 1Password (username `admin`).
2. With the port-forward to OpenBao open:

   ```shell
   jq -n --arg p "$(op item get 'Grafana | Prod | Admin' --account my.1password.com --vault Private --fields password --reveal)" \
     'if ($p|length)==0 then error("empty value - 1Password lookup failed") else {"admin-password":$p} end' \
   | BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN="$(op item get 'OpenBao | Prod | Recovery keys & root token' --account my.1password.com --vault Private --fields credential --reveal)" \
     bao kv put secret/grafana -
   ```

   JSON on stdin rather than `key=value`: a generated password may start with `@`, and a
   failed lookup would otherwise store an empty value (see "OpenBao" in `tofu/README.md`).

As soon as the first sync is done, resume the `Alertmanager Watchdog` monitor in Uptime Kuma;
it should turn green within a minute. While it is paused, Uptime Kuma answers every heartbeat
with 404, and after a few minutes Alertmanager reports that as `AlertmanagerFailedToSendAlerts`.
That alert resolves by itself about 15 minutes after the monitor is resumed.

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
{namespace="uptime-kuma"}                                  # one namespace's containers
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

1. A 1Password item `MariaDB | Prod | Root & replication` with two generated password
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

**After every unplanned failover** - also one that finished on its own - take a physical
backup at once. The old primary's last binary log never reached the bucket, so
point-in-time recovery cannot go past its last upload until a backup from the new primary
exists (`LAST RECOVERABLE TIME` in `kubectl get pitr` stays put):

```shell
kubectl --context d3strukt0r-prod-admin -n mariadb patch physicalbackup mariadb-daily --type=merge -p "{\"spec\":{\"schedule\":{\"onDemand\":\"$(date +%s)\"}}}"
```

### MariaDB: binary log archiving stuck

The MariaDB reports Ready=False with `Error archiving binlogs: ... error getting binary log
mariadb-bin.NNNNNN metadata`, and `kubectl get pitr` shows no or an old `LAST RECOVERABLE
TIME`. The archiver uploads the files in order and stops at one it cannot read - after a hard
crash of the primary, the file that was being written. Writes are not affected, only
point-in-time recovery.

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
