# hcloud

OpenTofu config for the Hetzner Cloud project, named after the provider like the other
modules. Today that is the `prod` cluster: three servers, their network, placement group
and firewall. Resources are named after the cluster, not after the Kubernetes distribution
running on it.

Shared rules, credentials and the state backend are in [`../README.md`](../README.md).

## Everything was created by hand and adopted afterwards

The Hetzner resources predate this repo; they were clicked together in the console.
`tofu/hcloud/imports.tf` adopts each one by its live ID. The config therefore describes
reality rather than an ideal, which is why some values look arbitrary:

- All three nodes are `cx33`. `prod-03` was created as `cx23` and resized in the console
  (2026-10-02, CPU and RAM only, so its disk stayed at 40 GB and a downgrade stays possible).
- Private IPs follow the node number: `prod-NN` = `10.0.0.1NN`. Not `10.0.0.NN`, because
  Hetzner reserves the first address of a network for its gateway, so `.1` can never be
  assigned. Changing a node's IP replaces its network attachment - never while k3s runs
  on it, since etcd peers find each other by these addresses.
- The network is `10.0.0.0/16` but its single auto-created subnet is `10.0.0.0/24`.

`tofu/hcloud/locals.tf` holds the `servers` map, which is the single source of truth: it
drives `hcloud_server`, `hcloud_server_network` **and** the `for_each` import blocks
(the live server IDs live in that map). Adding a node means adding one map entry - and its
public IP to `local.prod_nodes` in `tofu/cloudflare`, the cluster's DNS entry point.

**The server name is the host's identity** - the OS hostname, the Kubernetes node name
and the etcd member name. On the nodes, cloud-init runs `update_hostname` and
`update_etc_hosts` on every boot (`preserve_hostname: false`, `manage_etc_hosts: true`)
and by default takes the name from Hetzner's metadata. **That metadata keeps the name a
server was created with; renaming the server does not change it.** The rename from
`k3s-0N` to `prod-0N` showed this: after the apply the metadata still said `k3s-0N`. So
[`ansible/roles/hostname`](../../ansible/roles/hostname/README.md) pins the name for cloud-init with a drop-in,
`/etc/cloud/cloud.cfg.d/90-hostname.cfg` (`hostname: <inventory name>`), and sets it
immediately. cloud-init still does the work on every boot, just from that name. Renaming a
server therefore means a tofu apply plus a `prod.yml` run - and **never while k3s runs on
it**; uninstall k3s first, as that rename did.

Things are named after what they are - the `prod` cluster and its nodes - not after the
tool: `k3s` appears only where something genuinely is k3s (the Ansible role that installs
it, its paths and flags).

The module used to be called `k3s`, with state key `k3s/terraform.tfstate`. `moved.tf`
maps the old resource addresses to the new ones; the state was copied to the new key with
`tofu init -migrate-state`.

## Attributes OpenTofu cannot see

The Hetzner API returns neither `user_data` nor `ssh_keys` for a server, so both are
null in state and both force replacement. They are under `ignore_changes` on
`hcloud_server`. Editing either in `servers.tf` has **no effect on existing nodes** -
it only applies to nodes created from then on.

## Firewall attaches by label, not by server

`hcloud_firewall.prod_public` attaches by `apply_to { label_selector = "cluster=prod" }`.
There are no per-server attachments. Two consequences:

- Removing the `cluster = prod` label from a server silently removes its firewall.
- Do not add a `hcloud_firewall_attachment` resource; it fights with `apply_to` over
  the same API field. Pick one mechanism, and the chosen one is the label selector.

**Changing the label needs two applies.** Swapping the servers' label and the selector in
one apply has no guaranteed order; if the servers relabel first they are briefly
unfirewalled, with 6443, 10250 and etcd open. Several `apply_to` blocks are a union, so:
first add the new label and a second `apply_to`, then remove the old ones. The switch from
`role=k3s` to `cluster=prod` was done that way.

## The API port is open to admin_ips only

Port 6443 is open to `var.admin_ips` only, applied in commit `40ffa27`. `admin_ips` has
no default on purpose: an empty list makes an invalid rule, and a default would risk
silently opening the API. So a plan stops and asks for a value until `admin_ips` is set in
`tofu/hcloud/terraform.tfvars`. It takes full CIDRs (`/32` for one IPv4, `/128` for one IPv6)
and goes stale whenever the ISP reassigns the address - `kubectl` then hangs until it is
updated and re-applied.

## Network attachments

`hcloud_server_network` addresses the network by `network_id` + `ip`, never by
`subnet_id`. `subnet_id` is `RequiresReplace` and is not read back from the API, so
using it would plan a replace of every attachment on import.

## Destroy protection is two-layered

- `delete_protection` and `rebuild_protection` are `true` on every server - the API
  refuses the operation. Both default to `false` in the provider, so both must stay
  set explicitly or the next plan turns them off.
- `prevent_destroy = true` on `hcloud_server`, `hcloud_network` and
  `hcloud_network_subnet` - OpenTofu refuses to even produce such a plan, and
  `tofu destroy` fails. The server types are cost-optimized with limited
  availability; a released node may not be creatable again, so any plan that would
  destroy or replace one fails instead. Retiring a node means removing that line first,
  deliberately.

`prevent_destroy` does not block in-place updates, so a `server_type` resize still
works (it is a resize with a reboot, not a replace).

## Not managed here

The per-server primary IPs: `public_net` is deliberately omitted from `hcloud_server`, and
the provider suppresses that diff when unset.

## Running it

Needs `hcloud_token` and `admin_ips` in `hcloud/terraform.tfvars` - see
[Credentials](../README.md#credentials).

```sh
cd tofu/hcloud
tofu init
tofu plan
```

Read-only inspection of the live project uses the `hcloud` CLI, which is configured
with its own token (context `d3strukt0r-infrastructure`), independent of
`HCLOUD_TOKEN`:

```sh
hcloud server list -o json
hcloud firewall list -o json
```
