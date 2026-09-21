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

### Domains: verified against DNS, corrected through the API

Ten domains are registered at Infomaniak and delegated to Cloudflare; Infomaniak is
registrar only. `tofu/domains.tf` declares the expected delegation and DNSSEC state,
and `tofu/domains_checks.tf` asserts against reality on every plan.

There is deliberately no registrar resource. Infomaniak's API has
`PUT /2/domains/{domain}/nameservers` but **no GET** - `GET /2/domains/domains`
returns `contacts, created_at, expires_at, id, is_premium, name, options,
resale_status, status, tld` and nothing about nameservers. Without a read there can be
no import and no drift detection, so a resource would claim success forever. The
`Infomaniak/infomaniak` provider does not help either: its domain surface is only
zones and records *hosted at Infomaniak*, which these domains do not use.

So the TLD registry is queried instead, which is authoritative for delegation:

- `data "dns_ns_record_set"` reads the NS records. Its `nameservers` attribute is
  already sorted by the provider.
- `data "external"` runs `tofu/scripts/ds-lookup.sh`, which shells out to `dig DS`.
  The dns provider has no DS data source, and Infomaniak's readable
  `GET /2/domains/{domain}/dnssec/check` needs a bearer token - `data "http"` persists
  its request headers into state, so the token would land in the state file.

Two consequences worth knowing:

- **Check block failures are warnings, not errors.** They print on every `tofu plan`
  but do not fail the command or block an apply. That is intentional so a transient
  DNS hiccup cannot block cluster work. For a hard failure, move the condition into a
  `postcondition` on the data source.
- **`dig` must be on PATH** wherever OpenTofu runs, and a plan now does ~20 DNS
  lookups.

Correcting drift is handled by `tofu/domains_nameservers.tf`. Because Infomaniak's
provider cannot do it, a `terracurl_request` issues the `PUT` directly:

- `for_each = toset(local.delegation_drift)` - the resource **only exists for domains
  that have actually drifted**. In a steady state there are zero instances, so a plan
  is empty and no request is ever sent needlessly. Once a correction lands the instance
  disappears again.
- `headers_wo` and `request_body_wo` are **write-only**, so neither the token nor the
  body is stored in state. That matters because the state bucket has Object Lock: a
  secret written there could not be purged, only rotated. OpenTofu 1.12 supports
  write-only attributes; the body is write-only purely to avoid a standing
  "use the WriteOnly version" warning, which would drown out the check warnings that
  drift detection relies on.
- `skip_read = true` because there is no GET to read back, and `skip_destroy = true`
  because destroying a delegation setting is meaningless.
- A `precondition` fails with a readable message if a correction is needed but
  `TF_VAR_infomaniak_token` is unset, rather than sending `Bearer `. That token is
  created at <https://manager.infomaniak.com/v3/ng/profile/user/token/list> with
  scopes `domain:write` and `domain:read`, and is shown only once.

Registry NS changes take time to propagate, so a plan run shortly after a correction
may still see the old delegation and re-issue the PUT. It is idempotent.

`devops-rob/terracurl` is third-party and the OpenTofu registry holds no GPG key for
it, so `tofu init` reports "Signature validation was skipped". Its hash is pinned in
`.terraform.lock.hcl`, which catches later tampering, but the first download was not
signature-verified - and this is the provider the API token is handed to. That is the
trade-off taken against doing the PUT from a plain script.

### Not managed here

The Object Storage bucket itself (the hcloud provider has no resource for it), the
per-server primary IPs (`public_net` is deliberately omitted from `hcloud_server`, and
the provider suppresses that diff when unset), and the Cloudflare zones and records -
those are a separate, later piece of work. Registrar contacts, transfers and renewals
are out of scope too, as is domain expiry monitoring (readable via `expires_at`, but
it needs a bearer token).
