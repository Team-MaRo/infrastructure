# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

Infrastructure for a small Kubernetes cluster, `prod`, on Hetzner Cloud
(`Team-MaRo/infrastructure`), running the k3s distribution. Three nodes in nbg1, private
network, one public firewall. There is no application code here - everything is
declarative infrastructure.

- `tofu/hcloud/` - the Hetzner Cloud layer
- `tofu/objectstorage/` - the Object Storage buckets (etcd snapshots, tfstate) and their policies
- `tofu/openbao/` - OpenBao's own configuration: secrets engine, Kubernetes auth, policies
- `tofu/infomaniak/` - registrar-side delegation and DNSSEC checks
- `tofu/cloudflare/` - both Cloudflare accounts, zones, records and TLS settings
- `cloud-init/node.yaml` - node bootstrap, consumed over HTTPS (see below)
- `ansible/` - configures the running nodes, installs k3s, bootstraps Argo CD
- `kubernetes/` - what runs on the cluster, deployed by Argo CD from `master`

Each of the five tofu directories is a **separate root module with its own state key**
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
`cloudflare_token_*` for cloudflare, `openbao_token` for openbao, and only `project_id` for
objectstorage, which reads its key from the backend profile (see "Object Storage" below).
The hcloud file also carries `admin_ips`, which is not a secret but is a home address, and
this repo is public. Every module ships a `terraform.tfvars.example`.

The **backend** is the exception. Backend blocks cannot interpolate, so no variable can
supply the Object Storage keys. All five therefore carry
`profile = "d3strukt0r-hetzner"` and read them from `~/.aws/credentials`. The profile
name is account-scoped on purpose - profiles are global to `~/.aws`, so a bare
`hetzner` would collide with any second Hetzner account. If a plan cannot reach the
state bucket, check that file first: there is no environment variable to fall back on
by design.

`tofu/hcloud` declares `variable "hcloud_token"` and passes it to the provider explicitly
rather than relying on the provider's own `HCLOUD_TOKEN` lookup, so all modules
get their credentials the same way.

```sh
cd tofu/hcloud         # or tofu/objectstorage, tofu/openbao, tofu/infomaniak, tofu/cloudflare
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
(the live server IDs live in that map). Adding a node means adding one map entry - and its
public IP to `local.prod_nodes` in `tofu/cloudflare`, the cluster's DNS entry point.

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
- and the S3 backend never sends per-object retention headers. Every apply adds a version
and every plan leaves its lock file behind as a version plus a delete marker; a lifecycle
rule (`tofu/objectstorage/tfstate.tf`) removes versions 90 days after they are replaced,
then the orphaned markers. So an old state can be restored for 90 days, not longer. Old
versions can also be pruned early with `aws s3api delete-object --version-id`; nothing is
locked.

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
way to scope one is a bucket policy, so `tofu/objectstorage` writes every policy as
`Deny` + `NotPrincipal` naming the admin key, `arn:aws:iam:::user/p<project_id>:<access_key>`
(`local.admin_principal`), plus at most the one cluster key that bucket is for
(`local.etcd_principal`, `local.loki_principal`, `local.mariadb_backups_principal`,
`local.postgres_backups_principal`). That covers keys that do not exist yet, so
**each key the cluster holds reaches only its own bucket**. The cluster keys' access key IDs
(not their secrets) are `etcd_access_key_id`, `loki_access_key_id`,
`mariadb_backups_access_key_id` and `postgres_backups_access_key_id` in the module's tfvars; a
mistyped one shuts only that key out of its bucket. A `NotPrincipal` naming two keys was proven
on the Loki bucket first (2026-09-28): the admin key kept full access, and the etcd key got
`AccessDenied` there.

**Every bucket has a lifecycle rule** that aborts multipart uploads not completed within 7
days: a broken large upload leaves its parts behind, invisible in a listing but stored and
billed. **A versioned bucket's rule also expires noncurrent versions** and then orphaned
delete markers - versioning never forgets on its own. An unversioned bucket's rule does no
more than the abort: its writer deletes old data itself (Loki's compactor, a backup tool's
retention), and a second deleter could remove objects the tool still expects. The one
exception is `d3strukt0r-prod-mariadb-backups`, whose writer does not prune everything (see below). Hetzner's lifecycle only removes
*orphaned* markers with `expired_object_delete_marker`; it never expires current objects,
although uploads answer with an `Expiration` header that suggests so (tested 2026-09).

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
  `s3:BypassGovernanceRetention` and all lock, lifecycle and policy changes to every key but
  the admin's - the snapshot key included. A first statement shuts out every key but those
  two entirely. Object lock was only possible at creation and made versioning permanent.
- **`d3strukt0r-prod-loki`: unversioned, no lock**, lifecycle rule only for incomplete
  uploads; Loki's compactor deletes old logs. Only the admin key and Loki's key (console label
  `prod loki`, 1Password `Hetzner | S3 | prod loki`, OpenBao `secret/loki-s3`) reach it.
- **`d3strukt0r-prod-mariadb-backups`: unversioned, objects expire after 35 days.** It holds the
  MariaDB operator's physical backups and archived binary logs. The operator deletes backups
  past their 30-day retention itself but never the binary logs, so the lifecycle rule expires
  everything after 35 days - long enough that every kept backup still has its binary logs.
  Only the admin key and the backup key (console label `prod mariadb backups`, 1Password
  `Hetzner | S3 | prod mariadb backups`, OpenBao `secret/mariadb-backups-s3`) reach it.
- **`d3strukt0r-prod-postgres-backups`: unversioned, lifecycle rule only for incomplete
  uploads.** It holds CloudNativePG's base backups and archived WAL (Barman Cloud plugin) of
  the shared PostgreSQL. The plugin deletes base backups past their retention together with the
  WAL they no longer need, so nothing else expires here. Only the admin key and the backup key
  (console label `prod postgres backups`, 1Password `Hetzner | S3 | prod postgres backups`,
  OpenBao `secret/postgres-backups-s3`) reach it.
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

Nine domains are registered at Infomaniak and delegated to Cloudflare; Infomaniak is
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

- **`ssl` is not managed; `ssl_automatic_mode = "auto"` is.** Every zone uses Automatic
  SSL/TLS, so `ssl` is the scanner's current result and changes on its own; a plan writing it
  would switch the zone to manual mode. `d3st.dev`, `d3st.org` and `d3strukt0r.me` are on
  `flexible` by the scanner's choice and **loop with `301`s**: their web records point at
  `prod-old.d3strukt0r.dev` (the old DigitalOcean server, `161.35.16.9`), whose Traefik
  redirects HTTP to HTTPS but has no HTTPS route for these names. Left as is until they are
  served by the cluster with cert-manager, where the scanner will pick `strict` by itself.
- **Destroying a `cloudflare_zone_setting` only removes it from state** - the provider's
  Delete is a no-op and it says so in the plan. Dropping a setting from `local.zone_settings`
  therefore shows as destroys that change nothing in Cloudflare (the switch from `ssl` to
  `ssl_automatic_mode` did exactly that). A `removed` block cannot do it instead: it cannot
  address single `for_each` instances.
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

### The old server and the cluster share the names

The old DigitalOcean server still serves its apps and tools. It is
`prod-old.d3strukt0r.dev` (A and AAAA to its IPs), and every record for something it
serves points there: the wildcard `*.d3strukt0r.dev`, `ssh.`, and the apex CNAMEs of
`d3st.dev`, `d3st.org`, `d3strukt0r.me`, `manuele-vaccari.ch`, `manuele-robine.wedding`
and `old.robines.space`. `arepazo.ch` points at its IPs directly.

- **The wildcard is the default route**: any `*.d3strukt0r.dev` without a record of its own
  reaches the old server, whose Traefik routes it (it keeps its hostnames unchanged).
- **A service moving to the cluster gets its own record**, a CNAME to
  `prod.d3strukt0r.dev`; an explicit record beats the wildcard, so that one name moves and
  everything else stays.
- **`prod.d3strukt0r.dev` is the cluster**: one proxied A and one AAAA record per node
  (`local.prod_nodes` in `records_d3strukt0r_dev.tf`), and Cloudflare spreads requests over
  them. IPv6 reaches the cluster only through Traefik on the nodes' host network (see
  "Ingress"): k3s itself runs single-stack IPv4 (pods `10.42.0.0/16`, services
  `10.43.0.0/16`), dual-stack can only be chosen when a cluster is created, and on Hetzner it
  would also move pod traffic onto the public interface, because private networks are
  IPv4-only and Flannel uses one interface for both families. So pods reach IPv4 only; an
  IPv6-only destination is out of their reach. The node IPs are
  written there by hand (no other module's state is readable from here), so **adding or
  replacing a node means updating that map**, until a Hetzner Load Balancer gives the
  cluster one address.
- The rename was done without an interruption: prod-old was created with the old IPs and the
  CNAMEs were repointed to it first, in a separate apply before `prod` itself changed.

### CAA: only Let's Encrypt

Every zone has `0 issue "letsencrypt.org"` and `0 issuewild "letsencrypt.org"`
(`records_caa.tf`), since every certificate issued for these names comes from Let's Encrypt
- the old server's Traefik, GitHub Pages and cert-manager. Cloudflare's edge certificates
come from Google Trust Services and SSL.com (crt.sh showed nothing else in use when this was
set); **Cloudflare adds CAA records for its own CAs automatically** once a zone has any, and
hides them from the dashboard and API, so they never appear as drift - after the apply that
was `comodoca.com`, `digicert.com`, `pki.goog` and `ssl.com`, `issue` and `issuewild` each.
`dig CAA <zone> @1.1.1.1` shows the full set. A new certificate source using another CA needs that CA added here first.

### Records Cloudflare owns but OpenTofu now tracks

Seven AAAA records point at `100::` with `meta.origin_worker_id` set and
`meta.read_only = true` - `d3strukt0r.dev` apex and www, `weleda-webcenter-text-export`,
`robines.space` apex and www, `wundexpertinplus.com` apex and www. Cloudflare creates
and maintains these for Workers routes. They imported cleanly, but if a Worker route
changes Cloudflare rewrites them and a plan will report drift that nothing in this repo
caused. Do not "fix" that drift blindly - check the Workers config first.

**`_acme-challenge` records are never imported.** They are DNS-01 challenge tokens: an ACME
client creates one, Let's Encrypt reads it once, and the client should delete it again. Two
leftovers for `portainer.d3strukt0r.dev` (from 2024-05-17 and 2026-08-31) had been imported
to keep the plan clean and were then deleted on purpose. cert-manager now creates and removes
these records itself (see "Certificates: cert-manager"); OpenTofu must not track them.

### Two zones have no delegation check, on purpose

The `ns_delegation` check in `tofu/infomaniak` covers exactly the nine domains registered at
Infomaniak (`local.domains` is specifically that set). Two further zones live in the
Cloudflare accounts - `wundexpertinplus.com` (registered at GoDaddy) and `arepazo.ch` (an
unidentified `.ch` registrar) - but those domains are registered and managed by their
owners, and this repo's admin has no access at their registrars. A check could only warn,
never correct, so there deliberately is none: whether they point at Cloudflare is the
owners' concern. If one moves away, its zone and records in `tofu/cloudflare` are what to
clean up.

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
- **Run `ansible-galaxy collection install -r requirements.yml` once.** The `ansible`
  package bundles `ansible.posix`, `community.general` and `hetzner.hcloud`, but its
  hetzner.hcloud 6.12 prints a `hcloud_datacenter` deprecation warning for every server on
  every run - unconditionally, although nothing here uses that variable. 7.0 removed it, so
  `requirements.yml` requires `>=7.0.0`; the install lands in `.collections/`, which takes
  precedence over the bundle.
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
  nothing. The trade is that editing `k3s_server_args` afterwards has no effect on a
  running node - those flags have to be right the first time.
- **Everything that may change later goes in `k3s_config` instead.** The role writes it to
  `/etc/rancher/k3s/config.yaml.d/50-ansible.yaml` (keys as in k3s's config file) before
  the install, and on a node already running k3s a change restarts it and waits for
  `readyz` before moving on - one node at a time, thanks to `serial: 1`. k3s reads drop-ins
  even without a `config.yaml` (checked in its source). The first use is
  `disable: [local-storage]`, which removed the local-path provisioner and its
  StorageClass: node disks are 40 GB and hold the OS, and data there dies with the node.
- **Kernel parameters (`k3s_sysctls`) and node labels (`k3s_node_labels`)** are the role's
  too. The sysctls go to `/etc/sysctl.d/50-ansible-k3s.conf` and apply at once, without a
  restart. Labels are set with `k3s kubectl label` on the node itself, after reading the
  current ones, because k3s's `node-label` only applies at a node's first registration and a
  kubelet may not set `node-role.kubernetes.io/*` on itself. That uses the admin kubeconfig
  every server has; an agent node would need the task delegated to a server.
  `group_vars/prod.yml` labels every node `node-role.kubernetes.io/ingress=true` (see
  "Ingress").
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
- **Pruning does not cascade from `root` into the components.** No Application carries the
  `resources-finalizer.argocd.argoproj.io` finalizer, so deleting a file from
  `clusters/<name>/` makes `root` delete only the Application object - what that
  Application deployed keeps running, orphaned and no longer tracked. A mistakenly deleted
  Application file therefore cannot take OpenBao and its volume with it. Within one
  Application pruning works as usual: a manifest removed from a component is deleted.
  Really removing a component means deleting its objects by hand, or adding the finalizer
  first and then removing the Application. For the same reason Helm's
  `helm.sh/resource-policy: keep` (which Argo CD honours as `Delete=false`) currently
  changes nothing here - there are no Helm releases either, since Argo CD only renders charts
  with `helm template`; `helm list -A` shows just k3s's own Traefik.
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
- **The UI is not exposed** - port-forward from the admin context. TLS would be available
  (cert-manager and Traefik run), but the UIs open only once Zitadel provides SSO. The local
  `admin` account is used until then, and stays as the break-glass login afterwards.

### Bootstrap secrets come from 1Password, once

`ansible/secrets.yml` (role `cluster_secrets`) writes the Secrets the cluster needs before
OpenBao can supply any - `kube-system/hcloud`, the CSI driver's Hetzner token, which
OpenBao itself needs for its volumes, and `openbao/openbao-seal`, OpenBao's own seal key. The list is `cluster_secrets` in
`inventories/group_vars/prod.yml`; each value is one field of one 1Password item in
`onepassword_account` (`my.1password.com`, pinned because a second, work account is signed
in too) and `onepassword_vault` (`Private`).

- **1Password is the bootstrap root of trust only.** The `op` CLI reads the values on the
  admin's machine (Touch ID prompt); nothing in the cluster talks to 1Password. There is no
  Ansible Vault - it would only be a second place for the same secrets.
- **Values go to `kubectl` on stdin**, never argv, and every task is `no_log`.
- **Server-side apply, field manager `ansible`**: client-side apply would copy the value
  into the `last-applied-configuration` annotation. `kubectl diff --server-side` runs
  first, read-only, so `--check` is accurate and a re-run reports `changed=0`.
- Same shape as `argocd.yml`: runs from the Mac with the admin kubeconfig, targets
  `prod-01` only for its variables, not imported by `site.yml`. These Secrets are not in
  git, so Argo CD never prunes them.

### Persistent storage is Hetzner Volumes, never the node disk

`kubernetes/components/hcloud-csi/` deploys `hetznercloud/csi-driver` (pinned raw manifest,
v2.23.0) and its default StorageClass `hcloud-volumes`. k3s's local-path is disabled
(`k3s_config`), so nothing can put persistent data on a node's 40 GB disk.

- **A volume is not node storage.** Hetzner keeps every block on three physical servers
  and attaches the volume over the network to one server at a time; deleting a server
  detaches, never deletes. The driver finds its server through the metadata service, so no
  cloud controller manager is needed.
- **Limits:** `ReadWriteOnce` only, 16 volumes per server (the driver reports it to the
  scheduler), 10 GB minimum, nbg1 only, price linear per GB. A node that vanished without
  being deleted keeps its volumes until tainted `node.kubernetes.io/out-of-service`.
- **`reclaimPolicy: Retain`** (`patches/reclaim-retain.yaml`): Hetzner has **no backups of
  volumes** and Argo CD prunes, so a deleted PVC leaves a `Released` PV and the volume, to be
  removed by hand. reclaimPolicy is immutable on a StorageClass.
- **Consolidate at the service, not the disk.** PVCs are namespaced and RWO, so sharing one
  between apps does not work. One Postgres cluster (CloudNativePG, a database per app,
  backups to S3), files in S3, Redis without a volume. Databases want the volume mounted
  directly - never NFS, SMB or S3 as primary storage. A shared POSIX folder, if ever needed,
  is an NFS server on one volume.
- **The token** (`kube-system/hcloud`, from `ansible/secrets.yml`) is project-wide
  read+write - Hetzner cannot scope tokens, and volumes must live in the servers' project.
  The driver's code calls only volume endpoints plus a read of servers; server
  `delete_protection` and tofu drift detection are the backstop. It is a dedicated token,
  revocable without touching OpenTofu's.
- **Kubernetes 1.37 is not yet in the driver's CI** (k3s 1.33-1.36 when adopted).

### OpenBao: one replica, Raft, a static seal from 1Password

`kubernetes/clusters/prod/openbao.yaml` deploys the `openbao/openbao` Helm chart (pinned,
0.29.6 = OpenBao 2.6.3) with values from `kubernetes/components/openbao/values.yaml` - a
two-source Application, because OpenBao publishes only a chart. The pattern for Helm-only
upstreams: chart pinned in the cluster's Application, values in `components/`.

- **One replica, no HA preparation** (no `retry_join`), a deliberate simplicity choice like
  Argo CD's non-HA. External Secrets copies values into Kubernetes Secrets, so OpenBao being
  down while its pod moves delays changes but breaks no running workload.
- **Raft storage**, not the chart's default `file`, even with one node: Raft snapshots are
  the backup format (and the chart's `snapshotAgent` can ship them to S3).
- **Static seal** (`seal "static"`, OpenBao 2.4+): a 32-byte key, hex, in Secret
  `openbao/openbao-seal`, written by `ansible/secrets.yml` from 1Password item
  `OpenBao | Prod | Seal key`. OpenBao unseals itself on every start, and the cluster never
  talks to 1Password. **Losing that key makes the data and all snapshots unreadable.**
  Rotation adds the new key and moves the old one to `previous_key`, never a replacement.
- **Initialised once, by hand** (`bao operator init` via `kubectl exec`), done 2026-09-24. The
  five recovery keys and the initial root token are in their own 1Password item,
  `OpenBao | Prod | Recovery keys & root token`, apart from the seal key - which is written
  once and never edited, while the root token will be revoked and regenerated. There is no
  automation for this on purpose: it happens once per storage lifetime and its output must go
  straight into 1Password.
- **`tls_disable = 1`** on the listener. Traffic to OpenBao is still encrypted between
  nodes, by the pod network's WireGuard tunnels (see "Pod traffic between nodes is
  encrypted"); TLS on the listener would only add server authentication - clients could
  verify they talk to the real OpenBao.
- The injector is off; secrets reach workloads through External Secrets.
- **Its configuration is `tofu/openbao`**: the KV v2 engine `secret/`, Kubernetes auth, and
  the `external-secrets` policy and role. OpenBao is cluster-internal, so that module reaches
  it through `kubectl port-forward svc/openbao 8200:8200` and authenticates with the root
  token from its gitignored tfvars until an admin login exists. Secret **values** never go
  through OpenTofu - they would land in state - but in with `bao kv put`. The ESO policy reads
  all of `secret/`, so every namespace referencing the ClusterSecretStore reaches every value:
  fine for a single-admin cluster, to be narrowed per namespace once others deploy. Audit
  devices, if wanted, belong in the server config, not the API.
- **External Secrets delivers the values** (`kubernetes/clusters/prod/external-secrets.yaml`:
  chart `external-secrets` 2.11.0 pinned, plus `kubernetes/components/external-secrets/`
  with the `ClusterSecretStore` `openbao`). `ServerSideApply=true` is required there - the
  chart's CRDs exceed client-side apply's annotation limit - and the store carries
  `SkipDryRunOnMissingResource=true` because its CRD arrives in the same sync. Release name
  and namespace are both `external-secrets`, which yields exactly the service account the
  OpenBao role is bound to; the store sets no `serviceAccountRef`, so the operator logs in
  with its own token. It only sets what differs from the CRD defaults (`version` v2 and
  `mountPath` kubernetes are defaults).

### etcd snapshots go to Object Storage

k3s takes its scheduled snapshots (00:00 and 12:00 UTC) onto each server's disk and uploads
each to `d3strukt0r-prod-etcd`.

- **Retention is 5 (the default) - per node on disk, but in total in the bucket.** All three
  servers prune the same `etcd-snapshot-*` names in the one bucket, so it holds the newest 5
  of all of them: about a day, the last two rounds (seen 2026-09-29, the docs do not say so).
  Every pruned snapshot stays readable for 7 more days as a noncurrent version (object lock
  and the lifecycle rule below), so a week can be restored, the older part by version ID - see
  "etcd snapshots" in the README. Raising it would be `etcd-snapshot-retention` in
  `k3s_config`, which sets the S3 retention too: the Secret has no retention key, and the
  `etcd-s3-retention` flag, like every `etcd-s3-*` flag, would make k3s ignore the Secret.

- **All S3 settings come from Secret `kube-system/k3s-etcd-snapshot-s3-config`.**
  `k3s_config` sets only `etcd-s3: true` and `etcd-s3-config-secret`; per k3s's docs, **any
  other `etcd-s3-*` flag makes it ignore the Secret.** The Secret is built by the
  ExternalSecret in `kubernetes/clusters/prod/etcd-snapshots/` (Application
  `etcd-snapshots`, in the cluster directory rather than `components/` because the bucket
  is prod's): endpoint, region `nbg1` and bucket in git, the cluster's own S3 key from
  OpenBao `secret/etcd-snapshot-s3` (`access-key`, `secret-key`). That key is kept in
  1Password (`Hetzner | S3 | prod etcd snapshots`) and copied into OpenBao with `bao kv put`
  (command in `kubernetes/README.md`); a restore uses the admin key instead. Console label
  `prod etcd snapshots`.
- **k3s's pruning only adds delete markers** on this versioned bucket; each version stays
  locked 7 days and the lifecycle rule removes it afterwards. The cluster key cannot bypass
  the lock or touch any other bucket, and no other cluster key can touch this one (the
  bucket policies, see "Object Storage").
- **A restore cannot use the Secret** - the apiserver is not running then. It takes the S3
  settings as CLI flags (`--etcd-s3-endpoint`, `--etcd-s3-region`, `--etcd-s3-bucket`,
  keys from the `d3strukt0r-hetzner` profile) **and the original server token**, which
  decrypts the bootstrap data inside the snapshot. The token is in 1Password,
  `k3s | Prod | Server token`, copied from `/var/lib/rancher/k3s/server/token`.

### k3s upgrades itself

`kubernetes/components/system-upgrade-controller/` deploys Rancher's system-upgrade-controller
(pinned release manifests, v0.20.2) and one Plan, `server`, covering all three nodes. For each
node it runs a privileged Job that replaces the k3s binary and restarts k3s. Ansible's
`k3s_version` is therefore only the version a node is *installed* with; the Ansible install
never upgrades (`creates:`), and a new node catches up in the next window.

- **Channel `v1.37`, not `stable` - yet.** `stable` was still 1.36 when the cluster was
  installed at 1.37.0, and k3s-upgrade refuses to downgrade: the Job fails with
  `Current … is higher` and leaves the node cordoned. Once `stable` reaches 1.37 the channel
  switches to `stable`, which makes minor upgrades automatic too. Until then a minor is a
  one-line change to the channel, **never skipping a minor** (Kubernetes' skew policy).
  Before any minor, check that the Hetzner CSI driver supports it - its CI lagged by one.
- **Window 02:30-03:30 Europe/Zurich, daily**, after the scheduled etcd snapshot at
  00:00 UTC, so each upgrade starts with a fresh snapshot in Object Storage, and before the
  OS updates (see "The night's maintenance order"). The channel is polled every 15 minutes;
  Jobs start only inside the window but may run past it.
- **One node at a time, cordoned but not drained.** Restarting k3s leaves running pods alone
  (the containerd shims survive), so a drain would only move volumes around. The API is
  briefly unavailable on each node; etcd keeps quorum with two of three.
- **A failed Job leaves its node cordoned.** Look at the Job's log in `system-upgrade`, fix
  the cause, then `kubectl uncordon <node>`.
- The script exits early when the binary is already the target version, so re-running a
  Plan is harmless.

### The night's maintenance order

Everything that restarts or changes a node happens at night and is done by 06:00 Zurich
time, in windows that never overlap - so an upgrade and a reboot can never take out two
etcd members at once. All times are Europe/Zurich; the nodes' clocks stay on UTC.

| Time | What |
|---|---|
| 02:00 (01:00 in winter) | etcd snapshot (00:00 UTC, k3s's schedule) |
| 02:30-03:30 | k3s upgrades (system-upgrade-controller Plan window) |
| 03:30 + up to 15 min | OS updates (unattended-upgrades) |
| 04:30-06:00 | reboots by kured, only where a kernel update asks for one |

**OS updates** come from the Hetzner Debian image itself: `unattended-upgrades` is enabled
and installs from Debian and Debian-Security. `ansible/roles/os_updates` only moves its
`apt-daily-upgrade.timer` (Debian's default is 06:00 on the node's clock plus up to an hour)
with a drop-in, `/etc/systemd/system/apt-daily-upgrade.timer.d/50-ansible.conf`; systemd
evaluates the time zone in `OnCalendar` itself. `apt-daily.timer`, which refreshes the
package lists twice a day, is left alone. Debian's `Automatic-Reboot` stays off: it would
reboot every node at the same time.

**Reboots are kured's** (`kubernetes/components/kured/`, pinned release manifest 1.23.0, in
`kube-system`). A kernel update leaves `/var/run/reboot-required` behind - written by
unattended-upgrades' hook in `/etc/kernel/postinst.d/` - and kured, checking every 10
minutes inside its window, takes a lock on its own DaemonSet, cordons and **drains** the
node, reboots it and uncordons it once it is back; then the next node may take the lock.

- **Drain, unlike the k3s upgrade.** A reboot does stop every container, so pods are
  evicted first. OpenBao moves to another node and its volume re-attaches there, so it is
  briefly unavailable; External Secrets only delays refreshes meanwhile.
- **A drain that cannot finish blocks the reboot** (no `--force-reboot`), and nothing ever
  times it out (`--drain-timeout` 0): a PodDisruptionBudget allowing no disruption keeps the
  node cordoned and waiting. `kubectl -n kube-system logs -l name=kured` shows it.
- **Only kernels set the sentinel.** A host service using an updated library keeps the old
  copy until it restarts - there is no `needrestart` on these nodes - but containers bring
  their own libraries anyway, so this only concerns the few host daemons.

### Default resources through Kyverno

`kubernetes/clusters/prod/kyverno.yaml` deploys the `kyverno` Helm chart (pinned, 3.9.1 =
Kyverno v1.19.1) with `kubernetes/components/kyverno/` as values and extra resources - the
same two-source pattern as OpenBao, plus a third source for the directory's manifests. Its
policy `default-resources` gives every namespace but a few system ones a LimitRange, so a container without
`resources` of its own still has requests and limits; `allowed-registries` is described under
"Allowed registries".

- **Defaults, never a max.** Requests CPU `50m`, memory `64Mi`, ephemeral-storage `50Mi`;
  limits memory `128Mi`, ephemeral-storage `1Gi`. An app that needs more sets `resources`
  on its containers, which always wins. The ephemeral-storage limit keeps a container from
  filling a node's 40 GB disk, which holds etcd too.
- **No CPU limit, on purpose.** A CPU limit throttles a container even while the node is
  idle; requests already share CPU fairly under contention.
- **A container sets its memory itself where the defaults do not fit** - more than 64Mi
  used or a limit above 128Mi - and everywhere in `kube-system` and `system-upgrade`, which
  have no LimitRange; through a patch or Helm value in its component. Where the defaults fit,
  nothing is set, so there is nothing to keep in step. Without a request the scheduler reserves nothing for it (Argo CD's application controller uses
  1.2-1.6 GB), and without a limit a leak can take the node's memory. The values were set on
  2026-09-28 from Prometheus: the request about the usual use, the limit about twice the
  highest seen, 128Mi at the least. They rest on a few hours of data, so an OOM kill means
  raising that limit - `kubectl get pod` shows the restarts, `kubectl describe pod` the
  `OOMKilled` reason. Only what k3s itself deploys (CoreDNS, metrics-server, its Helm install
  Jobs) is left as k3s sets it.
- **Only four namespaces are left out** (the policy's `matchConditions`): `kube-system`,
  whose pods k3s manages; `system-upgrade`, whose k3s upgrade Jobs set no memory and must not
  be killed halfway through replacing the binary; and the pod-less `kube-public` and
  `kube-node-lease`. Infrastructure is covered like any app - a component that needs more
  than the defaults sets its own values, which always win. `kyverno` gets none regardless:
  Kyverno's own resource filter (`[*/*,kyverno,*]` in its ConfigMap) ignores its namespace,
  and its chart sets every container's resources itself. **This list and
  `k3s_psa_exempt_namespaces`** (see "Pod Security") **are independent**: one is about
  default resources, the other about privileges.
- **`GeneratingPolicy`, not `ClusterPolicy`**, which is deprecated since Kyverno's CEL-based
  policy types. `generateExisting` covers namespaces created before the policy;
  `synchronize` keeps the LimitRange in step with the policy and removes it with its
  namespace - hand edits to it are reverted.
- **Kyverno needs RBAC for what it generates.** Its background controller's own ClusterRole
  covers only a few kinds; `rbac-limitranges.yaml` adds LimitRanges through the aggregation
  label `rbac.kyverno.io/aggregate-to-background-controller`. Generating another kind means
  another such role.
- **Argo CD settings Kyverno needs**, from Kyverno's own notes: `ServerSideApply=true`
  (huge CRDs), `ServerSideDiff=true,IncludeMutationWebhook=true` on the Application,
  `config.preserve: false` (the default marks Kyverno's ConfigMap `helm.sh/resource-policy:
  keep`, which would orphan it when the Application is removed; Kyverno's notes still
  describe an older post-delete hook, which chart 3.9.1 no longer renders),
  and `ignoreAggregatedRoles: true` in `argocd-cm`, since aggregated ClusterRoles never
  match git. Argo CD's own defaults already exclude Kyverno's report kinds.
- The policy was tested offline with the Kyverno CLI (`kyverno apply <policy> --resource
  <namespaces>`), which is the quickest way to try a change before pushing.

### Pod traffic between nodes is encrypted

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

### Pod Security

Every namespace runs under the **restricted** Pod Security Standard, enforced by the API
server itself (Pod Security Admission, built into Kubernetes) - not by Kyverno. A webhook
policy fails open when Kyverno is down; the built-in admission cannot be down while the API
server is up. `ansible/roles/k3s` writes `/etc/rancher/k3s/psa.yaml` (an
`AdmissionConfiguration` with defaults `enforce`, `audit` and `warn` = `restricted`,
version `latest`) and points the API server at it through `kube-apiserver-arg:
admission-control-config-file=...` in `k3s_config` - the shape of k3s's own hardening
guide. A change to the file restarts k3s one node at a time, like a drop-in change.

- **Restricted means**, for every container: `runAsNonRoot`, `allowPrivilegeEscalation:
  false`, `capabilities.drop: [ALL]` (only `NET_BIND_SERVICE` may be added back),
  `seccompProfile: RuntimeDefault`, and no host namespaces, host paths or privileged mode.
  A pod that misses one is rejected at creation, with the reasons in the error.
- **Infrastructure namespaces are exempt** - `k3s_psa_exempt_namespaces` in the role's
  defaults. Some of it must run privileged: kured and the CSI driver in `kube-system`, the
  upgrade Jobs in `system-upgrade`. A new infrastructure component that needs more than the
  restricted standard goes onto that list in the commit that deploys it; components that run
  restricted anyway (`cert-manager`, `monitoring`, `loki`) are not on it.
- **The namespace label can override the default.** A namespace labelled
  `pod-security.kubernetes.io/enforce: baseline` (or `privileged`) gets that instead, so
  whoever may edit namespaces - today only the admin - can loosen it. Prefer adding a namespace to the exemption list over
  a label, so every exception stays in one reviewed place.

### Allowed registries

Pod Security cannot say where an image comes from, so that is Kyverno's:
`kubernetes/components/kyverno/allowed-registries.yaml`, a `ValidatingPolicy` in `Deny`
mode. In every namespace, infrastructure included, every container, init
container and ephemeral container must come from `docker.io`, `ghcr.io`, `quay.io`,
`registry.k8s.io` or `public.ecr.aws` - whole hosts, not single organisations (user
decision). Kyverno itself is set to pull from `ghcr.io` (`global.image.registry` in its
values) instead of its default `reg.kyverno.io`.

- **`kube-system` and `kyverno` are never checked**: Kyverno's own webhook configuration
  (its `config.webhooks` default) keeps them out, so a broken Kyverno cannot block CoreDNS,
  the CSI driver or its own restart. Their images happen to comply anyway.
- **`failurePolicy: Ignore`**: with Kyverno down, pods start unchecked instead of not at all.
  `Fail` would also stop Argo CD from restarting - the tool that repairs Kyverno - and need
  Kyverno's webhook configuration deleted by hand. Images only come from git, so the check
  guards against mistakes, not against someone timing a Kyverno outage.
- **A chart update that moves its images to another registry is refused** at sync, and Argo
  CD shows the error: either the chart gets a registry override, as Kyverno did, or the
  registry is added to the list.

- **Docker Hub is `index.docker.io` in the list**, not `docker.io`: Kyverno's CEL image
  library reports that registry for `nginx`, `rancher/x` and `docker.io/x` alike.
- **Hosts match exactly** (`docker.io.evil.com` is refused), and a reference the library
  cannot parse is refused too - `evil.example.com/x`, with a host but a single path
  segment, is one.
- **autogen** covers Deployments, StatefulSets, DaemonSets, Jobs and CronJobs, so a bad
  image is refused when the controller is applied, not later as a pod that never appears.
- Adding a registry is one entry in the `allowed` variable. Test a change offline first:
  `kyverno apply <policy> --resource <pods and deployments>`.

### Ingress

Traefik is k3s's own - k3s installs and upgrades it from its bundled chart - and is
configured only through the `HelmChartConfig` in `kubernetes/components/traefik/`
(Application `traefik`). It runs as a **DaemonSet on every ingress node's host network** and
listens on the node's ports 80 and 443 itself, IPv4 and IPv6.

- **Why the host network**: the cluster is single-stack IPv4, and k3s's servicelb only
  rewrites IPv4 packets to a pod (iptables DNAT; a pod without an IPv6 address has nothing to
  rewrite IPv6 to). On the host network Traefik accepts IPv6 directly and talks to the pods
  over the IPv4 pod network, reaching pods on other nodes through the WireGuard tunnels. Its
  Service is `ClusterIP` (`service.spec.type` in this chart, not `service.type`), so servicelb
  no longer claims 80/443. Pods themselves still cannot reach IPv6-only destinations; if that
  is ever needed, a forward proxy on the host network is the targeted fix, dual-stack (a
  rebuild) the complete one.
- **Ingress nodes are the ones labelled `node-role.kubernetes.io/ingress=true`**
  (`k3s_node_labels`, today all three). **That label, the DNS map `local.prod_nodes` in
  `tofu/cloudflare` and later a load balancer's targets must list the same nodes** - a node in
  DNS without Traefik answers nothing. A workload-only node gets no label: no Traefik, not
  internet-facing. There is no standard ratio; keep at least two or three for redundancy and
  size them by traffic, not by node count.
- **Ports below 1024 without root**: Traefik runs as UID 65532 without capabilities;
  `net.ipv4.ip_unprivileged_port_start = 80` on the nodes (`k3s_sysctls`) lets it bind 80 and
  443. Its other entrypoints, metrics `:9100` and `:8080`, also open on the host but are closed by
  the Hetzner firewall.
- **Updates replace one node at a time** (`maxUnavailable: 1`, `maxSurge: 0`): two Traefiks
  cannot hold the same host port. That node's share of requests fails for the seconds in
  between, as during a kured reboot - DNS keeps pointing at it.
- **A Hetzner Load Balancer fits on top without changing Traefik**: targets by label over the
  private network, TCP 80 and 443 passed through (TLS stays at Traefik), health checks, PROXY
  protocol for client IPs, and one address (IPv4 and IPv6) in DNS instead of one per node. The
  firewall could then close 80/443 on the nodes' public side.

### Certificates: cert-manager

`kubernetes/components/cert-manager/` deploys cert-manager (pinned release manifest,
v1.21.2; `ServerSideApply=true` is required, its CRDs are too large otherwise) and two
ClusterIssuers: `letsencrypt-staging` for trying things, `letsencrypt` for anything serving
traffic.

- **DNS-01 over the Cloudflare API**, not HTTP-01: cert-manager proves a name by creating an
  `_acme-challenge` TXT record and deletes it afterwards. No records need preparing, Let's
  Encrypt never has to reach the cluster, and it works for cluster-internal names and
  wildcards. No zone has CAA records, so Let's Encrypt may issue (CAA is a planned
  hardening - it must then also list Cloudflare's own CAs for its edge certificates).
- **Two tokens, one per Cloudflare account** - a token cannot span accounts. Each issuer
  has two solvers: one selecting `arepazo.ch` (the arepazo account's only zone) with the arepazo
  token, and one without a selector - the fallback for every other zone - with the personal
  token. cert-manager uses the most specific match. A zone added to the arepazo account
  must be added to that `dnsZones` list; the personal account needs nothing.
- **Tokens**: `Zone:DNS:Edit` + `Zone:Zone:Read` on "All zones from an account", no expiry
  (an expiring token would silently stop renewals) and **no client IP filter** - nodes may be
  added or replaced, and a filter on today's node IPs would break cert-manager on any new
  one. They are protected by where they live instead, in
  1Password as `Cloudflare | cert-manager DNS (prod cluster)` and
  `Cloudflare | Arepazo | cert-manager DNS (prod cluster)`, copied into OpenBao `secret/cloudflare-dns` (fields `personal`,
  `arepazo`) with `bao kv put`, and delivered as Secret `cert-manager/cloudflare-api-tokens`
  by an ExternalSecret. ClusterIssuers read their Secrets from cert-manager's own namespace.
- **No email on the ACME accounts**: optional, this repo is public, and Let's Encrypt no
  longer sends expiry mails. cert-manager renews by itself 30 days before expiry.
- **Its pods run restricted** (non-root, seccomp, no capabilities), so `cert-manager` is not
  in `k3s_psa_exempt_namespaces`.

### Monitoring

Each monitoring component gets its own namespace, under the restricted Pod Security
Standard; components that need the host (node-exporter, the log collector) go to
`kube-system` instead. The UIs are reached by port-forward only until Zitadel provides SSO.

**Uptime Kuma** (`kubernetes/components/uptime-kuma/`, plain manifests, image pinned,
namespace `uptime-kuma`) has two
jobs: checking the websites, and receiving Alertmanager's always-firing `Watchdog` alert as a
heartbeat - a push monitor that alerts through ntfy when the heartbeat stops, so a broken
alerting pipeline is noticed too. Its monitors and notifications live in its SQLite database,
configured in the UI (runbook and monitor list in `kubernetes/README.md`), not in git.

- **One replica, `strategy: Recreate`**: the SQLite file sits on a ReadWriteOnce volume, which
  only one node can attach; a rolling update would wait forever for the old pod's volume.
- **The rootless image runs as UID 1000**, set numerically in the pod because the image only
  names its user (`node`) and `runAsNonRoot` cannot check a name. `fsGroup` hands the fresh
  volume, owned by root, to that group. The root filesystem is read-only and `/tmp` an
  `emptyDir`; both were tested locally before the first deploy.
- **`UPTIME_KUMA_DB_TYPE=sqlite`** skips the first-start database wizard.
- **`enableServiceLinks: false`**: Kubernetes would inject `UPTIME_KUMA_PORT=tcp://...` for the
  Service of the same name, and Uptime Kuma reads that variable as its listen port.
- **It runs like an app, not like infrastructure**: its namespace is not PSA-exempt. It sets
  only its memory, which the defaults would undersize; the generated LimitRange fills in the
  CPU request and ephemeral storage. Its image comes from Docker Hub. Its own
  namespace also means that moving it out of the cluster is deleting one namespace.
- **It cannot report a dead cluster** while it runs inside it: a complete outage takes it down
  too. It moves to a machine outside (the home server) later; then it checks the cluster from
  there, and Alertmanager's heartbeat goes to that address.
- Since 2.0, the push endpoint accepts POST, so Alertmanager's webhook can call it directly - no
  translating proxy.

**kube-prometheus-stack** (`kubernetes/clusters/prod/kube-prometheus-stack.yaml`, chart
pinned, values and extra manifests in `kubernetes/components/kube-prometheus-stack/`) runs
Prometheus, Alertmanager and Grafana in `monitoring` - an ordinary namespace, not
PSA-exempt: the LimitRange's defaults fit most of its containers, so the values set memory
only for Prometheus and Grafana, which need more, and for Alertmanager, whose operator would
otherwise request 200Mi. node-exporter goes to `kube-system`
(`namespaceOverride`), since it needs the host's network, PIDs and files; there is no
LimitRange there, so it sets its own.

- **Prometheus keeps what fits in 8 GB** of its 10 GB volume (`retentionSize`, oldest first),
  roughly three weeks at ~350 MB a day, capped at 30 days. It started on 30 GB and was
  recreated at 10 GB: a volume can grow but never shrink, so the smaller start is the
  cheaper mistake. Alertmanager and Grafana have no volume: silences and anything
  changed in Grafana's UI are lost on restart - dashboards and datasources come from the chart
  and from ConfigMaps (the sidecar reads every ConfigMap labelled `grafana_dashboard: "1"`).
- **k3s is one process, so three scrapes are disabled** (controller manager, scheduler,
  proxy - there is nothing separate to reach), and with them their alerts and dashboards.
- **k3s serves one shared metrics registry on every port**: the API server's and each
  kubelet's `/metrics` return the same ~64,000 series per node (measured when adopted). The
  API server keeps them, minus histogram buckets that no rule or dashboard of the chart reads
  and minus the kubelet's own families; the kubelet keeps only its own (a keep list). Through
  the API server, the kubelet's volume metrics would also carry `namespace="default"` instead
  of the PVC's, and every volume alert would fire twice. Without that, Prometheus would hold
  roughly 380,000 series, half of them duplicates.
- **etcd is scraped by a `ScrapeConfig`** (`scrape-etcd.yaml`): the private IPs, port 2381, as
  opened by `etcd-expose-metrics` in `k3s_config`, which binds to the node's own address only.
  The chart's own way renders an Endpoints object, which Argo CD does not manage. `kubeEtcd`
  stays enabled with its Service and ServiceMonitor off, because disabling it would drop the
  etcd alerts and dashboard too; the job name `kube-etcd` matches them. **A new server node is
  one more target there.** Each target's `instance` is set to the node name rather than adding a
  `node` label: the chart's etcd alerts group `without (instance)`, and with a `node` label
  every member was a group of its own, so one member down fired `etcdInsufficientMembers`
  (failover test, 2026-09-28).
- **node-exporter listens on 9101**, not its default 9100: Traefik's metrics entrypoint holds
  9100 on the same host network. Both are closed by the Hetzner firewall.
- **The admission webhook's certificate comes from cert-manager**
  (`admissionWebhooks.certManager`), which removes the chart's Helm hook Jobs. The chart
  renders the webhooks without a `caBundle` and cert-manager's cainjector fills it in; since
  git never sets the field, Argo CD sees no difference and needs no `ignoreDifferences` - as
  with cert-manager's own webhook.
- **Alerts go to ntfy.sh, one topic, two priorities.** Alertmanager's webhook cannot set
  ntfy's priority header, so the priority goes into the topic URL, and there is one receiver
  per severity: `critical` at priority 5, everything else at 3, `info` nowhere (it stays in
  Grafana). ntfy's built-in `alertmanager` template formats the message. The URLs and token
  are assembled by the ExternalSecret `alertmanager-ntfy` from OpenBao `secret/ntfy` and read
  through `url_file`/`credentials_file`, so the topic - the secret on the free plan - never
  enters git or the Alertmanager config.
- **The Watchdog heartbeat**: the chart's always-firing `Watchdog` alert is routed to Uptime
  Kuma's push URL (OpenBao `secret/uptime-kuma`) every minute. Uptime Kuma alerts when it has
  not heard from it for about ten minutes - Alertmanager down, Prometheus down, or its rules
  not evaluating.
- **Own alerts** (`rules.yaml`): a node cordoned for over 2 hours (a failed upgrade Job or a
  drain that cannot finish), a server without a successful etcd snapshot upload for 13 hours
  or with a failed one, and a k3s certificate within 30 days of expiry (k3s renews at start
  within 120 days, so this means renewal failed). Their descriptions say what to do. The
  missing-upload alert reads the age of each node's newest S3 snapshot from k3s's
  `ETCDSnapshotFile` records, not from k3s's upload counter: after a k3s restart the counter
  reappears only with the next upload, already at 1, and `increase()` reads that as none - it
  fired falsely after the failover test.
- **Volume alerts at three levels** (`rules.yaml`): 80 % warning, 90 % critical, 95 % critical
  again, each a separate alert so every level notifies once; inhibit rules in the
  Alertmanager config let a higher level silence the lower ones for the same PVC. They replace
  the chart's `KubePersistentVolumeFillingUp` (disabled), which only warned at 15 % free when
  a trend predicted it full within four days.
- **Grafana's admin** comes from OpenBao `secret/grafana` through the ExternalSecret
  `grafana-admin`; the chart would otherwise generate a new random password on every render.
- **kube-state-metrics also reads operator objects** (`customResourceState` in the values):
  the status conditions of `MariaDB` and `PhysicalBackup` become
  `kube_customresource_condition{customresource_kind, name, type, status, reason}`, valued with
  the condition's `lastTransitionTime`, for the MariaDB alerts. The mariadb-operator reports
  archiving and backups nowhere else. k3s's `ETCDSnapshotFile` records become
  `kube_customresource_etcd_snapshot_created{node, name, bucket}` (creation time; `bucket` only
  on the S3 copies), for the snapshot alert. Another kind is one more entry there plus its
  `list`, `watch` in `rbac.extraRules`; the config can be tried locally by running the
  kube-state-metrics image with `--custom-resource-state-only` against the admin kubeconfig.

**Logs: Loki and Alloy.** Alloy (`kubernetes/clusters/prod/alloy.yaml`, chart `grafana/alloy`
pinned) runs on every node and sends that node's container logs (`/var/log/pods`) and whole
journal (k3s, kernel, apt, sshd), plus the cluster's Kubernetes events, to Loki
(`kubernetes/clusters/prod/loki.yaml`, chart `grafana-community/loki` pinned - the chart moved
out of `grafana/helm-charts`), which keeps them 30 days in `d3strukt0r-prod-loki`. Grafana has
Loki as a datasource (`additionalDataSources` in the kube-prometheus-stack values).

- **Loki is one monolithic instance** in namespace `loki`, an ordinary namespace like
  `monitoring`. The chart defaults `write`/`read`/`backend` to 3 replicas each and refuses them
  next to `singleBinary.replicas: 1`, so they are set to 0. Gateway, canary, Helm test and the
  memcached caches are off - `chunksCache` alone would reserve several GB. Single tenant
  (`auth_enabled: false`).
- **The Thanos object-store client** (`use_thanos_objstore: true`), built on minio-go - the
  client k3s's etcd snapshots already upload to Hetzner with. Loki's default aws-sdk-go-v2
  client sends checksum headers that S3-compatible stores have rejected. If Hetzner ever
  refuses it, the fallback is the default client with `AWS_REQUEST_CHECKSUM_CALCULATION` and
  `AWS_RESPONSE_CHECKSUM_VALIDATION` set to `when_required`.
- **Its key** is its own (console label `prod loki`, OpenBao `secret/loki-s3`), delivered by
  the ExternalSecret `loki-s3` as environment variables that `-config.expand-env` fills into
  the config. The bucket policy admits only it and the admin key.
- **Retention is the compactor's** (`retention_period: 720h`, `delete_request_store: s3`);
  the bucket's lifecycle rule only clears uploads that never completed.
- **Neither chart's RBAC is used.** Loki's rules sidecar would bring a ClusterRole reading every
  Secret; with the ruler (log-based alerting, not used yet) and sidecar off and
  `rbac.namespaced`, no RBAC is rendered. Alloy's chart role reads every Secret and ConfigMap
  too, so `rbac.create: false` and `components/alloy/clusterrole.yaml` grants only pods,
  namespaces and events.
- **Alloy runs in `kube-system`**, the PSA-exempt namespace, because it mounts `/var/log` from
  the host. The log files belong to root with mode 0640, so it runs as UID 0 - but with no
  capabilities, no privilege escalation and a read-only root filesystem; as the files' owner,
  root needs no capability to read them. Its read positions live on the node
  (`/var/lib/alloy`), so a restarted pod neither re-sends nor skips lines, and it mounts
  `/etc/machine-id`, by which the journal reader finds the node's own journal.
- **Events are collected once**: the Alloy instances form a cluster, and a
  `loki.source.kubernetes_events` watching all namespaces runs on one of them. The API keeps
  events for an hour; Loki keeps them 30 days.
- **Labels**: `namespace`, `pod`, `container`, `node`, `app` for containers; `job="node-journal"`,
  `unit`, `node` for the journal; `job="loki.source.kubernetes_events"` for events. Anything
  else is searched in the line, not labelled - Loki stays fast with few labels.
- Alloy sends no usage statistics (`enableReporting: false`).

### App database: MariaDB

The apps' database is MariaDB run by the community mariadb-operator
(`kubernetes/clusters/prod/mariadb-operator*.yaml`, charts pinned, values in
`kubernetes/components/mariadb-operator/`): a primary and one replica with automatic failover,
on 2 volumes (user decision, 2026-09-28).

- **Why not Galera** (Percona XtraDB Cluster, MariaDB Galera): Galera decides by majority vote,
  so 2 members lose the vote when one crashes and the survivor stops accepting writes. That
  needs 3 volumes, or a data-less arbitrator (garbd), which no maintained operator supports.
  Replication instead lets the operator, through the Kubernetes API, pick the new primary.
- **The known weak spot, accepted:** on a hard node loss the failover can hang until the node
  returns (17 minutes seen on k3s), and a node that stays dead needs manual steps
  ([mariadb-operator#1628](https://github.com/mariadb-operator/mariadb-operator/issues/1628)).
  The MariaDB instance's settings, alert and runbook mitigate it.
- **The CRDs are their own Application, which never prunes.** Deleting a CRD deletes every
  resource of its kind, so a MariaDB and its pods would go with it. The chart is 800 KB
  (`ServerSideApply=true` required). Bump both Applications together.
- **The webhook's certificate comes from cert-manager** (a self-signed Issuer of the chart's
  own; an ACME issuer cannot certify `.svc` names), so the chart's cert-controller Deployment
  is not rendered. The chart has no Helm hook Jobs.
- **The operator and its webhook each run twice, on different nodes** (`ha`, `webhook.ha`,
  required anti-affinity, PDBs). The operator performs the failover, so a single replica on
  the primary's node would die with it and the failover would wait about five minutes for
  Kubernetes to move it; with two, the standby takes over once the leader's 15-second lease
  runs out. Every change to a MariaDB passes the webhook with `failurePolicy: Fail`, the
  failover's own included: in the first failover test (2026-09-28, the primary's node powered
  off) the single webhook pod died with the node, the failover could not write, and it hung
  until the node was declared gone with the out-of-service taint.
- **Restricted Pod Security needs explicit securityContexts** - the chart sets none. The
  images the operator deploys are written fully qualified in `config.*` (docker.io, quay.io),
  so the registry check does not depend on short-name resolution.
- Kubernetes 1.37 is not yet in the operator's CI (1.36 when adopted).

**The instance** (`kubernetes/clusters/prod/mariadb.yaml`, `kubernetes/components/mariadb/`,
namespace `mariadb`): `MariaDB` `mariadb`, image pinned, 2 replicas on 10 GB each, one per
node. Apps connect to the Service **`mariadb-primary`**, which follows the primary through a
failover (`mariadb-secondary` reads from the replica).

- **The mitigations for #1628:**
  - `semiSyncWaitPoint: AfterSync` - a commit waits until the replica has it, so a failover
    loses nothing while the replica is up.
  - `semiSyncBootAsReplica` - a returning old primary boots read-only until configured.
  - `standaloneProbes` - the replication-aware liveness probe would restart exactly the
    replica a failover needs.
  - `readinessProbe.failureThreshold: 10` - with the default 3 the replica turns unready
    before the failover starts, leaving no candidate. The value comes from the issue thread;
    the failover tests measure what it needs here.
  - `MariaDBNoReadyPrimary` (critical, 5 min) - no ready endpoint behind `mariadb-primary`,
    i.e. a failover that hangs; the runbook is in `kubernetes/README.md`.
- **Known MariaDB bug, accepted: a replica returning under a new IP hangs the primary** (found
  and reproduced 2026-09-29). When the replica's node dies hard, its connection on the primary
  stays open - no FIN or RST ever arrives. When the replica comes back as a new pod (new IP), the
  primary ends that old connection, and `Ack_receiver::remove_slave()` waits for the semi-sync
  ack receiver thread to confirm - but that thread sits in `poll(..., -1)` on the replica
  sockets, and unlike `add_slave()`, `remove_slave()` does not wake it with `signal_listener()`
  (`sql/semisync_master_ack_receiver.cc`, unchanged on MariaDB's main branch). It wakes only when
  the kernel gives the dead connection up - with `tcp_retries2` 15, up to ~15 minutes.
  Meanwhile the new replica connection and every new login hang; the liveness probe restarts
  the primary after 30 s, sometimes followed by a failover and a replica rebuild. It heals
  itself within about a minute and loses nothing. A replica returning under its old IP does not
  hit it (its kernel resets the old connection).
  - Not fixed by `net_write_timeout` (tested at 15 s), and TCP keepalive does not apply (the
    primary's heartbeats keep the connection in retransmission, not idle).
  - `net.ipv4.tcp_retries2` would (a dead connection gone in ~12 s at 5), but it is not on
    Kubernetes' safe sysctl list: it needs `--allowed-unsafe-sysctls` on every kubelet and the
    `mariadb` namespace out of the restricted Pod Security Standard - too much for this.
  - Turning semi-sync off would avoid it, but then a failover can lose the last transactions;
    semi-sync stays (user decision: a replica must always hold every acknowledged commit).
  - Reproduction, for re-testing a MariaDB upgrade: on the replica's node,
    `iptables -t raw -I PREROUTING 1 -s <pod ip> -j DROP` and the same with `-d` (the `raw`
    table, since k3s's kube-router accepts established traffic before the filter chains), write
    once on the primary, wait a few minutes, force-delete the replica pod, and watch new logins
    on the primary; remove both rules afterwards. Reported upstream as
    [MDEV-41349](https://jira.mariadb.org/browse/MDEV-41349) (2026-09-29); once a fixed release
    is out, the bump is the fix - re-test with the reproduction.
- **Restricted Pod Security** needs securityContexts on the pod (999, the image's mysql user -
  setting `podSecurityContext` replaces the operator's default, so it is repeated), the
  container, the agent, the init container and the exporter (UID 65534, the image only names
  its user). Once the init container's block is set at all, its `image` is required: the
  operator's own image, **bumped together with the operator**. Custom init or sidecar
  containers cannot pass restricted (mariadb-operator#1835) - none are used.
- **Backups** (`backups.yaml`), to `d3strukt0r-prod-mariadb-backups`:
  - `PhysicalBackup` `mariadb-daily` at 23:30 UTC from the replica, kept 30 days. Staged on
    the node's disk before the upload, so it sets an ephemeral-storage limit of its own.
  - `PointInTimeRecovery` `pitr`: the agent next to the primary archives **closed** binary logs
    every ten minutes (hardcoded). The operator rotates only by size, so at low traffic the
    active file would stay unarchived for weeks; the CronJob `mariadb-flush-binlogs` runs
    `FLUSH BINARY LOGS` on the primary every ten minutes, so a restore loses at most about
    twenty. The bucket expires archived logs after 35 days; the server itself deletes its local
    copies after 7 (`binlog_expire_logs_seconds`, default 0 = never, which would fill the
    volume).
  - `syncBinlog: 1`: every commit's binary log event goes to disk before the commit returns.
    With the server's default 0, the failover test's hard power-off (2026-09-28) lost the end of
    the binary log, and crash recovery rolled back a commit the client had already been told
    succeeded: the primary came back without that row while the replica kept it, so the two
    silently differed. The same power-off left the file with a zeroed tail; the archiver stops
    at such a file and every later one waits (runbook in the README).
  - `mariadb-replica-recovery`: a never-scheduled template the operator uses to rebuild a
    replica whose replication stays broken.
  - The CronJob `mariadb-backup-after-switchover` starts `mariadb-daily` on demand after every
    switch of the primary (see the next-but-one bullet). Every 15 minutes it compares the
    `PrimarySwitched` condition's time with the backup's `lastScheduleTime`; its Role may only
    read that MariaDB and patch that PhysicalBackup. Image `docker.io/alpine/kubectl`, pinned
    like the others - kubectl alone has no shell for the comparison.
  - Alerts: `MariaDBBinlogArchivingFailing` (`BinlogsArchived` not True for 30 minutes) and
    `MariaDBBackupFailed` (`mariadb-daily`'s last run did not complete, or none has for 26
    hours - also a failed after-switchover backup, which the CronJob does not retry), from
    kube-state-metrics' custom resource metrics (see "Monitoring"). A failed Job's
    `Complete=False` makes the `status="True"` series vanish, so an age check alone would miss
    exactly a failure.
- **A restore always goes into a new MariaDB** (`bootstrapFrom.pointInTimeRecoveryRef` with a
  `targetRecoveryTime`), never in place. Its success condition is `BinlogsReplayed=True`.
  `tls.caSecretKeyRef` must be set on every S3 reference - without it the restore panics in
  the operator yet reports Ready with only the base backup (mariadb-operator#1915). `s3-ca`
  holds the ISRG roots Hetzner's certificate chains to; they are public and live in git.
  Tested 2026-09-28: restored to the millisecond (runbook in the README).
- **`strictMode: true` stays**, although an operator bug makes it refuse a target inside the
  newest archived binary log: it fails loudly rather than restoring less than asked, and
  "as late as possible" still works by giving `LAST RECOVERABLE TIME` exactly (the end of
  the newest file is accepted). Without strict mode a restore that falls short reports
  success anyway.
- **Every switch of the primary leaves a hole in the archive until the next physical
  backup**: a failover, but also the planned switch of an update or a node drain. The old
  primary's last binary log (up to ten minutes) never reaches the bucket (only the primary
  archives), the new primary does not log the transactions it replicated, and the restore timeline
  follows the server the backup came from - so `LAST RECOVERABLE TIME` stays at the old
  primary's last upload. No data is lost, only restorability past that point. Seen in the
  failover test, 2026-09-28; closed within about 15 minutes by the after-switchover backup.
  `LAST RECOVERABLE TIME` only moves once something writes after that backup, so its age
  alone says nothing on a quiet database - which is why no alert watches it.
- **PITR works only in this replication topology**, not with Galera or a standalone instance.
- **The root password is kept twice**: in Secret `mariadb` (from OpenBao) and in the
  operator's own `internal-mariadb`, which it compares against to rotate a changed password.
  An empty password synced first (a failed 1Password lookup, 2026-09-28) left
  `internal-mariadb` empty while MariaDB initialised with the real one, and the operator
  looped on "Access denied" until `internal-mariadb` was patched to match - the fix is in the
  README. MariaDB itself refuses to initialise without a password, so no empty root was set.
- **`ServerSideDiff=true` on the Application**: the client-side diff kept the MariaDB
  OutOfSync after every successful sync, although a server-side apply changes nothing in it.

### PostgreSQL: CloudNativePG

The shared PostgreSQL for every app that only supports it - Zitadel first - the counterpart of
the shared MariaDB (user decision, 2026-09-29). Run by CloudNativePG
(`kubernetes/clusters/prod/cloudnative-pg.yaml`, both charts pinned, values in
`kubernetes/components/cloudnative-pg/`): a primary and a replica on two volumes, like MariaDB,
built small while prod-03 waits for its resize.

- **Why it needs none of MariaDB's workarounds:** PostgreSQL archives its WAL continuously and
  starts a new file at least every 5 minutes (`archive_timeout`), so no flush job; and its
  timelines chain the WAL of the old and the new primary across a failover, so no backup after
  a switch. Confirmed by the tests below: a restore reached through five timelines, including the
  last minutes before a hard power-off that the dead primary never archived - the synchronous
  replica had them, and PostgreSQL carries them into the new timeline's first WAL file.
- **The operator runs twice, on different nodes** (`replicaCount`, required anti-affinity,
  leader election built into the chart), for the same reason as mariadb-operator: it performs
  the failover and serves its own webhook (`failurePolicy: Fail`).
- **The Barman Cloud plugin** ships base backups and WAL to `d3strukt0r-prod-postgres-backups`.
  In-tree Barman support (`barmanObjectStore`) is deprecated and removed in CloudNativePG 1.31.
  The plugin must run in the operator's namespace, where the operator finds it by the label on
  its Service; its TLS comes from its own cert-manager Issuer. **It runs as one replica only** -
  a second never becomes ready, since the plugin serves only on its leader
  (plugin-barman-cloud#1105, unreleased). The archiving itself runs in a sidecar inside each
  PostgreSQL pod; the Deployment is needed to start new PostgreSQL pods and backups, so it
  tolerates an unreachable or not-ready node for 30 seconds instead of five minutes.
- **Archiving after a switch** (plugin-barman-cloud#828, "Expected empty archive"): reported to
  stop archiving after a switchover or failover up to 1.30.0; the operator-side fix is in 1.30.1,
  and the plugin honours it since 0.14. Not reproduced in four switches (2026-09-29) - each
  timeline's WAL reached the bucket. The only "failed" archive attempts are CloudNativePG's own
  "switchover in progress, refusing archiving", retried a second later. Should it ever appear,
  the workaround is the annotation `cnpg.io/skipEmptyWalArchiveCheck`.
- **The CRDs come with the operator chart**, marked `helm.sh/resource-policy: keep`, which Argo CD
  honours as `Delete=false` - so, unlike mariadb-operator's, they need no Application of their
  own. The chart renders 1.3 MB (`ServerSideApply=true` required). Bump both charts together.
- Both charts meet the restricted Pod Security Standard as they are; images are on `ghcr.io`.
  The operator's metrics are scraped through the chart's PodMonitor, and its Grafana dashboard
  comes as a ConfigMap. Kubernetes 1.37 is "tested, but not supported" for 1.30.x.

**The instance** (`kubernetes/clusters/prod/postgres.yaml`, `kubernetes/components/postgres/`,
namespace `postgres`): `Cluster` `postgres`, 2 instances on 10 GB each, one per node. Apps connect
to the Service **`postgres-rw`**, which follows the primary (`postgres-ro` reads from the
replica).

- **Settings that differ from the operator's defaults:**
  - the image pinned to one dated build (`18.6-<build>-minimal-trixie`), since the plain tag moves
    with every rebuild and the two instances could end up on different images;
  - `podAntiAffinityType: required` (the default only prefers separate nodes);
  - `primaryUpdateMethod: switchover` (the default restarts the primary in place);
  - synchronous replication with `dataDurability: preferred` - a commit waits for the replica
    while it is up, and the primary goes on alone while it is not, like MariaDB's semi-sync;
  - 256Mi/512Mi and `shared_buffers` 128MB, raised with real data.
  WAL stays on the data volume - a WAL volume per instance would make four. The initdb bootstrap
  creates CloudNativePG's default `app` database and owner, which nothing uses.
- **Apps get their users and databases declaratively**, as `DatabaseRole` (the docs' recommended
  way, over the Cluster's inline `managed.roles`) and `Database` objects in `postgres`. The role's
  password comes from a `kubernetes.io/basic-auth` Secret there, built by an ExternalSecret from
  OpenBao `secret/postgres-apps/<app>`; the app's own namespace gets the same value by a second
  ExternalSecret. No app reads CloudNativePG's generated Secrets. These objects sit in the
  app's own component (`components/<app>/database.yaml`, with `namespace: postgres`), not in
  the postgres component: everything about an app stays in one folder (user decision,
  2026-09-29). Steps in the README.
- **Backups** (`backups.yaml`), to `d3strukt0r-prod-postgres-backups`:
  - `ObjectStore` `postgres-backups`: WAL and base backups gzipped, `retentionPolicy: 30d`. Its
    `instanceSidecarConfiguration` sets `AWS_REQUEST_CHECKSUM_CALCULATION` and
    `AWS_RESPONSE_CHECKSUM_VALIDATION` to `when_required` (boto3's checksum headers, which
    S3-compatible stores reject - the plugin docs' workaround) and 64Mi/256Mi for the sidecar,
    whose uploads need more than the LimitRange's 128Mi. No region is set: Hetzner accepts
    requests signed for boto3's default (tested 2026-09-29).
  - `ScheduledBackup` `postgres-daily` at 23:00 UTC, from the replica (the cluster's default
    target), and once right away when created.
- **Alerts** (`rules.yaml`, from the instances' metrics through `pod-monitor.yaml` - the Cluster's
  `enablePodMonitor` is deprecated): `PostgresNoReadyPrimary` (critical, 5 minutes),
  `PostgresReplicaMissing`, `PostgresReplicaLagging`, `PostgresWALArchivingFailing` (a failure
  newer than the last success - a quiet database archives nothing for hours without that being a
  problem) and `PostgresBackupFailed` (last backup failed, or none for 26 hours).
- **`ServerSideDiff=true` on the Application** from the start, the MariaDB lesson.
- **A `DatabaseRole`'s password Secret must carry `cnpg.io/reload: "true"`.** CloudNativePG
  applies a role only when its spec or its Secret changes; the first attempt raced the operator
  adding the Secret to the instances' Role and was refused, and nothing retried it until the
  Secret changed. The label also makes password changes apply at once. The per-app
  ExternalSecret sets it through its template (README).
- **Failover tests (2026-09-29)**, with a pod writing a row every second through `postgres-rw`:

  | Test | Writes paused | Result |
  |---|---|---|
  | Planned switchover (`kubectl cnpg promote`) | ~10 s | no row lost |
  | Primary pod deleted | ~8 s | no row lost, pod rejoined as replica |
  | Primary's node powered off, with the active operator on it | ~96 s | no acknowledged row lost; 65 s until the node was NotReady, then 29 s for the failover |

  The identity column jumps by up to 32 after each switch: PostgreSQL logs sequence values in
  batches of 32, and a new primary continues after the logged batch - not lost rows. While the
  replica is gone, `synchronous_standby_names` is empty (`dataDurability: preferred`). The
  restore drill (runbook in the README) matched row count, highest id and checksum exactly and
  took 90 seconds.

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

The cluster was installed at k3s `v1.37.0+k3s1` (Kubernetes 1.37, containerd 2.3.4) on
Debian 13 with kernel 6.12, and upgrades itself from there (see "k3s upgrades itself"). That combination clears every requirement for **per-pod user namespaces**
(`hostUsers: false`), which are GA and locked since Kubernetes 1.36 and need Linux 6.3+,
containerd 2.0+ and an idmap-capable filesystem. This is the answer to wanting Podman's
rootless property: k3s's own `--rootless` mode is experimental and, per its docs,
multi-node rootless clusters are unsupported, so it is a single-node mode and not an
option here.

The installer script is pinned as well: the role fetches `install.sh` from the release's
own tag (`raw.githubusercontent.com/k3s-io/k3s/<k3s_version>/install.sh`), not from
`get.k3s.io`, which always serves the latest script, and checks it against
`k3s_install_sha256` - it runs as root. **Bumping `k3s_version` means bumping
`k3s_install_sha256` too**; the command to compute it is next to the value in
`roles/k3s/defaults/main.yml`.

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
