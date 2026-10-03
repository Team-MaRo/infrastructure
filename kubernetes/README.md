# kubernetes

What runs on the cluster, deployed by Argo CD from this directory on `master`. Every component
documents itself in its own `README.md` - design, settings that differ from upstream, and its
runbooks; this file is the overview. Before changing a component, read its README, and update
it in the same change.

**Pushing to `master` is deploying.** Argo CD watches this directory and applies what it
finds, pruning anything removed. There is no separate release step, which makes branch
protection on the repository a security control rather than a formality.

Deleting a file from `clusters/<name>/` removes what that Application deployed only if it
carries the finalizer `resources-finalizer.argocd.argoproj.io` - apps whose data lives elsewhere
(Gatus, Zitadel, oauth2-proxy, ...), with their own `namespace.yaml`. Apps with CRDs or volumes
(the operators, OpenBao, the databases, Loki, Prometheus) deliberately have none: deleting their
file removes only the Application, and what it deployed keeps running untracked. Which app has
it, and why, is "Pruning and the finalizer" in
[`components/argocd/README.md`](components/argocd/README.md).

## Layout

```
kubernetes/
├── clusters/                  # what runs where - one directory per cluster
│   └── prod/
│       ├── root.yaml          # syncs this directory - itself and every sibling
│       ├── private.yaml       # syncs Team-MaRo/infrastructure-private, what must not be public
│       ├── argocd.yaml        # Argo CD managing its own installation
│       ├── argocd-integrations.yaml  # its certificate, webhook secret, private repository access, monitors
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
│       ├── phpmyadmin.yaml        # the shared MariaDB's web UI
│       ├── pgadmin.yaml           # the shared PostgreSQL's web UI
│       ├── reloader.yaml          # restarts apps on changed Secrets: chart pinned here, values in components/
│       ├── trust-manager.yaml     # the databases' CAs in the apps' namespaces: chart pinned here
│       ├── keel.yaml              # rolls out new images under floating tags: chart pinned here
│       ├── wedding-manuele-robine.yaml  # the wedding website
│       ├── robines-portfolio.yaml  # the old WordPress portfolio at old.robines.space
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
    │   ├── admin-role.yaml        # the superuser role `admin` for people
    │   ├── backups.yaml           # the object store, WAL archiving, the nightly base backup
    │   ├── external-secrets.yaml  # the S3 key from OpenBao
    │   ├── pod-monitor.yaml       # the instances' metrics
    │   └── rules.yaml             # alerts: no ready primary, replication, archiving, backups
    ├── zitadel/
    │   ├── values.yaml            # Helm values: domain, database, first admin, securityContexts
    │   ├── kustomization.yaml
    │   ├── database.yaml          # its role and database, in the postgres namespace
    │   ├── external-secrets.yaml  # masterkey, first admin password, database password
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
    ├── gatus/
    │   ├── kustomization.yaml     # config.yaml becomes the ConfigMap gatus
    │   ├── config.yaml            # what is checked, the Watchdog heartbeat, ntfy
    │   ├── namespace.yaml
    │   ├── deployment.yaml        # one replica, pinned image, read-only
    │   ├── database.yaml          # its role and database, in the postgres namespace
    │   ├── external-secrets.yaml  # ntfy, the heartbeat token, the database password
    │   ├── service.yaml
    │   ├── certificate.yaml       # status.d3strukt0r.dev
    │   └── ingress.yaml           # public, no login
    ├── phpmyadmin/
    │   ├── kustomization.yaml
    │   ├── namespace.yaml
    │   ├── database.yaml          # its configuration storage and control user, in the mariadb namespace
    │   ├── external-secrets.yaml  # the control user's password
    │   ├── deployment.yaml        # one replica, pinned image, as www-data
    │   ├── service.yaml
    │   ├── certificate.yaml       # phpmyadmin.d3strukt0r.dev
    │   ├── middleware-oauth2-proxy.yaml  # the Zitadel gate
    │   └── ingress.yaml
    ├── pgadmin/
    │   ├── kustomization.yaml     # config_system.py and servers.json become the ConfigMap pgadmin
    │   ├── config_system.py       # the Zitadel login, cookie settings
    │   ├── servers.json           # the shared server "PostgreSQL"
    │   ├── namespace.yaml
    │   ├── database.yaml          # its configuration database and role, in the postgres namespace
    │   ├── external-secrets.yaml  # the configuration database's URI, the internal user's password
    │   ├── deployment.yaml        # one replica, pinned image, no volume
    │   ├── service.yaml
    │   ├── certificate.yaml       # pgadmin.d3strukt0r.dev
    │   └── ingress.yaml           # no gate: pgAdmin logs in through Zitadel itself
    ├── reloader/
    │   ├── values.yaml            # Helm values: annotations strategy, watched namespaces, securityContext
    │   ├── kustomization.yaml
    │   └── namespace.yaml
    ├── trust-manager/
    │   ├── values.yaml            # Helm values: its own trust namespace
    │   ├── kustomization.yaml
    │   ├── namespace.yaml
    │   ├── ca-copies.yaml         # the two CA certificates copied into its trust namespace
    │   ├── bundles.yaml           # postgres-ca and mariadb-ca for labelled namespaces
    │   └── service-monitor.yaml
    ├── keel/
    │   ├── values.yaml            # Helm values: polling, RBAC without Secrets, securityContext
    │   ├── kustomization.yaml
    │   ├── namespace.yaml
    │   ├── external-secrets.yaml  # the ntfy URL for its notifications
    │   └── service-monitor.yaml
    ├── wedding-manuele-robine/
    │   ├── kustomization.yaml
    │   ├── namespace.yaml         # exempt from restricted Pod Security (root images, TODO)
    │   ├── database.yaml          # its database and user, in the mariadb namespace
    │   ├── external-secrets.yaml  # app secrets, database URL, S3 key, JWT keys, Maps key
    │   ├── api.yaml               # the Symfony API and its Service
    │   ├── pwa.yaml               # the website and its Service
    │   └── ingresses.yaml         # certificates and Ingresses for the four names
    └── robines-portfolio/
        ├── kustomization.yaml
        ├── namespace.yaml
        ├── database.yaml          # its database and user, in the mariadb namespace
        ├── external-secrets.yaml  # keys and salts, database password, the two S3 keys
        ├── deployment.yaml        # WordPress from stock images, plugins at start, uploads proxy
        ├── cronjob.yaml           # WP-Cron every five minutes
        ├── service.yaml
        ├── certificate.yaml
        ├── ingress.yaml           # old.robines.space
        └── config/                # plugin list, nginx, php.ini, the S3-Uploads loader
```

| Component | What it is |
|---|---|
| [`argocd`](components/argocd/README.md) | Argo CD managing its own installation; pruning, bootstrap, upgrades, its UI |
| [`argocd-integrations`](components/argocd-integrations/README.md) | Argo CD's certificate, webhook secret and metrics monitors |
| [`hcloud-csi`](components/hcloud-csi/README.md) | Persistent volumes on Hetzner Volumes; released volumes |
| [`openbao`](components/openbao/README.md) | The secret store: seal, recovery keys, snapshots, restore |
| [`external-secrets`](components/external-secrets/README.md) | Delivers OpenBao values as Kubernetes Secrets |
| [`system-upgrade-controller`](components/system-upgrade-controller/README.md) | Upgrades k3s on the nodes |
| [`kured`](components/kured/README.md) | Reboots nodes after kernel updates |
| [`kyverno`](components/kyverno/README.md) | Default resources (LimitRange) and allowed registries |
| [`traefik`](components/traefik/README.md) | k3s's Traefik on the ingress nodes' host network, the dashboard |
| [`cert-manager`](components/cert-manager/README.md) | Certificates from Let's Encrypt over DNS-01 |
| [`gatus`](components/gatus/README.md) | The public status page and the alerting heartbeat |
| [`kube-prometheus-stack`](components/kube-prometheus-stack/README.md) | Prometheus, Alertmanager, Grafana; the cluster's own alerts |
| [`loki`](components/loki/README.md) | Log storage in Object Storage, 30 days |
| [`alloy`](components/alloy/README.md) | Log collection on every node |
| [`mariadb-operator`](components/mariadb-operator/README.md) | The operator running the app database |
| [`mariadb`](components/mariadb/README.md) | The shared MariaDB: failover, backups, restore runbooks |
| [`cloudnative-pg`](components/cloudnative-pg/README.md) | The PostgreSQL operator and its backup plugin |
| [`postgres`](components/postgres/README.md) | The shared PostgreSQL: adding an app, failover, restore runbooks |
| [`zitadel`](components/zitadel/README.md) | The identity provider (SSO) |
| [`oauth2-proxy`](components/oauth2-proxy/README.md) | The Zitadel login gate for UIs without a login of their own |
| [`phpmyadmin`](components/phpmyadmin/README.md) | The shared MariaDB's web UI, behind the Zitadel gate |
| [`pgadmin`](components/pgadmin/README.md) | The shared PostgreSQL's web UI, with its own Zitadel login |
| [`reloader`](components/reloader/README.md) | Restarts apps when a Secret or ConfigMap they read at start changes |
| [`trust-manager`](components/trust-manager/README.md) | The databases' CA certificates as ConfigMaps in the namespaces that ask for them |
| [`keel`](components/keel/README.md) | Rolls out new images under floating tags without a commit |
| [`wedding-manuele-robine`](components/wedding-manuele-robine/README.md) | The wedding website (moved from prod-old) |
| [`robines-portfolio`](components/robines-portfolio/README.md) | The old WordPress portfolio at old.robines.space (moved from prod-old) |
| [`kubeelasti`](components/kubeelasti/README.md) | Scale to zero; letting an app sleep |
| [`cluster-rbac`](components/cluster-rbac/README.md) | Cluster-wide rights for kubectl logins through Zitadel |
| [`clusters/prod/etcd-snapshots`](clusters/prod/etcd-snapshots/README.md) | prod-only: the S3 settings k3s uploads etcd snapshots with; restore |

Components are Kustomize over a pinned upstream manifest where upstream publishes one.
Where upstream ships only a Helm chart (OpenBao), the cluster's Application pins the chart
and reads its values from `components/<app>/values.yaml` through a second source.

The split is **what** against **how**. `clusters/<name>/` decides which components a
cluster runs; `components/<app>/` holds the manifests, written once and shared. Each
cluster runs its own Argo CD, which syncs only its own directory, so no cluster can change
another.

Each cluster directory is an app-of-apps: `root` syncs the directory it lives in, so it
manages itself as well as its siblings.

- **Adding a component** means a directory under `components/` with its own `README.md`, and
  one Application file in each cluster directory that should run it. If the app reads a Secret
  or ConfigMap only at start, it opts in to Reloader: the annotation on its workload and its
  namespace in Reloader's list ([`components/reloader/README.md`](components/reloader/README.md)).
  An app on a floating tag opts in to Keel ([`components/keel/README.md`](components/keel/README.md)).
- **When one cluster needs something different**, give it a small Kustomize overlay under
  its own directory that references the component, rather than copying the component.
- **Adding a cluster** means a new `clusters/<name>/` with its own `root.yaml` and
  `argocd.yaml`, and `cluster_name` set for its inventory group. See "What a second cluster
  would need" in [`AGENTS.md`](../AGENTS.md) for what else in Ansible and OpenTofu still
  assumes a single cluster.

## Bootstrap

Argo CD cannot deploy itself the first time, so `ansible/argocd.yml` installs it once and
applies `clusters/<cluster_name>/root.yaml` - see "Bootstrap" in
[`components/argocd/README.md`](components/argocd/README.md) and
[`ansible/roles/argocd/README.md`](../ansible/roles/argocd/README.md).

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

Every UI logs in through Zitadel with the role `infra-admin`; each README has the break-glass
way in (a local admin and a port-forward).

- **Argo CD** - `https://argocd.d3strukt0r.dev`, CLI `argocd login argocd.d3strukt0r.dev --sso --grpc-web`:
  "Accessing the UI" in [`components/argocd/README.md`](components/argocd/README.md)
- **Grafana, Prometheus, Alertmanager** - `https://grafana.d3strukt0r.dev`, the other two by
  port-forward: "Accessing the UIs" in
  [`components/kube-prometheus-stack/README.md`](components/kube-prometheus-stack/README.md)
- **OpenBao** - `https://openbao.d3strukt0r.dev`, method OIDC: "Logging in" in
  [`components/openbao/README.md`](components/openbao/README.md)
- **Traefik dashboard** - `https://traefik.d3strukt0r.dev`, behind the Zitadel gate:
  [`components/traefik/README.md`](components/traefik/README.md),
  [`components/oauth2-proxy/README.md`](components/oauth2-proxy/README.md)
- **phpMyAdmin** - `https://phpmyadmin.d3strukt0r.dev`, behind the Zitadel gate, then a MariaDB
  login: [`components/phpmyadmin/README.md`](components/phpmyadmin/README.md)
- **pgAdmin** - `https://pgadmin.d3strukt0r.dev`, "Login with Zitadel", then a PostgreSQL role
  (`admin`): [`components/pgadmin/README.md`](components/pgadmin/README.md)
- **Zitadel console** - `https://auth.d3strukt0r.dev`:
  [`components/zitadel/README.md`](components/zitadel/README.md)
- **Status page** - `https://status.d3strukt0r.dev`, public, no login:
  [`components/gatus/README.md`](components/gatus/README.md)

## What an app pod must look like

Every app namespace enforces the **restricted** Pod Security Standard (configured in k3s,
see [`ansible/roles/k3s/README.md`](../ansible/roles/k3s/README.md)). A pod that breaks it is
rejected when it is created, and the error lists what is missing. Each container needs at least:

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

A container that sets no `resources` gets Kyverno's defaults (requests 50m CPU, 64Mi memory,
limit 128Mi memory, no CPU limit); one that needs more sets its own - "Default resources" in
[`components/kyverno/README.md`](components/kyverno/README.md).

## When a node dies

Running workloads on the other nodes carry on; what stops depends on the component, and
`node.kubernetes.io/out-of-service` is the taint that declares a node gone:

- Argo CD stops syncing until the node returns - "When a node dies" in
  [`components/argocd/README.md`](components/argocd/README.md).
- The dead node keeps its volumes until the taint - [`components/hcloud-csi/README.md`](components/hcloud-csi/README.md).
- A database failover can hang - "Failover hangs" in
  [`components/mariadb/README.md`](components/mariadb/README.md) and
  [`components/postgres/README.md`](components/postgres/README.md).
