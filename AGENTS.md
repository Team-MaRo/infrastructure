# AGENTS.md

Guidance for AI coding agents (Claude Code, Codex, OpenCode, Cursor, …) working in this
repository. Claude Code reads `CLAUDE.md` instead, which is not in git: create it once per
checkout as a one-line import of this file:

```sh
printf '@AGENTS.md\n' > CLAUDE.md
```

Path-scoped rules for Claude Code are in `.claude/rules/`; they repeat the rules below for the
directory being edited.

## What this is

Infrastructure for a small Kubernetes cluster, `prod`, on Hetzner Cloud
(`Team-MaRo/infrastructure`), running the k3s distribution. Three nodes in nbg1, private
network, one public firewall. There is no application code here - everything is
declarative infrastructure.

- `tofu/` - the cloud resources, one root module per directory ([`tofu/README.md`](tofu/README.md))
- `cloud-init/node.yaml` - node bootstrap, consumed over HTTPS ([`cloud-init/README.md`](cloud-init/README.md))
- `ansible/` - configures the running nodes, installs k3s, bootstraps Argo CD ([`ansible/README.md`](ansible/README.md))
- `kubernetes/` - what runs on the cluster, deployed by Argo CD from `master` ([`kubernetes/README.md`](kubernetes/README.md))

Each of the six tofu directories is a **separate root module with its own state key**
in the same bucket. They are deliberately independent: none can break another's plan,
and each needs only its own credentials.

The layers run in order and do not reach back: OpenTofu creates a node, cloud-init
prepares it once as it boots, Ansible configures it from then on, and Argo CD deploys onto
the cluster from `kubernetes/`. Each directory has its own README; the root one is only an
index. **Before changing files under a directory, read the `README.md` of that directory (and
of its component or role); update it in the same change.** The index is at the end of this
file.

## Commands

No credential is ever exported by hand, and no value belongs in a `.tf` file: each module
reads its own token from a gitignored `terraform.tfvars`, and every backend reads the Object
Storage keys from the `d3strukt0r-hetzner` profile in `~/.aws/credentials`. If a plan cannot
reach the state bucket, check that file first. The credentials model, the state bucket and
restoring an old state are in `tofu/README.md`.

```sh
cd tofu/hcloud         # or tofu/objectstorage, tofu/openbao, tofu/zitadel, tofu/infomaniak, tofu/cloudflare
tofu fmt            # must produce no output
tofu validate
tofu init
tofu plan
```

`tofu init -backend=false` initialises providers only and needs no credentials -
useful for `fmt`/`validate` when the S3 keys are not to hand.

Read-only inspection of the live project uses the `hcloud` CLI, which is configured
with its own token (context `d3strukt0r-infrastructure`), independent of
`HCLOUD_TOKEN`:

```sh
hcloud server list -o json
hcloud firewall list -o json
```

Ansible runs from `ansible/`, where `ansible.cfg` sets the default inventory:

```sh
cd ansible
ansible-inventory --graph          # prod with three hosts, home empty
ansible-playbook prod.yml --check --diff
```

Applies and playbook runs that change something are the admin's to run, not an agent's.

### Rules for OpenTofu in this repo

- **Never run `tofu destroy`.**
- **Never `tofu apply` a plan that is not clean.** A clean plan is
  `0 to add, 0 to change, 0 to destroy`. If a plan shows `forces replacement`,
  `must be replaced` or `will be destroyed`, that is a bug in the config - fix the
  config and re-plan.
- Adoption of existing resources goes through `import` blocks in the module's `imports.tf`,
  not `tofu import` CLI calls, so it stays reviewable in git.

### Rules for Ansible in this repo

Details are in `ansible/README.md` and each role's `ansible/roles/<role>/README.md`.

- Run **from the `ansible/` directory** - `ansible.cfg` is only read from the current
  directory, and the inventory's token lookup uses a path relative to it.
- **Inventory decides membership, the playbook's `hosts:` decides what that means, and there
  are no feature flags.** A default inventory is set, which is safe only because `hosts:`
  constrains each playbook. **Do not write a playbook with `hosts: all`**, or that guarantee
  is gone.
- `prod.yml` runs the k3s role in two plays, with `prod-01` written out literally in the host
  patterns and kept in step with `k3s_init_node` - see `ansible/roles/k3s/README.md`.
- `kubeconfig.yml`, `argocd.yml` and `secrets.yml` run from the admin's machine and are not
  imported by `site.yml`.

### cloud-init

`user_data` is a two-line `#include` of `cloud-init/node.yaml` on `master`, so editing it
changes only nodes created or rebuilt afterwards, never running ones. **`cloud-init/k3s.yaml`
must stay** until `prod-01..03` are gone, and it must hold no comments - every line of an
`#include` file is read as a URL. Details in [`cloud-init/README.md`](cloud-init/README.md).

## Design principles

- **As little manual intervention as possible.** The cluster is run by one admin with limited
  operations experience: prefer a self-healing design, even at the cost of an extra component,
  as long as that component needs no care itself. Before recommending a tool, look for open
  bugs in the failure paths it would be trusted with. Every manual step that remains gets a
  written runbook in the README of its directory, and anything that can fail silently gets an
  alert.
- **Only open-source, self-hostable services.** A hosted instance of a self-hostable tool is
  fine as a start (ntfy.sh for alerts today); SaaS-only services are not (notifications go to
  ntfy, email or Matrix - not Telegram, Discord or Slack). No deeper coupling to Cloudflare
  than DNS, since DNS may move in-house one day.
- **Use the tool's own mechanism** before adding another program: a module's
  `terraform.tfvars`, the backend's AWS profile, a chart value. No sourced env files or extra
  tools just to hold credentials.

## Conventions

**Every reference to a 1Password item carries its title and its link** (user rule,
2026-09-30), so either finds it: in Markdown `` [`Title`](link) ``, in code comments the title
and the link on the line below; values the code uses (`item:` in group_vars, titles in `op`
commands) stay titles, since `op` finds items by title. That covers the docs, the
`terraform.tfvars.example` files (and the local tfvars), `variables.tf`, group_vars and every
ExternalSecret whose OpenBao value comes from 1Password. The link is the app's "copy link",
built from IDs only:
`op item get '<title>' --account my.1password.com --format json | jq -r --arg a "$(op account list --format json | jq -r '.[] | select(.url=="my.1password.com") | .account_uuid')" '"https://start.1password.com/open/i?a=\($a)&v=\(.vault.id)&i=\(.id)&h=my.1password.com"'`.
It opens only for someone signed in with access to the vault; it exposes account, vault and
item IDs (accepted) and **goes stale when an item is recreated** - regenerate its links then.

**Every directory documents itself in its `README.md`**, updated in the same change: a tofu
module, an Ansible role, a Kubernetes component or cluster subdirectory. The README holds the
design, every setting that differs from upstream and why, and the runbooks; this file holds
only what applies across directories.

**Things are named after what they are** - the `prod` cluster and its nodes - not after the
tool: `k3s` appears only where something genuinely is k3s (the Ansible role that installs it,
its paths and flags).

**An app keeps its full name everywhere** - namespace, bucket, database, OpenBao paths,
1Password titles, Terraform names: `wedding-manuele-robine`, never a shortened `wedding` (user
rule, 2026-10-02). Where a name cannot hold a hyphen, the hyphens become underscores
(`wedding_manuele_robine`). The app's own 1Password items carry its display name
(`Wedding Manuele Robine | Prod | App`); items of a cluster service keep the technical name
(`MariaDB | Prod | wedding-manuele-robine`).

**Every `kubectl` command names its context** (`--context d3strukt0r-prod-admin`, or
`d3strukt0r-prod` through Zitadel) - the admin's current context may be another cluster.

### Kubernetes

- **Pushing to `master` is deploying**, pruning included; branch protection is a security
  control. Never re-apply `kubernetes/` manifests from Ansible or by hand while Argo CD runs -
  a working copy that differs from `master` fights its self-heal
  ([`kubernetes/components/argocd/README.md`](kubernetes/components/argocd/README.md)).
- **`kubernetes/clusters/<name>/` decides what runs where; `kubernetes/components/<app>/`
  holds how**, shared by clusters. A cluster that needs something different gets a Kustomize
  overlay in its own directory, not a copy of the component. Components are Kustomize over a
  pinned upstream manifest; a Helm-only upstream gets its chart pinned in the cluster's
  Application and its values from `components/<app>/` as a second source (a third for extra
  manifests).
- **The Application finalizer** `resources-finalizer.argocd.argoproj.io` decides whether
  deleting a file from `clusters/<name>/` deletes what the Application deployed; without it
  only the Application goes and its resources keep running, untracked.
  - It is on apps whose data lives elsewhere or does not matter: `gatus`, `zitadel`,
    `oauth2-proxy`, `phpmyadmin`, `pgadmin`, `reloader`, `keel`, `wedding-manuele-robine`,
    `robines-portfolio`, `kured`, `alloy`, `etcd-snapshots`, `cluster-rbac`,
    `argocd-integrations`. A Postgres app's database and role stay when it goes
    (CloudNativePG's reclaim policies default to `retain`); a MariaDB app's objects set
    `cleanupPolicy: Skip`, since mariadb-operator's default is `Delete`.
  - **Never on an app that brings CRDs** (deleting a CRD deletes every object of its kind):
    cert-manager, external-secrets, kyverno, cloudnative-pg, the mariadb-operator ones,
    system-upgrade-controller, kube-prometheus-stack, kubeelasti, trust-manager. **Never on an
    app with a volume** (openbao, mariadb, postgres, loki, kube-prometheus-stack). **Never on the
    foundation**: root, private, argocd, hcloud-csi, traefik.
  - An app with the finalizer brings its own `namespace.yaml` instead of `CreateNamespace`,
    since Argo CD never deletes a namespace it created that way. Apps in `kube-system` get no
    `namespace.yaml`, or removing them would delete `kube-system`.
  - Details: "Pruning and the finalizer" in
    [`kubernetes/components/argocd/README.md`](kubernetes/components/argocd/README.md).
- **Render a chart with our values before trusting a key** (`helm template` with the pinned
  version and `components/<app>/values.yaml`): chart docs and READMEs drift from what the
  templates actually read.
- **Large CRDs need `ServerSideApply=true`** on the Application; operators that default fields
  in their objects need `ServerSideDiff=true`, or the app stays OutOfSync. A ServiceMonitor,
  PodMonitor or PrometheusRule outside kube-prometheus-stack carries
  `SkipDryRunOnMissingResource=true`, since its CRD belongs to another Application.
- **Every namespace runs the restricted Pod Security Standard** (see "Pod Security" below).
- **Default resources come from Kyverno's LimitRange** (requests 50m CPU, 64Mi memory, 50Mi
  ephemeral storage; limits 128Mi memory, 1Gi ephemeral storage). **No CPU limits**, anywhere:
  they throttle an idle node. A container sets its memory itself only where the defaults do
  not fit, and always in `kube-system` and `system-upgrade`, which have no LimitRange
  ([`kubernetes/components/kyverno/README.md`](kubernetes/components/kyverno/README.md)).
- **Images come only from `docker.io`, `ghcr.io`, `quay.io`, `registry.k8s.io` and
  `public.ecr.aws`** (Kyverno's `allowed-registries`); a new registry is one entry there.
- **Secret values never enter git or OpenTofu state.** They go into OpenBao with
  `bao kv put` (JSON on stdin) and reach workloads through an ExternalSecret against the
  `ClusterSecretStore` `openbao`
  ([`kubernetes/components/external-secrets/README.md`](kubernetes/components/external-secrets/README.md),
  [`tofu/openbao/README.md`](tofu/openbao/README.md)). Only the bootstrap Secrets come from
  1Password through `ansible/secrets.yml`.
- **An app that reads a Secret or ConfigMap only at start opts in to Reloader**: the annotation
  `secret.reloader.stakater.com/reload: <name>` (or `configmap.`) on its workload and its
  namespace in `reloader.namespaces` - External Secrets updates Secrets in place, so nothing
  else restarts it after a change in OpenBao. Never on operator-managed databases or OpenBao
  ([`kubernetes/components/reloader/README.md`](kubernetes/components/reloader/README.md)).
- **An app on a floating tag** (`:latest`, `:1`) opts in to Keel: `keel.sh/policy: force`,
  `keel.sh/matchTag: "true"` and `keel.sh/trigger: poll` on its workload, `imagePullPolicy:
  Always`, and a Docker Hub image written **without** `docker.io/` - Keel writes the short form
  back, and the long one in git would make self-heal roll it out twice
  ([`kubernetes/components/keel/README.md`](kubernetes/components/keel/README.md)).
- **The databases are shared**: one PostgreSQL (CloudNativePG) and one MariaDB, a database per
  app. An app's database objects live in the app's own component with `namespace: postgres`
  (`DatabaseRole`/`Database`) or `namespace: mariadb` (`User`/`Grant`/`Database`); the database's
  CA reaches the app through a label on its namespace (trust-manager)
  ([`kubernetes/components/postgres/README.md`](kubernetes/components/postgres/README.md),
  [`kubernetes/components/mariadb/README.md`](kubernetes/components/mariadb/README.md)).
- **Persistent data is on Hetzner Volumes** (`hcloud-volumes`, `Retain`), never a node disk;
  consolidate at the service, not the disk
  ([`kubernetes/components/hcloud-csi/README.md`](kubernetes/components/hcloud-csi/README.md)).

### Pod Security

Every namespace runs under the **restricted** Pod Security Standard, enforced by the API
server itself (Pod Security Admission) - not by Kyverno. A webhook policy fails open when
Kyverno is down; the built-in admission cannot be down while the API server is up. The k3s
role configures it (`psa.yaml`, `k3s_psa_exempt_namespaces`) - see "Pod Security admission"
in `ansible/roles/k3s/README.md`.

- **Restricted means**, for every container: `runAsNonRoot`, `allowPrivilegeEscalation:
  false`, `capabilities.drop: [ALL]` (only `NET_BIND_SERVICE` may be added back),
  `seccompProfile: RuntimeDefault`, and no host namespaces, host paths or privileged mode.
  A pod that misses one is rejected at creation, with the reasons in the error.
- **Infrastructure namespaces are exempt** (`k3s_psa_exempt_namespaces` in
  `ansible/roles/k3s/defaults/main.yml`). A new infrastructure component that needs more than
  the restricted standard goes onto that list in the commit that deploys it. One app is on it as
  a temporary exception with a TODO (`wedding-manuele-robine`, root images); apps otherwise
  never are.
- **The namespace label can override the default.** A namespace labelled
  `pod-security.kubernetes.io/enforce: baseline` (or `privileged`) gets that instead, so
  whoever may edit namespaces - today only the admin - can loosen it. Prefer adding a
  namespace to the exemption list over a label, so every exception stays in one reviewed
  place.
- The Kyverno default-resources exclusions and `k3s_psa_exempt_namespaces` are independent:
  one is about default resources, the other about privileges.

## The night's maintenance order

Everything that restarts or changes a node happens at night and is done by 06:00 Zurich
time, in windows that never overlap - so an upgrade and a reboot can never take out two
etcd members at once. All times are Europe/Zurich; the nodes' clocks stay on UTC.

| Time | What |
|---|---|
| 02:00 (01:00 in winter) | etcd snapshot (00:00 UTC, k3s's schedule) |
| 02:30-03:30 | k3s upgrades (system-upgrade-controller Plan window) |
| 03:30 + up to 15 min | OS updates (unattended-upgrades) |
| 04:30-06:00 | reboots by kured, only where a kernel update asks for one |

Each window is described with its owner: etcd snapshots in
[`kubernetes/clusters/prod/etcd-snapshots/README.md`](kubernetes/clusters/prod/etcd-snapshots/README.md),
k3s upgrades in
[`kubernetes/components/system-upgrade-controller/README.md`](kubernetes/components/system-upgrade-controller/README.md),
OS updates in [`ansible/roles/os_updates/README.md`](ansible/roles/os_updates/README.md),
reboots in [`kubernetes/components/kured/README.md`](kubernetes/components/kured/README.md).
A new scheduled job that touches nodes must fit between them.

## What a second cluster would need

`kubernetes/` is already laid out per cluster, because once Argo CD is bootstrapped its
paths are live and moving them risks pruning. Names are per cluster too - servers,
network, placement group, firewall, label, Ansible group and kubeconfig all say `prod`.
What is still single-cluster is the *structure*, deliberately left until a second cluster
is actually coming, since nothing live depends on it:

- **Inventory.** `inventories/hcloud.yml` selects `cluster=prod` into the `prod` group. A
  second cluster is a second source file selecting `cluster=<name>` into its own group,
  with its own `group_vars/<name>.yml` (`cluster_name`, `k3s_init_node`,
  `admin_kubeconfig`). A second cluster's nodes cannot leak into `prod`, because the label
  differs.
- **Playbooks.** `prod.yml`, `kubeconfig.yml` and `argocd.yml` name `prod-01` literally in
  their host patterns, so each cluster needs its own copies or the plays need
  restructuring.
- **`tofu/hcloud/`** has one network, placement group and firewall, all for `prod`, as
  single resources. A second cluster in the same project means turning those into
  per-cluster maps (or a local module), with a `moved` block per address.

## Index

Before changing files under a directory, read the `README.md` of that directory (and of its
component); update it in the same change.

| README | What it covers |
|---|---|
| [`tofu/README.md`](tofu/README.md) | Credentials model, state bucket and backend, restoring an old state |
| [`tofu/hcloud/README.md`](tofu/hcloud/README.md) | Servers, network, firewall, placement group; adopted resources, destroy protection |
| [`tofu/objectstorage/README.md`](tofu/objectstorage/README.md) | Buckets, lifecycle rules, bucket policies scoping each S3 key |
| [`tofu/openbao/README.md`](tofu/openbao/README.md) | OpenBao's engine, Kubernetes auth, policies, OIDC login; putting values in |
| [`tofu/zitadel/README.md`](tofu/zitadel/README.md) | Zitadel's policies, domains, projects, roles, apps and the groups claim |
| [`tofu/infomaniak/README.md`](tofu/infomaniak/README.md) | Registrar delegation and DNSSEC checks, drift correction |
| [`tofu/cloudflare/README.md`](tofu/cloudflare/README.md) | Both Cloudflare accounts, zones, records, CAA and TLS settings |
| [`cloud-init/README.md`](cloud-init/README.md) | Node bootstrap at first boot; why `k3s.yaml` still exists |
| [`ansible/README.md`](ansible/README.md) | Layout, selection model, inventories, playbooks, the two kubeconfigs |
| [`ansible/roles/k3s/README.md`](ansible/roles/k3s/README.md) | Installing k3s, its config drop-ins, Pod Security, Zitadel authentication |
| [`ansible/roles/hostname/README.md`](ansible/roles/hostname/README.md) | Pinning the hostname for cloud-init |
| [`ansible/roles/os_updates/README.md`](ansible/roles/os_updates/README.md) | Moving unattended-upgrades into the night's window |
| [`ansible/roles/argocd/README.md`](ansible/roles/argocd/README.md) | Bootstrapping Argo CD once |
| [`ansible/roles/cluster_secrets/README.md`](ansible/roles/cluster_secrets/README.md) | The bootstrap Secrets from 1Password |
| [`ansible/roles/swap/README.md`](ansible/roles/swap/README.md) | The unreferenced swap role and what its values claim |
| [`kubernetes/README.md`](kubernetes/README.md) | Layout, adding components and clusters, kubectl, UIs, what a pod must look like |
| [`kubernetes/components/argocd/README.md`](kubernetes/components/argocd/README.md) | Argo CD: pruning and the finalizer, installation, login, upgrades, runbooks |
| [`kubernetes/components/argocd-integrations/README.md`](kubernetes/components/argocd-integrations/README.md) | Argo CD's certificate, webhook secret and monitors |
| [`kubernetes/components/hcloud-csi/README.md`](kubernetes/components/hcloud-csi/README.md) | Hetzner Volumes, limits, `Retain`, removing released volumes |
| [`kubernetes/components/openbao/README.md`](kubernetes/components/openbao/README.md) | OpenBao: static seal, recovery keys, snapshots, restore |
| [`kubernetes/components/external-secrets/README.md`](kubernetes/components/external-secrets/README.md) | The `openbao` store and writing an ExternalSecret |
| [`kubernetes/components/system-upgrade-controller/README.md`](kubernetes/components/system-upgrade-controller/README.md) | k3s upgrades: channel, window, failed Jobs |
| [`kubernetes/components/kured/README.md`](kubernetes/components/kured/README.md) | Reboots after kernel updates, blocked drains |
| [`kubernetes/components/kyverno/README.md`](kubernetes/components/kyverno/README.md) | Default resources and allowed registries |
| [`kubernetes/components/traefik/README.md`](kubernetes/components/traefik/README.md) | Ingress on the host network, ingress nodes, the dashboard |
| [`kubernetes/components/cert-manager/README.md`](kubernetes/components/cert-manager/README.md) | DNS-01 issuers, the Cloudflare tokens, requesting a certificate |
| [`kubernetes/components/cluster-rbac/README.md`](kubernetes/components/cluster-rbac/README.md) | Cluster-wide rights for Zitadel logins, revoking |
| [`kubernetes/components/gatus/README.md`](kubernetes/components/gatus/README.md) | The status page, the Watchdog heartbeat, adding checks |
| [`kubernetes/components/kube-prometheus-stack/README.md`](kubernetes/components/kube-prometheus-stack/README.md) | Prometheus, Alertmanager, Grafana, scraping k3s, alerts |
| [`kubernetes/components/loki/README.md`](kubernetes/components/loki/README.md) | Log storage, retention, searching logs |
| [`kubernetes/components/alloy/README.md`](kubernetes/components/alloy/README.md) | Log collection on every node |
| [`kubernetes/components/mariadb-operator/README.md`](kubernetes/components/mariadb-operator/README.md) | The MariaDB operator, its CRDs and webhook |
| [`kubernetes/components/mariadb/README.md`](kubernetes/components/mariadb/README.md) | The shared MariaDB: failover, backups, PITR, runbooks |
| [`kubernetes/components/cloudnative-pg/README.md`](kubernetes/components/cloudnative-pg/README.md) | The PostgreSQL operator and the Barman Cloud plugin |
| [`kubernetes/components/postgres/README.md`](kubernetes/components/postgres/README.md) | The shared PostgreSQL: adding an app, failover, restore |
| [`kubernetes/components/zitadel/README.md`](kubernetes/components/zitadel/README.md) | The identity provider, its database, keys and groups webhook |
| [`kubernetes/components/oauth2-proxy/README.md`](kubernetes/components/oauth2-proxy/README.md) | The login gate for UIs without their own Zitadel login |
| [`kubernetes/components/phpmyadmin/README.md`](kubernetes/components/phpmyadmin/README.md) | The shared MariaDB's web UI, its configuration storage |
| [`kubernetes/components/pgadmin/README.md`](kubernetes/components/pgadmin/README.md) | The shared PostgreSQL's web UI, its Zitadel login and configuration database |
| [`kubernetes/components/reloader/README.md`](kubernetes/components/reloader/README.md) | Restarting apps when a Secret or ConfigMap they read at start changes |
| [`kubernetes/components/trust-manager/README.md`](kubernetes/components/trust-manager/README.md) | The databases' CA certificates in the apps' namespaces |
| [`kubernetes/components/keel/README.md`](kubernetes/components/keel/README.md) | Rolling out new images under floating tags, opting an app in |
| [`kubernetes/components/wedding-manuele-robine/README.md`](kubernetes/components/wedding-manuele-robine/README.md) | The wedding website; its two TODOs; moving it from prod-old |
| [`kubernetes/components/robines-portfolio/README.md`](kubernetes/components/robines-portfolio/README.md) | The old WordPress portfolio: stock images, plugins at start, uploads behind a signing proxy, WP-Cron |
| [`kubernetes/components/kubeelasti/README.md`](kubernetes/components/kubeelasti/README.md) | Scale to zero, letting an app sleep |
| [`kubernetes/clusters/prod/etcd-snapshots/README.md`](kubernetes/clusters/prod/etcd-snapshots/README.md) | etcd snapshots to Object Storage, restoring one |
