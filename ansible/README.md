# ansible

Configures the running nodes. Where [`../cloud-init/`](../cloud-init/README.md) prepares a
node once at first boot, this keeps it configured from then on.

## Layout

```
ansible/
├── ansible.cfg
├── requirements.yml          # version floors; the ansible package already satisfies them
├── site.yml                  # the whole estate: imports the playbooks below
├── prod.yml                  # hosts: prod  ->  roles
├── kubeconfig.yml            # fetches the admin credential; not part of site.yml
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
    ├── k3s/
    │   ├── defaults/main.yml # version and server flags
    │   └── tasks/
    │       ├── main.yml      # detect iface, assemble flags, install, wait
    │       ├── server_init.yml
    │       └── server_join.yml
    └── swap/
        ├── defaults/main.yml # swap_size and the three sysctls
        └── tasks/main.yml    # the six tasks
```

**Inventory decides membership, the playbook's `hosts:` decides what that means.** There
are no feature flags. A role either appears in a playbook's `roles:` list or it does not.

Both inventory sources live in one directory and load together, so `prod` and `home` are
groups in a single inventory rather than separate inventories. That is what makes a
default inventory safe: `prod.yml` says `hosts: prod`, so it cannot touch a home server no
matter what is passed. The protection is structural rather than a habit of typing `-i`.

The corollary: **do not write a playbook with `hosts: all`**, or that guarantee is gone.

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

## Prerequisites

```shell
brew install ansible
```

That is enough. The `ansible` package bundles every collection used here -
`ansible.posix` (sysctl, mount), `community.general` (filesize) and `hetzner.hcloud` (the
dynamic inventory). `requirements.yml` records the versions this was written against, and
can pin them under `.collections/` if you ever want reproducibility:

```shell
ansible-galaxy collection install -r requirements.yml
```

Host key checking is **on** - these nodes are on the public internet. Seed
`known_hosts` once:

```shell
ssh-keyscan 178.104.135.61 178.104.133.199 78.47.68.27 >> ~/.ssh/known_hosts
```

`hcloud_token` must be filled in in `../tofu/hcloud/terraform.tfvars`; the inventory reads
it from there rather than keeping a second copy.

## Usage

Run from this directory - `ansible.cfg` is only read from the current directory, and the
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

## The prod inventory

Dynamic, so the three addresses are not copied out of `../tofu/hcloud/locals.tf` a third
time and adding a node needs no edit here. It selects by the same `cluster=prod` label the
firewall attaches by, and puts the results in the **`prod` group** rather than the plugin's default
`hcloud` - the group name should say what the hosts are, not where they are hosted.

`network: prod` filters to nodes on the private network and exposes each node's
`hcloud_private_ipv4`. That is a value the cluster is configured with, **not** how Ansible
connects - the private network is internal to Hetzner, so a laptop cannot route to
`10.0.0.0/24`. Connections go over the public address on port 22.

## The `hostname` role

Runs before `k3s` in both plays of `prod.yml`. cloud-init sets each node's hostname and
`/etc/hosts` on every boot from Hetzner's metadata - but the metadata keeps the name a
server was *created* with, so renaming a server in tofu never reaches the host by itself.
This role writes `/etc/cloud/cloud.cfg.d/90-hostname.cfg` with `hostname:` set to the
inventory name (the current Hetzner name), which cloud-init then uses instead, and applies
it immediately so the k3s role, which takes the node name from the hostname at install
time, already sees it. Tagged `hostname`, so it can run on its own:
`ansible-playbook prod.yml --tags hostname`.

## The `k3s` role

Brings up a three-server cluster with embedded etcd. Every node runs the control plane and
schedules workloads, so losing one is survivable.

`prod.yml` is two plays, which is forced rather than stylistic: `--cluster-init` has to
finish on one node before the others can join, joining uses `serial: 1` so etcd keeps a
quorum while members are added, and `hosts:` and `serial:` are play-level settings a role
cannot change.

`prod-01` appears literally in the host patterns. A play's pattern is resolved before any
host is selected, so inventory variables are not available there - templating `hosts:`
from `group_vars` fails outright. `k3s_init_node` in `group_vars/prod.yml` names the same
node and has to be kept in step with it. The role has no default for it on purpose: it
names a host, which is inventory data.

The playbook touches nothing but the nodes. Getting a kubeconfig onto your machine is
`kubeconfig.yml`, below.

Install tasks are guarded by `creates: /usr/local/bin/k3s`. The trade is that editing
`defaults/main.yml` afterwards changes nothing on a running node - which is why
`--secrets-encryption` has to be set at install time, since it cannot be enabled later
without restarting every server.

Settings that may change on a running cluster live in `k3s_config` instead. The role writes
them to `/etc/rancher/k3s/config.yaml.d/50-ansible.yaml` before installing, so a new node
starts with them, and when the file changes on a node that already runs k3s it restarts k3s
there and waits until the API answers again. With `serial: 1` that is one server at a time,
so etcd never loses its quorum. Today it holds `disable: [local-storage]`: k3s's local-path
provisioner kept volumes on the node's own 40 GB disk, where they would die with the node.

A dry run cannot cover everything: the join needs a token only a real first play produces,
so it is skipped under `--check`. What it does verify is connectivity, private-interface
detection and flag assembly on all three nodes.

## `argocd.yml` and the `argocd` role

Argo CD cannot deploy itself the first time, so this installs it once from
`../kubernetes/components/argocd/` and applies
`../kubernetes/clusters/<cluster_name>/root.yaml`, where `cluster_name` comes from
`inventories/group_vars/prod.yml`. From then on Argo CD manages itself and everything else
from git, and this finds it installed and does nothing - the same bootstrap-once trade the
k3s role makes with `creates:`.

It runs **from your machine**, not on the nodes: `kubectl` with the admin kubeconfig from
`kubeconfig.yml` (path in `admin_kubeconfig`), applying straight from the working copy.
So it runs after `kubeconfig.yml`, and only from an address in `admin_ips` - port 6443 is
closed to everything else. If the API is unreachable, the first task fails with
`kubectl`'s own error rather than mistaking it for "not installed". The play targets
`prod-01` only to pick up the group's variables; it makes no SSH connection.

That guard is deliberate in both directions. Ansible must never re-apply those manifests
once Argo CD owns them: a working copy that differs from `master` would be fighting Argo
CD's self-heal. And on a rebuilt cluster the guard is absent, so it bootstraps again.

**Push before running it.** The root Application syncs from GitHub, not from your working
copy. See [`../kubernetes/README.md`](../kubernetes/README.md) for what happens after the
bootstrap.

## `secrets.yml` and the `cluster_secrets` role

Writes the Secrets the cluster needs before OpenBao can hand out any - today the Hetzner
API token for the CSI driver (`kube-system/hcloud`), which OpenBao itself depends on for
its volumes. Each entry in `cluster_secrets` (`inventories/group_vars/prod.yml`) names a
Secret, a key, and the 1Password item and field its value comes from, in the account and
vault given by `onepassword_account` and `onepassword_vault`.

1Password is the root of trust for this bootstrap only: the values are read on your machine,
through the `op` CLI (expect a Touch ID prompt), and the cluster never talks to 1Password.
Like `argocd.yml` it runs from your machine with the admin kubeconfig, from an address in
`admin_ips`, and makes no SSH connection.

For each Secret it first runs `kubectl diff --server-side`, which only reads - so
`--check` shows the real answer - and applies only what differs, so a second run reports
`changed=0`. Values reach `kubectl` on stdin, never as arguments visible in the process
list, and every task is `no_log`. The apply is server-side because client-side apply would
store a second copy of the value in the `last-applied-configuration` annotation.

The Secrets are not in git, so Argo CD neither prunes nor overwrites them. Rotating one is
changing it in 1Password and re-running this.

## `kubeconfig.yml`

Separate from `prod.yml` and **not** imported by `site.yml`, because it does not configure a
server - it provisions a credential onto a workstation. Run it when you need the
credential:

```shell
ansible-playbook kubeconfig.yml
```

What it fetches is the **break-glass** credential, not an everyday one. The kubeconfig k3s
generates embeds a client certificate whose subject is `CN=system:admin, O=system:masters`,
and that group bypasses RBAC entirely - its access cannot be revoked by removing bindings.
So it lands as `~/.kube/d3strukt0r-prod-admin.yaml` with context `d3strukt0r-prod-admin`,
leaving the plain name free for a scoped identity later.

It goes beside `~/.kube/config` rather than into it, because the playbook writes the file
whole and would otherwise clobber an existing one. Chain them so kubectl sees both; writes
go to the first:

```shell
export KUBECONFIG="$HOME/.kube/config:$HOME/.kube/d3strukt0r-prod-admin.yaml"
```

Two things are rewritten on the way out of the node. The server address, from the node's
`127.0.0.1` to `prod-01`'s public address - all three nodes are in the certificate's SANs,
so if `prod-01` is down, editing the `server:` line to another node is enough. And the
cluster, user and context names, which k3s all calls `default`. Each replacement is
anchored to its key (`name: default`, not `default`) because base64 contains no colon or
space, so an anchored pattern cannot match inside the certificate blobs.

**Re-run it periodically.** k3s issues 365-day certificates and renews them on startup
within 120 days of expiry, on the node. This copy does not follow, so it eventually stops
working until refreshed.

## The `swap` role

**Written and verified, but not applied.** No playbook references it. Nothing runs on
these nodes yet, so there is no memory pressure to measure and any tuning value would be
guesswork.

To enable it, add it to the `roles:` list of the first two plays in `prod.yml`:

```yaml
  roles:
    - swap
    - role: k3s
```

That is the whole decision, which is why there is no `swap_enabled` flag to set to false
and then wonder about later. k3s was installed with
`--kubelet-arg=fail-swap-on=false`, so kubelet will not object.

Provisions a 2 GB swap file and the kernel tuning that makes swap safe on a Kubernetes
node. Values follow the *Recommended starting point* in the Kubernetes
[swap deep-dive](https://kubernetes.io/blog/2025/08/19/tuning-linux-swap-for-kubernetes-a-deep-dive/),
not the values it used while experimenting:

| Setting | Value | Why |
|---|---|---|
| `vm.swappiness` | 60 | The kernel default. Set explicitly to record intent - this is not a change |
| `vm.min_free_kbytes` | 3% of RAM | Upper end of the blog's "2-3% of total node memory", scaled rather than hardcoded so one role serves a 4 GB cx23, an 8 GB cx33 and the home servers |
| `vm.watermark_scale_factor` | 2000 | The value that actually changes behaviour - widens the reclaim window so kswapd can page out before the node hits a critical state |

`min_free_kbytes` is derived from what the kernel reports, which is well under the
nominal size, so it is not a round number:

| Node | Reported RAM | `min_free_kbytes` |
|---|---|---|
| `prod-03` (cx23) | 3826 MB | 117534 (~115 MiB) |
| `prod-01`, `prod-02` (cx33) | 7757 MB | 238295 (~233 MiB) |

Check it against the node rather than a fixed figure:

```shell
echo "want $(awk '/MemTotal/{printf "%d", $2*0.03}' /proc/meminfo), got $(sysctl -n vm.min_free_kbytes)"
```

If OOM kills still happen under real load, `min_free_kbytes` is the number to raise. The
blog's own test used far more than its recommendation, and its recommendation contradicts
its own test node size - so benchmark rather than trusting either.

### On swappiness

Worth knowing before changing `swap_swappiness` from the default 60, because it is one of
the most mythologised knobs in Linux.

It is **not** an eagerness dial. The kernel defines it as the relative IO cost of swapping
versus filesystem paging, on a 0-200 scale where 100 means the two are equally expensive
(`anon_prio = swappiness`, `file_prio = 200 - swappiness`). A low value asserts that swap
IO is far costlier than file IO - true of spinning disks, false here, where swap and the
filesystem share the same local NVMe.

Low values also do not avoid disk IO. They shift the thrashing from anonymous pages to the
page cache: instead of writing out cold anonymous memory once, the kernel repeatedly
evicts and re-reads binaries, libraries and cached data. That can be slower, and can help
cause the contention it was meant to avoid.

If a low value is ever wanted, **1 is the floor, not 0**: since a 2012 vmscan change, 0
refuses to scan anonymous pages at all until severe contention.

The hardware here would justify something nearer 100. It stays at 60 because an unmeasured
100 is no better founded than an unmeasured 1 - measure first.

**Swap is not encrypted.** It would only protect paged-out memory against someone reading
the disk offline, and on the same unencrypted root filesystem sit the etcd datastore with
every Kubernetes Secret, the cluster CA private keys, the join token and a cluster-admin
kubeconfig. Encrypting swap alone shutters a window beside an open door; the answer to
that threat is full-disk encryption, which on Hetzner costs unattended reboots.

## Not here yet

- **A `common` role.** One real role is enough to prove the layout.
- **The home fleet's hosts.**
