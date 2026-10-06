# k3s

Brings up a three-server cluster with embedded etcd. Every node runs the control plane and
schedules workloads, so losing one is survivable. Besides the install, the role owns the
k3s configuration that may change later (`k3s_config`), kernel parameters, node labels, the
Pod Security admission file and the API server's authentication configuration.

How the playbooks and the inventory fit together is in [`../../README.md`](../../README.md).
How the running cluster upgrades itself is in
[`kubernetes/components/system-upgrade-controller/README.md`](../../../kubernetes/components/system-upgrade-controller/README.md).

## Why it is two plays

`prod.yml` runs `roles/k3s` in two plays rather than one. That is forced, not stylistic:
`--cluster-init` has to finish on one node before the others can join, joining wants
`serial: 1` so etcd keeps a quorum while members are added, and `hosts:` and `serial:` are
play-level settings a role cannot change.

- **`prod-01` is written out literally** in the host patterns. A play's pattern is resolved
  before any host is selected, so inventory variables are not available to it - templating
  `hosts:` from `group_vars` fails with `'k3s_init_node' is undefined`.
  `k3s_init_node` in `group_vars/prod.yml` names the same node and has to be kept in step;
  the role deliberately has no default for it, since it names a host, which is inventory
  data.
- **The second play is `prod:!prod-01`**, not `all:!prod-01`, which would sweep in `home`.
- **The playbook touches nothing but the nodes.** Fetching a kubeconfig is
  `ansible/kubeconfig.yml`, deliberately separate - see "`kubeconfig.yml`" in
  [`../../README.md`](../../README.md).
- **Install tasks are guarded by `creates: /usr/local/bin/k3s`**, so a re-run changes
  nothing. The trade is that editing `k3s_server_args` afterwards has no effect on a
  running node - those flags have to be right the first time (see "Two flags the install
  has to set up front" below).
- **A dry run cannot cover all of it.** The join needs a token only a real first play
  produces, so it is skipped under `--check`. What a dry run does verify is connectivity,
  private-interface detection and flag assembly on all three nodes.

## `k3s_config`: settings that may change later

**Everything that may change later goes in `k3s_config` instead.** The role writes it to
`/etc/rancher/k3s/config.yaml.d/50-ansible.yaml` (keys as in k3s's config file) before the
install, so a new node starts with them, and on a node already running k3s a change restarts
it and waits for `readyz` before moving on - one node at a time, thanks to `serial: 1`, so
etcd never loses its quorum. k3s reads drop-ins even without a `config.yaml` (checked in its
source). The first use is `disable: [local-storage]`, which removed the local-path
provisioner and its StorageClass: node disks are 40 GB and hold the OS, and data there dies
with the node.

Besides that it holds the etcd snapshot upload to S3, the WireGuard
backend for the pod network (`flannel-backend: wireguard-native`, which encrypts pod traffic
between nodes), and the paths to the Pod Security and the authentication configuration - each
described below.

## Memory: eviction and reservation for k3s

**The kubelet's own settings go in `k3s_kubelet_config`**, which the role writes as a
`KubeletConfiguration` drop-in to `/var/lib/rancher/k3s/agent/etc/kubelet.conf.d/50-ansible.conf`
(k3s 1.32 and later read that directory; a change restarts one node at a time, as above). Not
as `kubelet-arg` in `k3s_config`: the installer put `--kubelet-arg=fail-swap-on=false` on the
command line, and k3s lets a command-line flag replace the whole list from its config files. The
kubelet reads k3s's `00-k3s-defaults.conf` first and ours after it; **every field ours names
replaces k3s's as a whole**, maps included.

What it sets, and why (measured 2026-10-06):

- **`evictionHard`**: k3s sets only `imagefs.available` and `nodefs.available` (5% each), which
  drops the kubelet's memory threshold - no pod was ever evicted for memory, and prod-03 went
  down to 78 MiB available. `memory.available: 200Mi` has the kubelet evict pods (those
  furthest over their requests first) before the kernel's OOM killer chooses, which could hit
  k3s or etcd. The two disk values are k3s's, repeated because the map is replaced.
- **`kubeReserved: 1536Mi` and `systemReserved: 256Mi`** take memory k3s itself and the OS use
  out of the node's allocatable, so the scheduler stops placing pods into it: allocatable drops
  from 7.6 GiB to about 5.6 GiB. Measured: k3s (API server, etcd, controllers, kubelet,
  containerd) holds 1.9-2.7 GiB, the rest of the OS including the kernel about 0.2 GiB. 1.5 GiB is
  k3s's baseline with embedded etcd ([resource profiling](https://docs.k3s.io/reference/resource-profiling));
  above that it grows mostly with the CRDs - 133 here, whose schemas the API server keeps parsed
  several times over - and the controllers watching them. Less than measured, so the largest
  node's pod requests keep fitting; raise it once oversized requests are trimmed. It only
  reserves: enforcing it (`enforceNodeAllocatable`) would put a hard memory limit on k3s and etcd.

Checking what a kubelet uses, and the nodes' allocatable memory:

```sh
kubectl --context d3strukt0r-prod-admin get --raw /api/v1/nodes/prod-01/proxy/configz \
  | jq '.kubeletconfig | {evictionHard, kubeReserved, systemReserved, failSwapOn}'
kubectl --context d3strukt0r-prod-admin get nodes \
  -o custom-columns=NAME:.metadata.name,CAPACITY:.status.capacity.memory,ALLOCATABLE:.status.allocatable.memory
```

## Kernel parameters and node labels

**Kernel parameters (`k3s_sysctls`) and node labels (`k3s_node_labels`)** are the role's
too. The sysctls go to `/etc/sysctl.d/50-ansible-k3s.conf` and apply at once, without a
restart; today only `net.ipv4.ip_unprivileged_port_start = 80`, so Traefik can listen on
80/443 on the host network without root. Labels are set with `k3s kubectl label` on the node
itself, after reading the current ones, because k3s's `node-label` only applies at a node's
first registration and a kubelet may not set `node-role.kubernetes.io/*` on itself. That uses
the admin kubeconfig every server has; an agent node would need the task delegated to a
server. `group_vars/prod.yml` labels every node `node-role.kubernetes.io/ingress=true` (see
[`kubernetes/components/traefik/README.md`](../../../kubernetes/components/traefik/README.md)).

## Two flags the install has to set up front

Both are in `roles/k3s/defaults/main.yml`, and both are install-time only:

- `--secrets-encryption`, which is free at install time and **cannot be enabled on an
  existing server without restarting it** - every server. Verify with
  `k3s secrets-encrypt status` on a node; it should report `Enabled`.
- `--kubelet-arg=fail-swap-on=false`, because kubelet refuses to start on a swap-enabled
  node. Swap is off today, so this currently changes nothing - it exists so the
  [swap role](../swap/README.md) can be enabled later without reinstalling k3s. Leave
  `swapBehavior` at its default `NoSwap`: swap then protects the node and system daemons
  while pods cannot use it. `LimitedSwap` only ever grants swap to Burstable pods anyway.

The cluster was installed at k3s `v1.37.0+k3s1` (Kubernetes 1.37, containerd 2.3.4) on
Debian 13 with kernel 6.12, and upgrades itself from there (see
[`kubernetes/components/system-upgrade-controller/README.md`](../../../kubernetes/components/system-upgrade-controller/README.md)). That combination clears every requirement for **per-pod
user namespaces** (`hostUsers: false`), which are GA and locked since Kubernetes 1.36 and need
Linux 6.3+, containerd 2.0+ and an idmap-capable filesystem. This is the answer to wanting
Podman's rootless property: k3s's own `--rootless` mode is experimental and, per its docs,
multi-node rootless clusters are unsupported, so it is a single-node mode and not an option
here.

The installer script is pinned as well: the role fetches `install.sh` from the release's
own tag (`raw.githubusercontent.com/k3s-io/k3s/<k3s_version>/install.sh`), not from
`get.k3s.io`, which always serves the latest script, and checks it against
`k3s_install_sha256` - it runs as root. **Bumping `k3s_version` means bumping
`k3s_install_sha256` too**; the command to compute it is next to the value in
`roles/k3s/defaults/main.yml`.

## Pod Security admission

Every namespace runs under the restricted Pod Security Standard, enforced by the API server
itself (what that means for a pod is "Pod Security" in [`AGENTS.md`](../../../AGENTS.md)).
`ansible/roles/k3s` writes `/etc/rancher/k3s/psa.yaml` (an `AdmissionConfiguration` with
defaults `enforce`, `audit` and `warn` = `restricted`, version `latest`) and points the API
server at it through `kube-apiserver-arg: admission-control-config-file=...` in `k3s_config` -
the shape of k3s's own hardening guide. A change to the file restarts k3s one node at a time,
like a drop-in change.

- **Infrastructure namespaces are exempt** - `k3s_psa_exempt_namespaces` in the role's
  defaults. Some of it must run privileged: kured and the CSI driver in `kube-system`, the
  upgrade Jobs in `system-upgrade`. A new infrastructure component that needs more than the
  restricted standard goes onto that list in the commit that deploys it; components that run
  restricted anyway (`cert-manager`, `monitoring`, `loki`) are not on it.
- **One app is exempt too, temporarily**: `wedding-manuele-robine`, whose images run as root
  (supervisord, nginx, php-fpm and cron on port 80). A TODO next to the entry: make the images
  rootless, then remove it. No other app goes on the list.

## Authentication: Zitadel's ID tokens

The everyday kubeconfig logs in through Zitadel (see "`kubeconfig.yml`" in
[`../../README.md`](../../README.md)); the API server side of that is this role's.

- **The API server checks the token itself**: `ansible/roles/k3s` writes
  `/etc/rancher/k3s/authn.yaml`, an `AuthenticationConfiguration` (structured authentication,
  GA since Kubernetes 1.34), and passes it with `kube-apiserver-arg: authentication-config`.
  Issuer and audience are `k3s_oidc_issuer` and `k3s_oidc_client_id` in
  `inventories/group_vars/prod.yml`, which `kubeconfig.yml` reads too. Username is
  `preferred_username`, groups the webhook's `groups` claim, both prefixed `zitadel:` - so no
  token can name a built-in `system:` user or group. Zitadel's ID token audience is the client
  ID plus the project ID; one matching entry is enough.
- **k3s drops its `--anonymous-auth=false` once that file exists** (it warns "Not setting
  kube-apiserver 'anonymous-auth' flag"), which reopened `/version` and `/readyz` to anonymous
  requests. The file therefore says `anonymous: enabled: false` itself; anonymous requests get
  `401` again (checked 2026-09-30). A change to the file restarts k3s like `psa.yaml`: the API
  server reloads the `jwt` part on its own, but not every part.
- **Zitadel inside the cluster is no problem at start**: the API server starts without the
  issuer and retries every 10 seconds; certificates and ServiceAccount tokens work meanwhile.
  A typo in the issuer is therefore not caught at start - only a login shows it.

## Pod traffic between nodes is encrypted

k3s's pod network, Flannel, runs the `wireguard-native` backend (`flannel-backend` in
`k3s_config`) instead of the default `vxlan`: every pair of nodes is joined by a WireGuard
tunnel, interface `flannel-wg`, UDP 51820 over the private interface (`--flannel-iface`).
Each node generates its own key pair and Flannel publishes the public keys as node
annotations; nothing to manage. So all pod-to-pod traffic that leaves a node - External
Secrets reading from OpenBao, later databases - is encrypted on the Hetzner private network,
without TLS per service. The control plane (API server, kubelet, etcd) already used TLS.

- **The Hetzner firewall needs nothing**: it applies to the public side, not the private
  network the tunnels use.
- **Flannel options must be identical on all servers**, and the backend must never be
  changed casually: while nodes disagree, cross-node pod traffic breaks. The switch from
  `vxlan` (2026-09-26) was a rolling `prod.yml` run and then a rolling reboot of each node,
  which clears the old `flannel.1` interface and its routes. Going back is the same with
  `vxlan`.
- Check it with `ip -d link show flannel-wg` (type `wireguard`) on a node and the node
  annotation `flannel.alpha.coreos.com/backend-type` (`wireguard`). `wg` and `tcpdump` are
  not installed on the nodes.

## etcd snapshots to Object Storage

k3s takes its scheduled snapshots (00:00 and 12:00 UTC) onto each server's disk and uploads
each to `d3strukt0r-prod-etcd`. The bucket, the pruning and the restore are in
[`kubernetes/clusters/prod/etcd-snapshots/README.md`](../../../kubernetes/clusters/prod/etcd-snapshots/README.md);
what is k3s configuration is here.

- **Retention is 5 (the default) - per node on disk, but in total in the bucket.** All three
  servers prune the same `etcd-snapshot-*` names in the one bucket, so it holds the newest 5
  of all of them: about a day, the last two rounds (seen 2026-09-29, the docs do not say so).
  Every pruned snapshot stays readable for 7 more days as a noncurrent version (object lock
  and the bucket's lifecycle rule), so a week can be restored, the older part by version ID -
  see
  [`kubernetes/clusters/prod/etcd-snapshots/README.md`](../../../kubernetes/clusters/prod/etcd-snapshots/README.md). Raising it
  would be `etcd-snapshot-retention` in `k3s_config`, which sets the S3 retention too: the
  Secret has no retention key, and the `etcd-s3-retention` flag, like every `etcd-s3-*` flag,
  would make k3s ignore the Secret.
- **All S3 settings come from Secret `kube-system/k3s-etcd-snapshot-s3-config`.**
  `k3s_config` sets only `etcd-s3: true` and `etcd-s3-config-secret`; per k3s's docs, **any
  other `etcd-s3-*` flag makes it ignore the Secret.** The Secret is built by the
  ExternalSecret in `kubernetes/clusters/prod/etcd-snapshots/` (Application
  `etcd-snapshots`, in the cluster directory rather than `components/` because the bucket
  is prod's): endpoint, region `nbg1` and bucket in git, the cluster's own S3 key from
  OpenBao `secret/etcd-snapshot-s3` (`access-key`, `secret-key`). That key is kept in
  1Password ([`Hetzner | S3 | prod etcd snapshots`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=w2po7jqmxren7y7eruj73p5w4a&h=my.1password.com)) and copied into OpenBao with `bao kv put`
  (command in the etcd-snapshots README); a restore uses the admin key instead. Console label
  `prod etcd snapshots`.

## Running it

```shell
cd ansible
ansible-playbook prod.yml --check --diff
ansible-playbook prod.yml
```
