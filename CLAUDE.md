# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Infrastructure for a small Kubernetes cluster, `prod`, on Hetzner Cloud
(`Team-MaRo/infrastructure`), running the k3s distribution. Three nodes in nbg1, private
network, one public firewall. There is no application code here - everything is
declarative infrastructure.

- `tofu/hcloud/` - the Hetzner Cloud layer
- `tofu/objectstorage/` - the Object Storage buckets (etcd snapshots, tfstate) and their policies
- `tofu/infomaniak/` - registrar-side delegation and DNSSEC checks
- `tofu/cloudflare/` - both Cloudflare accounts, zones, records and TLS settings
- `cloud-init/node.yaml` - node bootstrap, consumed over HTTPS (see below)
- `ansible/` - configures the running nodes, installs k3s, bootstraps Argo CD
- `kubernetes/` - what runs on the cluster, deployed by Argo CD from `master`

Each of the four tofu directories is a **separate root module with its own state key**
in the same bucket. They are deliberately independent: none can break another's plan,
and each needs only its own credentials.

The layers run in order and do not reach back: OpenTofu creates a node, cloud-init
prepares it once as it boots, Ansible configures it from then on, and Argo CD deploys onto
the cluster from `kubernetes/`. Each directory has its own README; the root one is only an
index.

## Commands

No credential is ever exported by hand, and no value belongs in a `.tf` file. Each
module reads its own token from a gitignored `terraform.tfvars` next to its `.tf` files:
`hcloud_token` for hcloud, `infomaniak_token` for infomaniak, the two
`cloudflare_token_*` for cloudflare, and only `project_id` for objectstorage, which reads
its key from the backend profile (see "Object Storage" below). The hcloud file also carries `admin_ips`, which is not a secret but is a
home address, and this repo is public. Every module ships a `terraform.tfvars.example`.

The **backend** is the exception. Backend blocks cannot interpolate, so no variable can
supply the Object Storage keys. All four therefore carry
`profile = "d3strukt0r-hetzner"` and read them from `~/.aws/credentials`. The profile
name is account-scoped on purpose - profiles are global to `~/.aws`, so a bare
`hetzner` would collide with any second Hetzner account. If a plan cannot reach the
state bucket, check that file first: there is no environment variable to fall back on
by design.

`tofu/hcloud` declares `variable "hcloud_token"` and passes it to the provider explicitly
rather than relying on the provider's own `HCLOUD_TOKEN` lookup, so all modules
get their credentials the same way.

```sh
cd tofu/hcloud         # or tofu/objectstorage, tofu/infomaniak, tofu/cloudflare
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

### Rules for OpenTofu in this repo

- **Never run `tofu destroy`.**
- **Never `tofu apply` a plan that is not clean.** A clean plan is
  `0 to add, 0 to change, 0 to destroy`. If a plan shows `forces replacement`,
  `must be replaced` or `will be destroyed`, that is a bug in the config - fix the
  config and re-plan.
- Adoption of existing resources goes through `import` blocks in the module's `imports.tf`,
  not `tofu import` CLI calls, so it stays reviewable in git.

## Architecture

### Everything was created by hand and adopted afterwards

The Hetzner resources predate this repo; they were clicked together in the console.
`tofu/hcloud/imports.tf` adopts each one by its live ID. The config therefore describes
reality rather than an ideal, which is why some values look arbitrary:

- The three nodes are **not** the same type: `prod-01` and `prod-02` are `cx33`,
  `prod-03` is `cx23`.
- Private IPs follow the node number: `prod-NN` = `10.0.0.1NN`. Not `10.0.0.NN`, because
  Hetzner reserves the first address of a network for its gateway, so `.1` can never be
  assigned. Changing a node's IP replaces its network attachment - never while k3s runs
  on it, since etcd peers find each other by these addresses.
- The network is `10.0.0.0/16` but its single auto-created subnet is `10.0.0.0/24`.

`tofu/hcloud/locals.tf` holds the `servers` map, which is the single source of truth: it
drives `hcloud_server`, `hcloud_server_network` **and** the `for_each` import blocks
(the live server IDs live in that map). Adding a node means adding one map entry.

**The server name is the host's identity** - the OS hostname, the Kubernetes node name
and the etcd member name. On the nodes, cloud-init runs `update_hostname` and
`update_etc_hosts` on every boot (`preserve_hostname: false`, `manage_etc_hosts: true`)
and by default takes the name from Hetzner's metadata. **That metadata keeps the name a
server was created with; renaming the server does not change it.** The rename from
`k3s-0N` to `prod-0N` showed this: after the apply the metadata still said `k3s-0N`. So
`ansible/roles/hostname` pins the name for cloud-init with a drop-in,
`/etc/cloud/cloud.cfg.d/90-hostname.cfg` (`hostname: <inventory name>`), and sets it
immediately. cloud-init still does the work on every boot, just from that name. Renaming a
server therefore means a tofu apply plus a `prod.yml` run - and **never while k3s runs on
it**; uninstall k3s first, as that rename did.

Things are named after what they are - the `prod` cluster and its nodes - not after the
tool: `k3s` appears only where something genuinely is k3s (the Ansible role that installs
it, its paths and flags).

### cloud-init is fetched at boot, not embedded

`user_data` on every server is a two-line `#include` pointing at the raw GitHub URL of
`cloud-init/node.yaml` on `master`. Consequences:

- Editing `cloud-init/node.yaml` and pushing changes what **newly created or rebuilt**
  nodes get, immediately, with no OpenTofu run involved.
- It does nothing to running nodes.
- **`cloud-init/k3s.yaml` must stay** until `prod-01..03` are gone. It is a bare two-line
  `#include` forwarding to `node.yaml`, because those servers were created when the file
  had that name, and Hetzner never allows user-data to change. Without it a rebuild of one
  of them fetches a 404 and never creates the SSH user. It holds no comments: every line
  of an `#include` file is read as a URL. The chaining is unverified until a rebuild.

### Attributes OpenTofu cannot see

The Hetzner API returns neither `user_data` nor `ssh_keys` for a server, so both are
null in state and both force replacement. They are under `ignore_changes` on
`hcloud_server`. Editing either in `servers.tf` has **no effect on existing nodes** -
it only applies to nodes created from then on.

### Firewall attaches by label, not by server

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
backend with `use_lockfile = true` and `profile = "d3strukt0r-hetzner"`. The Hetzner
module's key is `hcloud/terraform.tfstate`; it was `k3s/terraform.tfstate` before the
rename and was copied with `tofu init -migrate-state`. `tofu/hcloud/moved.tf` maps the old
resource addresses to the new ones. Changing
anything in a backend block means `tofu init -reconfigure` on the next run.

Hetzner is S3-compatible but not AWS, so all the `skip_*` flags in
`tofu/hcloud/versions.tf` are required; `skip_s3_checksum = true` specifically is what
makes lock-object writes succeed.

Versioning and Object Lock are both enabled on the bucket, but
`get-object-lock-configuration` returns no `Rule`, so there is **no default retention**
- and the S3 backend never sends per-object retention headers. Every apply therefore
adds a version that accumulates but stays deletable. Old versions can be pruned with
`aws s3api delete-object --version-id`; nothing is locked.

```sh
aws --profile d3strukt0r-hetzner s3api list-object-versions --bucket d3strukt0r-tfstate --prefix hcloud/
```

That relies on `endpoint_url` (and `cli_pager =`) in the profile's `~/.aws/config`
section - see `tofu/README.md`. Without it, add
`--endpoint-url https://nbg1.your-objectstorage.com`.

Freshly generated Hetzner S3 credentials propagate across their gateways over several
minutes. During that window `tofu init` fails with
`operation error S3: HeadObject ... StatusCode: 403` while the key already works for
listing buckets. That is not a permissions problem - wait and retry before touching
ACLs, bucket policies or `use_path_style`.

### Object Storage: every key reaches every bucket

Hetzner S3 keys are valid for every bucket in the project, with every permission. The only
way to scope one is a bucket policy, so `tofu/objectstorage` writes both of its policies as
`Deny` + `NotPrincipal` naming the admin key, `arn:aws:iam:::user/p<project_id>:<access_key>`
(`local.admin_principal`). That covers keys that do not exist yet, such as the cluster's.

- **`d3strukt0r-tfstate`: every action denied to every other key** - the Hetzner web
  console included, which reads buckets under its own identity and so shows "Ressource ist
  blockiert" and no files. Browse it with the admin key (aws CLI, or an S3 client such as
  Cyberduck on `nbg1.your-objectstorage.com`). The bucket predates the
  repo and is adopted through `tofu/objectstorage/imports.tf`, with `prevent_destroy`. It
  holds this module's own state too, so it must never leave the config. On import the
  provider reads the custom policy as `acl = "private"`, the default, so there is no ACL
  diff - an ACL change would clear the policy. The allowed
  key must be the one in the `[d3strukt0r-hetzner]` profile, since the backends use it and
  this module must stay able to change the policy. The provider cannot read AWS profiles
  (only its own attributes or `MINIO_*` variables), so `locals.tf` parses that section of
  `~/.aws/credentials` with `file()` + `regex()` - no second copy that could drift, and no
  secret in state, since locals and provider configuration are not stored. The regex
  expects `aws_access_key_id` before `aws_secret_access_key`.
- **A wrong principal is a lockout.** It denies `s3:*` to the admin key as well, including
  `PutBucketPolicy`, so OpenTofu cannot revert it; every backend stops and only Hetzner
  support can remove the policy. Any change to how the principal is built is first proven
  on the etcd bucket with the same statement - the staged procedure is in `tofu/README.md`.
- **`d3strukt0r-prod-etcd`: object lock, GOVERNANCE, 7 days, plus a lifecycle rule** that
  removes versions 7 days after they become noncurrent, and orphaned delete markers after
  that. GOVERNANCE rather than COMPLIANCE so the admin key can still delete early; since a
  Hetzner key holds the governance bypass like any permission, the policy denies
  `s3:BypassGovernanceRetention` and all lock, lifecycle and policy changes to other keys.
  Object lock was only possible at creation and made versioning permanent.
- **Provider `aminueza/minio`, `s3_compat_mode` off.** The `aws` provider cannot refresh a
  bucket here (it reads Accelerate, Website, Logging, Replication and Tagging, all
  unimplemented). Compat mode would swallow "not implemented" errors, object lock and
  lifecycle included, so an unsupported setting would look applied while doing nothing.
  Off, anything Hetzner rejects fails loudly. Tagging and the provider's MinIO edition probe
  already degrade gracefully without it. `minio_ssl` defaults to `false` and must stay set.
- **A plan can briefly show `minio_s3_bucket_object_lock_configuration` as "will be
  created".** The provider drops it from state when a read says the bucket or its lock
  configuration does not exist, and Hetzner's gateways answered that once for a moment
  during the first stage-B apply, while the setting was intact. Check with
  `aws s3api get-object-lock-configuration` before anything else; re-applying is harmless,
  since it writes the identical default retention.
- **The bucket's `acl` (default `private`) clears the bucket policy** when the bucket is
  created or its `acl` changes. Never set `acl` on these buckets - the policy resource owns
  the policy.

### Domains: verified against DNS, corrected through the API

Ten domains are registered at Infomaniak and delegated to Cloudflare; Infomaniak is
registrar only. `tofu/infomaniak/domains.tf` declares the expected delegation and DNSSEC state,
and `tofu/infomaniak/domains_checks.tf` asserts against reality on every plan.

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
- `data "external"` runs `tofu/infomaniak/scripts/ds-lookup.sh`, which shells out to `dig DS`.
  The dns provider has no DS data source, and Infomaniak's readable
  `GET /2/domains/{domain}/dnssec/check` needs a bearer token - `data "http"` persists
  its request headers into state, so the token would land in the state file.

Two consequences worth knowing:

- **Check block failures are warnings, not errors.** They print on every `tofu plan`
  but do not fail the command or block an apply. For a hard failure, move the condition
  into a `postcondition` on the data source.
- **The data sources themselves do error**, unlike the checks. A lapsed domain, a typo
  in `local.domains` or a DNS outage makes `tofu plan` fail *in this module*. That is
  why this is a separate root module: it used to live alongside the cluster, where the
  same failure blocked Hetzner work for an unrelated reason.
- **`dig` must be on PATH** wherever OpenTofu runs, and a plan now does ~20 DNS
  lookups.

Correcting drift is handled by `tofu/infomaniak/domains_nameservers.tf`. Because Infomaniak's
provider cannot do it, a `terracurl_request` issues the `PUT` directly:

- `for_each = toset(local.delegation_drift)` - the resource **only exists for domains
  that have actually drifted**. In a steady state there are zero instances, so a plan
  is empty and no request is ever sent needlessly. Once a correction lands the instance
  disappears again.
- `headers_wo` and `request_body_wo` are **write-only**, so neither the token nor the
  body is stored in state. A secret in state would land in a versioned bucket, where
  purging it means hunting down every version that contains it - possible, since no
  retention rule is set, but far easier never to write it. OpenTofu 1.12 supports
  write-only attributes; the body is write-only purely to avoid a standing
  "use the WriteOnly version" warning, which would drown out the check warnings that
  drift detection relies on.
- `skip_read = true` because there is no GET to read back, and `skip_destroy = true`
  because destroying a delegation setting is meaningless.
- A `precondition` fails with a readable message if a correction is needed but
  `infomaniak_token` is unset, rather than sending `Bearer `. That token is
  created at <https://manager.infomaniak.com/v3/ng/profile/user/token/list> with
  scopes `domain:write` and `domain:read`, and is shown only once.

Registry NS changes take time to propagate, so a plan run shortly after a correction
may still see the old delegation and re-issue the PUT. It is idempotent.

`devops-rob/terracurl` is third-party and the OpenTofu registry holds no GPG key for
it, so `tofu init` reports "Signature validation was skipped". Its hash is pinned in
`.terraform.lock.hcl`, which catches later tampering, but the first download was not
signature-verified - and this is the provider the API token is handed to. That is the
trade-off taken against doing the PUT from a plain script.

### The Cloudflare module

`tofu/cloudflare/` has its own state (`key = cloudflare/terraform.tfstate`, same
bucket), like the other two. Running `tofu` there is a separate `init`/`plan`, with its
own two tokens.

There are **two Cloudflare accounts under two different logins**, which is why
`providers.tf` declares `cloudflare.personal` and `cloudflare.arepazo` as aliases and
**no default provider** - every resource must name its account, so a mistake fails to
resolve instead of writing to the wrong place. Tokens must be scoped to
`All zones from an account`; with `All zones` either token reaches both accounts and the
split is decorative.

Things that will bite:

- The provider is **v5**, which was regenerated from Cloudflare's OpenAPI spec. The
  resource is `cloudflare_dns_record`; the v4 name `cloudflare_record` is gone, so most
  examples found online do not apply.
- Import IDs: `cloudflare_zone` is `<zone_id>`, `cloudflare_dns_record` is
  `<zone_id>/<dns_record_id>`, `cloudflare_zone_setting` is `<zone_id>/<setting_id>`.
- `cloudflare_zone` has almost no writable surface - `account.id`, `name`, `paused`,
  `type`, `vanity_name_servers`, and everything else is read-only. It is kept anyway
  because it records which account owns which zone.
- Every A/AAAA record is proxied, and `proxied = true` forces `ttl = 1`. A generated
  record with any other TTL will fight the API.
- cf-terraforming can *generate* HCL for zones, records and settings, but only lists
  `cloudflare_dns_record` as import-capable for v5. The zone and zone-setting import
  blocks come from `local.zones`/`local.setting_pairs` via `for_each`, not from the
  tool. Also, `--resource-type cloudflare_zone` ignores `--zone` and dumps every zone
  the token can see, so its output needs deduplicating.

### Records Cloudflare owns but OpenTofu now tracks

Seven AAAA records point at `100::` with `meta.origin_worker_id` set and
`meta.read_only = true` - `d3strukt0r.dev` apex and www, `weleda-webcenter-text-export`,
`robines.space` apex and www, `wundexpertinplus.com` apex and www. Cloudflare creates
and maintains these for Workers routes. They imported cleanly, but if a Worker route
changes Cloudflare rewrites them and a plan will report drift that nothing in this repo
caused. Do not "fix" that drift blindly - check the Workers config first.

`_acme-challenge.portainer.d3strukt0r.dev` exists twice (`f6b035be` from 2026-08-31 and
`1a87e754` from 2024-05-17, both TTL 120), leftovers from ACME validation. Both are
imported so the plan stays clean; removing the stale one is a deliberate change.

### Zones exist that no delegation check covers

The `ns_delegation` check in the cluster module covers exactly the ten domains
registered at Infomaniak. Two further zones are delegated to Cloudflare but registered
elsewhere - `wundexpertinplus.com` at GoDaddy and `arepazo.ch` at an unidentified `.ch`
registrar. Nothing verifies their delegation. Extending the check needs a second list,
since `local.domains` in `tofu/infomaniak/domains.tf` is specifically the Infomaniak set.

### Ansible

Run **from the `ansible/` directory** - `ansible.cfg` is only read from the current
directory, and the inventory's token lookup uses a path relative to it.

The selection model is the thing to understand: **inventory decides membership, the
playbook's `hosts:` decides what that means, and there are no feature flags.** A role
either appears in a playbook's `roles:` list or it does not. This follows the layout in
Ansible's own sample setup and in the Red Hat CoP good practices, both of which map a
group to roles in one playbook per type; neither uses conditional `*_enabled` gating.

- **One inventory directory, two sources.** `inventories/hcloud.yml` is dynamic (Hetzner
  API, `label_selector: cluster=prod`, `group: prod`) and `inventories/home.yml` is static.
  They load together, so `prod` and `home` are groups in one inventory.
- **A default inventory is set**, which is safe only because `hosts:` constrains each
  playbook - `prod.yml` cannot touch a home server. **Do not write a playbook with
  `hosts: all`**, or that guarantee is gone.
- **`site.yml` imports the per-type playbooks.** Run it for everything, or one playbook
  for one type. `kubeconfig.yml` is deliberately not among them.
- **The prod inventory connects over public IPs.** `network: prod` filters to nodes on the
  private network and exposes `hcloud_private_ipv4` as a hostvar for the cluster's configuration,
  but `ansible_host` stays the public address - a laptop cannot route to `10.0.0.0/24`.
- **Its token comes from `tofu/hcloud/terraform.tfvars`** via a `lookup`, rather than a
  second copy or an environment variable. `HCLOUD_TOKEN` overrides it if that breaks.
- **No `requirements.yml` install is needed.** The `ansible` package bundles
  `ansible.posix`, `community.general` and `hetzner.hcloud`; that file records floors.
- **Host key checking is on**, because these nodes are on the public internet.

### The k3s role, and why it is two plays

`prod.yml` runs `roles/k3s` in two plays rather than one. That is forced, not stylistic:
`--cluster-init` has to finish on one node before the others can join, joining wants
`serial: 1` so etcd keeps a quorum while members are added, and `hosts:` and `serial:` are
play-level settings a role cannot change.

- **`prod-01` is written out literally** in the host patterns. A play's pattern is resolved
  before any host is selected, so inventory variables are not available to it - templating
  `hosts:` from `group_vars` fails with `'k3s_init_node' is undefined`.
  `k3s_init_node` in `group_vars/prod.yml` names the same node and has to be kept in step;
  the role deliberately has no default for it, since it names a host.
- **The second play is `prod:!prod-01`**, not `all:!prod-01`, which would sweep in `home`.
- **The playbook touches nothing but the nodes.** Fetching a kubeconfig is
  `ansible/kubeconfig.yml`, deliberately separate - see below.
- **Install tasks are guarded by `creates: /usr/local/bin/k3s`**, so a re-run changes
  nothing. The trade is that editing `defaults/main.yml` afterwards has no effect on a
  running node - the flags have to be right the first time.
- **A dry run cannot cover all of it.** The join needs a token only a real first play
  produces, so it is skipped under `--check`. What a dry run does verify is connectivity,
  private-interface detection and flag assembly on all three nodes.

### The admin kubeconfig is break-glass, and is fetched separately

`ansible/kubeconfig.yml` is its own playbook and is **not** imported by `site.yml`. Writing
into `~/.kube` through `delegate_to: localhost` provisions a credential onto a workstation;
it is not configuring a server, and it does not belong inside a role named after what it
installs on the nodes.

What it fetches is not an everyday credential. `/etc/rancher/k3s/k3s.yaml` embeds a client
certificate whose subject is `CN=system:admin, O=system:masters`, and per the Kubernetes
RBAC guidance any member of that group **"bypasses all RBAC rights checks and will always
have unrestricted superuser access, which cannot be revoked by removing RoleBindings or
ClusterRoleBindings"**. No RBAC can limit that file. Scoping everyday access therefore
means issuing a *second* identity, not writing policy against this one - which is what the
next stage does. Until then every `kubectl` command runs as an unrestricted superuser.

It lands as `~/.kube/d3strukt0r-prod-admin.yaml`, context `d3strukt0r-prod-admin`, leaving
the plain name free. It is written beside `~/.kube/config` rather than into it, because the
playbook writes the file whole. The name rewrites are anchored to their keys
(`name: default`, not `default`) because base64 contains no colon or space, so an anchored
pattern cannot collide with the certificate blobs.

It also expires: k3s issues 365-day certificates and renews them on startup within 120 days
of expiry, on the node. This copy does not follow, so `kubeconfig.yml` has to be re-run.

### Argo CD: bootstrapped once, then managing itself

`ansible/argocd.yml` (role `argocd`) installs Argo CD from `kubernetes/components/argocd/`
and applies `kubernetes/clusters/<cluster_name>/root.yaml` (`cluster_name: prod` in
`inventories/group_vars/prod.yml`), **once** - it checks for the `argocd-server` Deployment
and skips everything if present. It runs `kubectl` **from the admin's machine** with the
admin kubeconfig (`admin_kubeconfig`, same group_vars), so it runs after `kubeconfig.yml`
and only from an address in `admin_ips`; nothing is copied to the nodes. A guard error
other than `NotFound` (typically 6443 unreachable) fails loudly instead of being read as
"not installed". Like `kubeconfig.yml` it is not imported by `site.yml`. After that Argo CD manages
itself and every other component from `kubernetes/` on `master`. Never re-apply those
manifests from Ansible: a working copy that differs from `master` would fight Argo CD's
self-heal.

- **Pushing to `master` is deploying**, pruning included. Branch protection on the repo is
  a security control. The files must also be pushed *before* the first bootstrap, because
  `root` syncs from GitHub, not from the working copy.
- **`kubernetes/clusters/<name>/` decides what runs where; `kubernetes/components/<app>/`
  holds how**, shared by clusters. Each cluster directory is an app-of-apps: `root` syncs
  the directory it lives in, so it manages itself. One Argo CD per cluster, each syncing
  only its own directory, so no cluster can change another. A cluster that needs something
  different gets a Kustomize overlay in its own directory, not a copy of the component.
- **`kubernetes/components/argocd/` is Kustomize over the pinned upstream `install.yaml`**, the
  approach Argo CD's own docs use for self-management. The base is a raw single-file URL:
  the `github.com/...//manifests` form makes Kustomize shell out to `git`, which the nodes
  do not have. Upgrading is bumping that URL, **one minor at a time** - Argo CD does not
  support skipping minors.
- **Bootstrap and self-management use the same directory**, so the first self-sync is a
  no-op. Both apply server-side; `ServerSideApply=true` is required for a self-managed
  Argo CD because its CRDs are too large for client-side apply's annotation.
- **Dex is removed** by `patches/dex.yaml`; Zitadel will be the OIDC provider and Argo CD
  will talk to it directly. `argocd-server` still mounts an optional `argocd-dex-server-tls`
  secret volume from upstream - harmless, left alone rather than diverging from upstream.
- **Non-HA on purpose.** Even the HA manifest keeps the application controller - the part
  that syncs - at one replica, so HA mostly buys a UI that survives a node failure. That
  controller is a StatefulSet, which Kubernetes will not replace on an unreachable node:
  syncing stays stopped until the node returns or is tainted
  `node.kubernetes.io/out-of-service`.
- **The UI is not exposed** - port-forward from the admin context until cert-manager can
  provide TLS. The local `admin` account is used until Zitadel exists, and stays as the
  break-glass login afterwards.

### What a second cluster would need

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

### The swap role is written but unreferenced

No playbook lists it, so it does nothing. That is deliberate: nothing runs on these nodes
yet, so there is nothing to measure and any tuning value would be guesswork. Enabling it
is adding `- swap` to the `roles:` of the first two plays in `ansible/prod.yml` - that
line *is* the decision, which is why there is no `swap_enabled` flag sitting at false.

### The swap role, and what it does not claim

Values follow the *Recommended starting point* in the Kubernetes swap deep-dive, not the
values that post used while experimenting. Two are easy to misread:

- **`vm.swappiness = 60` is the kernel default.** It is set explicitly to record intent,
  which means the role currently changes nothing about swappiness. Do not call it tuned.
  swappiness is a relative IO *cost ratio* (0-200, 100 = swap and file IO equally
  expensive), not an eagerness dial. A low value claims swap IO is far costlier than file
  IO - true of spinning disks, false of the local NVMe here - and does not avoid disk IO,
  it shifts thrashing onto the page cache. If a low value is ever wanted, 1 is the floor,
  not 0: since a 2012 vmscan change, 0 will not scan anonymous pages until severe
  contention.
- **`vm.min_free_kbytes` is 3% of *reported* RAM**, the upper end of the blog's
  "2-3% of total node memory" - not a round number, because a 4 GB node reports about
  3900 MB. Verify against `/proc/meminfo`, never a fixed figure. The blog contradicts
  itself here: its own test used 512 MiB on a 5 GiB node, roughly 10%.

`vm.watermark_scale_factor = 2000` is the one value that genuinely changes behaviour, by
widening the reclaim window so kswapd can page out before the node hits a critical state.

**Swap is deliberately not encrypted.** It would only protect paged-out memory from
someone reading the disk offline, and the same unencrypted root filesystem holds the etcd
datastore with every Secret, the cluster CA keys, the join token and a cluster-admin
kubeconfig. The answer to that threat is full-disk encryption, which on Hetzner costs
unattended reboots - a real trade for a 3-node etcd cluster.

### Two flags the install has to set up front

Both are in `roles/k3s/defaults/main.yml`, and both are install-time only:

- `--secrets-encryption`, which is free at install time and **cannot be enabled on an
  existing server without restarting it**. Verify with `k3s secrets-encrypt status` on a
  node; it should report `Enabled`.
- `--kubelet-arg=fail-swap-on=false`, because kubelet refuses to start on a swap-enabled
  node. Swap is off today, so this currently changes nothing - it exists so the swap role
  can be enabled later without reinstalling k3s. Leave `swapBehavior` at its default
  `NoSwap`: swap then protects the node and system daemons while pods cannot use it.
  `LimitedSwap` only ever grants swap to Burstable pods anyway.

The install pins k3s `v1.37.0+k3s1` (Kubernetes 1.37, containerd 2.3.4) on Debian 13 with
kernel 6.12. That combination clears every requirement for **per-pod user namespaces**
(`hostUsers: false`), which are GA and locked since Kubernetes 1.36 and need Linux 6.3+,
containerd 2.0+ and an idmap-capable filesystem. This is the answer to wanting Podman's
rootless property: k3s's own `--rootless` mode is experimental and, per its docs,
multi-node rootless clusters are unsupported, so it is a single-node mode and not an
option here.

Port 6443 is open to `var.admin_ips` only, applied in commit `40ffa27`. `admin_ips` has
no default on purpose: an empty list makes an invalid rule, and a default would risk
silently opening the API. So a plan stops and asks for a value until `admin_ips` is set in
`tofu/hcloud/terraform.tfvars`. It takes full CIDRs (`/32` for one IPv4, `/128` for one IPv6)
and goes stale whenever the ISP reassigns the address - `kubectl` then hangs until it is
updated and re-applied.

### Not managed here

The per-server primary IPs (`public_net` is deliberately omitted from `hcloud_server`, and
the provider suppresses that diff when unset), and the Object Storage keys - no provider
has a resource for them, so they and their console labels live only in the Hetzner
console. Registrar contacts, transfers and renewals
are out of scope too, as is domain expiry monitoring (readable via `expires_at`, but
it needs a bearer token).
