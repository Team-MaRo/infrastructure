# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Infrastructure for a small k3s cluster on Hetzner Cloud (`Team-MaRo/infrastructure`).
Three nodes in nbg1, private network, one public firewall. There is no application
code here - everything is declarative infrastructure.

- `tofu/` - OpenTofu config for the Hetzner Cloud layer
- `cloud-init/k3s.yaml` - node bootstrap, consumed over HTTPS (see below)
- `ansible/`, `kubernetes/` - planned, not present yet

## Commands

All OpenTofu work happens in `tofu/`. Credentials come from the environment and must
never be written into a `.tf` file:

```sh
export HCLOUD_TOKEN=...
export AWS_ACCESS_KEY_ID=...      # Hetzner Object Storage access key
export AWS_SECRET_ACCESS_KEY=...
```

```sh
cd tofu
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

### Rules for OpenTofu in this repo

- **Never run `tofu destroy`.**
- **Never `tofu apply` a plan that is not clean.** A clean plan is
  `0 to add, 0 to change, 0 to destroy`. If a plan shows `forces replacement`,
  `must be replaced` or `will be destroyed`, that is a bug in the config - fix the
  config and re-plan.
- Adoption of existing resources goes through `import` blocks in `tofu/imports.tf`,
  not `tofu import` CLI calls, so it stays reviewable in git.

## Architecture

### Everything was created by hand and adopted afterwards

The Hetzner resources predate this repo; they were clicked together in the console.
`tofu/imports.tf` adopts each one by its live ID. The config therefore describes
reality rather than an ideal, which is why some values look arbitrary:

- The three nodes are **not** the same type: `k3s-01` and `k3s-02` are `cx33`,
  `k3s-03` is `cx23`.
- Private IPs are not in name order: `k3s-01` = 10.0.0.3, `k3s-02` = 10.0.0.2,
  `k3s-03` = 10.0.0.4.
- The network is `10.0.0.0/16` but its single auto-created subnet is `10.0.0.0/24`.

`tofu/locals.tf` holds the `servers` map, which is the single source of truth: it
drives `hcloud_server`, `hcloud_server_network` **and** the `for_each` import blocks
(the live server IDs live in that map). Adding a node means adding one map entry.

### cloud-init is fetched at boot, not embedded

`user_data` on every server is a two-line `#include` pointing at the raw GitHub URL of
`cloud-init/k3s.yaml` on `master`. Consequences:

- Editing `cloud-init/k3s.yaml` and pushing changes what **newly created or rebuilt**
  nodes get, immediately, with no OpenTofu run involved.
- It does nothing to running nodes.

### Attributes OpenTofu cannot see

The Hetzner API returns neither `user_data` nor `ssh_keys` for a server, so both are
null in state and both force replacement. They are under `ignore_changes` on
`hcloud_server`. Editing either in `servers.tf` has **no effect on existing nodes** -
it only applies to nodes created from then on.

### Firewall attaches by label, not by server

`hcloud_firewall.k3s_public` uses a single `apply_to { label_selector = "role=k3s" }`.
There are no per-server attachments. Two consequences:

- Removing the `role = k3s` label from a server silently removes its firewall.
- Do not add a `hcloud_firewall_attachment` resource; it fights with `apply_to` over
  the same API field. Pick one mechanism, and the chosen one is the label selector.

### Network attachments

`hcloud_server_network` addresses the network by `network_id` + `ip`, never by
`subnet_id`. `subnet_id` is `RequiresReplace` and is not read back from the API, so
using it would plan a replace of every attachment on import.

### Destroy protection is two-layered

- `delete_protection` and `rebuild_protection` are `true` on every server - the API
  refuses the operation. Both default to `false` in the provider, so both must stay
  set explicitly or the next plan turns them off.
- `prevent_destroy = true` on `hcloud_server`, `hcloud_network` and
  `hcloud_network_subnet` - OpenTofu refuses to even produce such a plan, and
  `tofu destroy` fails. The server types are cost-optimized with limited
  availability; a released node may not be creatable again. Retiring a node means
  removing that line first, deliberately.

`prevent_destroy` does not block in-place updates, so a `server_type` resize still
works (it is a resize with a reboot, not a replace).

### State backend

Hetzner Object Storage bucket `d3strukt0r-tfstate` in **nbg1** (not fsn1), via the S3
backend with `use_lockfile = true`. Hetzner is S3-compatible but not AWS, so all the
`skip_*` flags in `tofu/versions.tf` are required; `skip_s3_checksum = true`
specifically is what makes lock-object writes succeed. The bucket has versioning and
Object Lock enabled, so every state write creates a version that cannot be deleted
until its retention expires.

Freshly generated Hetzner S3 credentials propagate across their gateways over several
minutes. During that window `tofu init` fails with
`operation error S3: HeadObject ... StatusCode: 403` while the key already works for
listing buckets. That is not a permissions problem - wait and retry before touching
ACLs, bucket policies or `use_path_style`.

### Not managed here

The Object Storage bucket itself (the hcloud provider has no resource for it) and the
per-server primary IPs (`public_net` is deliberately omitted from `hcloud_server`, and
the provider suppresses that diff when unset).
