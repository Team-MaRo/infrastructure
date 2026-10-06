# ansible

Configures the running nodes. Where [`../cloud-init/`](../cloud-init/README.md) prepares a
node once at first boot, this keeps it configured from then on. This file covers what spans
the playbooks - layout, inventories, how to run them, and `kubeconfig.yml`, which has no role
of its own; each role's design and runbooks are in its own README:

| Role | Run by | README |
|---|---|---|
| `hostname` | `prod.yml` | [`roles/hostname/README.md`](roles/hostname/README.md) |
| `os_updates` | `prod.yml` | [`roles/os_updates/README.md`](roles/os_updates/README.md) |
| `k3s` | `prod.yml` | [`roles/k3s/README.md`](roles/k3s/README.md) |
| `swap` | nothing yet | [`roles/swap/README.md`](roles/swap/README.md) |
| `argocd` | `argocd.yml` | [`roles/argocd/README.md`](roles/argocd/README.md) |
| `cluster_secrets` | `secrets.yml` | [`roles/cluster_secrets/README.md`](roles/cluster_secrets/README.md) |

## Layout

```
ansible/
├── ansible.cfg
├── requirements.yml          # collection floors; install once (hetzner.hcloud 7 over the bundle)
├── site.yml                  # the whole estate: imports the playbooks below
├── prod.yml                  # hosts: prod  ->  roles
├── kubeconfig.yml            # writes the Zitadel and the admin kubeconfig; not part of site.yml
├── argocd.yml                # bootstraps Argo CD from this machine; not part of site.yml
├── secrets.yml               # writes bootstrap Secrets from 1Password; not part of site.yml
├── inventories/
│   ├── hcloud.yml            # dynamic, Hetzner API, group: prod
│   ├── home.yml              # static, group: home (no hosts yet)
│   └── group_vars/
│       ├── prod.yml
│       └── home.yml
└── roles/
    ├── argocd/
    │   └── tasks/main.yml    # bootstraps Argo CD once, then hands over to git
    ├── cluster_secrets/
    │   └── tasks/main.yml    # diffs and server-side applies Secrets from 1Password
    ├── hostname/
    │   └── tasks/main.yml    # pins the hostname to the inventory name, for cloud-init
    ├── os_updates/
    │   ├── defaults/main.yml # when unattended-upgrades installs
    │   └── tasks/main.yml    # a drop-in for apt-daily-upgrade.timer
    ├── k3s/
    │   ├── defaults/main.yml # version and server flags
    │   └── tasks/
    │       ├── main.yml      # detect iface, assemble flags, install, wait
    │       ├── server_init.yml
    │       ├── server_join.yml
    │       ├── agent_join.yml   # workers: k3s agent with the server token
    │       ├── wait_ready.yml   # a server's API answers
    │       ├── wait_node_ready.yml  # an agent's node is Ready, asked of prod-01
    │       └── labels.yml    # node labels via kubectl, on prod-01
    └── swap/
        ├── defaults/main.yml # swap_size and the three sysctls
        └── tasks/main.yml    # the six tasks
```

Every role directory also holds its `README.md`.

## The selection model

The selection model is the thing to understand: **inventory decides membership, the
playbook's `hosts:` decides what that means, and there are no feature flags.** A role
either appears in a playbook's `roles:` list or it does not. This follows the layout in
Ansible's own sample setup and in the Red Hat CoP good practices, both of which map a
group to roles in one playbook per type; neither uses conditional `*_enabled` gating.

- **One inventory directory, two sources.** `inventories/hcloud.yml` is dynamic (Hetzner
  API, `label_selector: cluster=prod`, `group: prod`) and `inventories/home.yml` is static.
  They load together, so `prod` and `home` are groups in a single inventory rather than
  separate inventories.
- **A default inventory is set**, which is safe only because `hosts:` constrains each
  playbook - `prod.yml` says `hosts: prod`, so it cannot touch a home server no matter what
  is passed. The protection is structural rather than a habit of typing `-i`.
- The corollary: **do not write a playbook with `hosts: all`**, or that guarantee is gone.
- **`site.yml` imports the per-type playbooks.** Run it for everything, or one playbook
  for one type. `kubeconfig.yml` is deliberately not among them.

### If you are used to one playbook.yml plus tasks/

The pieces are the same, just placed differently:

| Playbook-and-tasks | Here |
|---|---|
| `inventory.ini` | `inventories/hcloud.yml` and `inventories/home.yml` |
| the `vars:` block in `playbook.yml` | `roles/swap/defaults/main.yml` - only that role's vars |
| `tasks/swap.yml` | `roles/swap/tasks/main.yml` - the same tasks, unchanged |
| `include_tasks: tasks/swap.yml` | one entry in a playbook's `roles:` list |
| `playbook.yml` | `prod.yml`, or `site.yml` to run every type |

The variables moved for one reason: precedence. Ansible ranks a playbook's `vars:` block
*above* inventory (12 vs 4), so values kept there cannot be overridden per group or host.
Role defaults are the weakest thing in Ansible (rank 2), so `group_vars/prod.yml` can set
`swap_size: 8G` for one fleet and the same role serves both.

## The prod inventory

Dynamic, so the three addresses are not copied out of `../tofu/hcloud/locals.tf` a third
time and adding a node needs no edit here. It selects by the same `cluster=prod` label the
firewall attaches by, and puts the results in the **`prod` group** rather than the plugin's default
`hcloud` - the group name should say what the hosts are, not where they are hosted. Workers
carry the extra label `role=agent` and land in **`prod_agents`** too (a constructed group in
`inventories/hcloud.yml`); its group_vars make the k3s role install an agent.

**The prod inventory connects over public IPs.** `network: prod` filters to nodes on the
private network and exposes each node's `hcloud_private_ipv4` as a hostvar. That is a value
the cluster is configured with, **not** how Ansible connects - `ansible_host` stays the
public address, since the private network is internal to Hetzner and a laptop cannot route
to `10.0.0.0/24`. Connections go over the public address on port 22.

**Its token comes from `tofu/hcloud/terraform.tfvars`** via a `lookup`, rather than a
second copy or an environment variable. `HCLOUD_TOKEN` overrides it if that breaks.

## Prerequisites

```shell
brew install ansible
```

The `ansible` package bundles every collection used here - `ansible.posix` (sysctl, mount),
`community.general` (filesize) and `hetzner.hcloud` (the dynamic inventory). Install from
`requirements.yml` once anyway:

```shell
ansible-galaxy collection install -r requirements.yml
```

The bundled hetzner.hcloud is 6.12, which prints a `hcloud_datacenter` deprecation warning
for every server on every run - unconditionally, although nothing here uses that variable.
7.0 removed it, so `requirements.yml` requires `>=7.0.0`. The install goes to
`.collections/`, which takes precedence over the bundle.

Host key checking is **on** - these nodes are on the public internet. Seed
`known_hosts` once:

```shell
ssh-keyscan 178.104.135.61 178.104.133.199 78.47.68.27 >> ~/.ssh/known_hosts
```

`hcloud_token` must be filled in in `../tofu/hcloud/terraform.tfvars`; the inventory reads
it from there rather than keeping a second copy.

## Usage

Run **from this directory** - `ansible.cfg` is only read from the current directory, and the
inventory's token lookup uses a path relative to it.

```shell
ansible-inventory --graph        # prod with three hosts, home empty
ansible prod -m ansible.builtin.ping

ansible-playbook prod.yml --check --diff
ansible-playbook prod.yml
ansible-playbook kubeconfig.yml  # afterwards, to get a working kubectl
ansible-playbook argocd.yml      # then, once, to bootstrap Argo CD
ansible-playbook secrets.yml     # and whenever a bootstrap secret changes in 1Password
```

`site.yml` runs everything. Running a single playbook configures one type, which is more
explicit than `--limit`. `kubeconfig.yml`, `argocd.yml` and `secrets.yml` are not in it -
all three depend on this machine, not only on the nodes.

Re-running is safe: every install task is guarded, so a second run reports `changed=0`.

## The playbooks

- **`prod.yml`** - the `prod` cluster, in three plays: `hosts: prod-01` initialises the first
  server, `hosts: prod:!prod-01:!prod_agents` joins the other servers and `hosts: prod_agents`
  the workers, each with `serial: 1`. Each play runs
  [`hostname`](roles/hostname/README.md), [`os_updates`](roles/os_updates/README.md) and
  [`k3s`](roles/k3s/README.md); why it has to be three plays, and why `prod-01` is written out
  literally, is in the k3s role's README.
- **`site.yml`** - imports the per-type playbooks; today only `prod.yml`.
- **`kubeconfig.yml`** - writes the two kubeconfigs onto this machine; below, since it has no
  role.
- **`argocd.yml`** - bootstraps Argo CD once, from this machine; the
  [`argocd` role](roles/argocd/README.md).
- **`secrets.yml`** - writes the bootstrap Secrets from 1Password, from this machine; the
  [`cluster_secrets` role](roles/cluster_secrets/README.md).

## `kubeconfig.yml`

Two kubeconfigs: Zitadel every day, the admin certificate as break-glass.

`ansible/kubeconfig.yml` is its own playbook and is **not** imported by `site.yml`. Writing
into `~/.kube` through `delegate_to: localhost` provisions a credential onto a workstation;
it is not configuring a server, and it does not belong inside a role named after what it
installs on the nodes. It writes two files beside `~/.kube/config` (rather than into it,
because it writes each file whole and would otherwise clobber an existing one):

| Context | File | Logs in as |
|---|---|---|
| `d3strukt0r-prod` | `~/.kube/d3strukt0r-prod.yaml` | `zitadel:<username>`, through the browser - every day |
| `d3strukt0r-prod-admin` | `~/.kube/d3strukt0r-prod-admin.yaml` | `system:admin`, a client certificate - break-glass |

```shell
brew install kubelogin              # once; the everyday kubeconfig runs it
ansible-playbook kubeconfig.yml
kubectl --context d3strukt0r-prod auth whoami   # zitadel:<username>, group zitadel:infra-admin
```

Chain them so kubectl sees all three; writes go to the first:

```shell
export KUBECONFIG="$HOME/.kube/config:$HOME/.kube/d3strukt0r-prod.yaml:$HOME/.kube/d3strukt0r-prod-admin.yaml"
```

### The Zitadel kubeconfig

**The Zitadel kubeconfig holds no credential.** Its user runs kubelogin
(`brew install kubelogin`, called as `kubectl oidc-login get-token`), which opens the browser,
logs in with PKCE against the public app `Kubernetes` (`tofu/zitadel/apps_kubernetes.tf`,
redirects `http://localhost:8000` and `:18000` - a native app, so no development mode) and
keeps the tokens in the macOS keychain. It asks for `profile` (the username claim) and
`offline_access` (a refresh token, so the hourly ID token renews without the browser). Server
and CA are the admin file's. How the API server accepts those tokens (`authn.yaml`) is
"Authentication: Zitadel's ID tokens" in the [k3s role's README](roles/k3s/README.md).

What that login may do, and how revoking it works, is in
[`kubernetes/components/cluster-rbac/README.md`](../kubernetes/components/cluster-rbac/README.md).

### The admin kubeconfig

**The admin kubeconfig is not an everyday credential.** It is fetched from the node:
`/etc/rancher/k3s/k3s.yaml` embeds a client certificate whose subject is
`CN=system:admin, O=system:masters`, and per the Kubernetes RBAC guidance any member of that
group **"bypasses all RBAC rights checks and will always have unrestricted superuser access,
which cannot be revoked by removing RoleBindings or ClusterRoleBindings"**. No RBAC can limit
that file, which is why everyday access is a second identity rather than policy against this
one. It keeps working when Zitadel is down.

Two things are rewritten on the way out of the node. The server address, from the node's
`127.0.0.1` to `prod-01`'s public address - all three nodes are in the certificate's SANs, so
if `prod-01` is down, editing the `server:` line to another node (in both files) is enough.
And the cluster, user and context names, which k3s all calls `default`. The name rewrites are
anchored to their keys (`name: default`, not `default`) because base64 contains no colon or
space, so an anchored pattern cannot collide with the certificate blobs.

**Re-run it periodically** for the admin kubeconfig. It expires: k3s issues 365-day
certificates and renews them on startup within 120 days of expiry, on the node. This copy
does not follow, so `kubeconfig.yml` has to be re-run before it stops working.

## Not here yet

- **A `common` role.** One real role is enough to prove the layout.
- **The home fleet's hosts.**
