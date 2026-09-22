# ansible

Configures the running nodes. Where [`../cloud-init/`](../cloud-init/README.md) prepares a
node once at first boot, this keeps it configured from then on.

## Layout

```
ansible/
├── ansible.cfg
├── requirements.yml          # version floors; the ansible package already satisfies them
├── site.yml                  # the whole estate: imports the playbooks below
├── k3s.yml                   # hosts: k3s  ->  roles
├── inventories/
│   ├── hcloud.yml            # dynamic, Hetzner API, group: k3s
│   ├── home.yml              # static, group: home (no hosts yet)
│   └── group_vars/
│       ├── k3s.yml
│       └── home.yml
└── roles/
    └── swap/
        ├── defaults/main.yml # swap_size and the three sysctls
        └── tasks/main.yml    # the six tasks
```

**Inventory decides membership, the playbook's `hosts:` decides what that means.** There
are no feature flags. A role either appears in a playbook's `roles:` list or it does not.

Both inventory sources live in one directory and load together, so `k3s` and `home` are
groups in a single inventory rather than separate inventories. That is what makes a
default inventory safe: `k3s.yml` says `hosts: k3s`, so it cannot touch a home server no
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
| `playbook.yml` | `k3s.yml`, or `site.yml` to run every type |

The variables moved for one reason: precedence. Ansible ranks a playbook's `vars:` block
*above* inventory (12 vs 4), so values kept there cannot be overridden per group or host.
Role defaults are the weakest thing in Ansible (rank 2), so `group_vars/k3s.yml` can set
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

`hcloud_token` must be filled in in `../tofu/k3s/terraform.tfvars`; the inventory reads it
from there rather than keeping a second copy.

## Usage

Run from this directory - `ansible.cfg` is only read from the current directory, and the
inventory's token lookup uses a path relative to it.

```shell
ansible-inventory --graph        # k3s with three hosts, home empty
ansible k3s -m ansible.builtin.ping

ansible-playbook k3s.yml --check --diff
ansible-playbook k3s.yml
```

`site.yml` runs everything. Running a single playbook configures one type, which is more
explicit than `--limit`.

Re-running is safe: every install task is guarded, so a second run reports `changed=0`.

## The k3s inventory

Dynamic, so the three addresses are not copied out of `../tofu/k3s/locals.tf` a third time
and adding a node needs no edit here. It selects by the same `role=k3s` label the firewall
attaches by, and puts the results in the **`k3s` group** rather than the plugin's default
`hcloud` - the group name should say what the hosts are, not where they are hosted.

`network: k3s` filters to nodes on the private network and exposes each node's
`hcloud_private_ipv4`. That is a value for k3s to be configured with, **not** how Ansible
connects - the private network is internal to Hetzner, so a laptop cannot route to
`10.0.0.0/24`. Connections go over the public address on port 22.

## The `swap` role

**Written and verified, but not applied.** No playbook references it. Nothing runs on
these nodes yet, so there is no memory pressure to measure and any tuning value would be
guesswork.

To enable it, uncomment the one line in `k3s.yml`:

```yaml
  roles:
    - swap
```

That single line is the whole decision, which is why there is no `swap_enabled` flag to
set to false and then wonder about later.

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
| `k3s-03` (cx23) | 3826 MB | 117534 (~115 MiB) |
| `k3s-01`, `k3s-02` (cx33) | 7757 MB | 238295 (~233 MiB) |

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

- **The k3s install.** When it happens it needs `--kubelet-arg=fail-swap-on=false`,
  because kubelet refuses to start on a swap-enabled node, and `--secrets-encryption`,
  which is free at install time and
  [cannot be enabled later without restarting every server](https://docs.k3s.io/security/secrets-encryption).
  Leave `swapBehavior` at its default `NoSwap`: swap then protects the node and system
  daemons while pods cannot touch it, which is the right trade on a 4 GB node.
- **Port 6443 is closed.** `kubectl` will need a firewall rule scoped to your own address
  before it can reach the API.
- **A `common` role.** One real role is enough to prove the layout.
- **The home fleet's hosts.**
