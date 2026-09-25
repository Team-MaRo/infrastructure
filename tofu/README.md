# tofu

Five independent root modules, each with its own state key in the Hetzner Object
Storage bucket `d3strukt0r-tfstate` (nbg1), via the S3 backend with native locking:

| Module | State key | What it manages |
|---|---|---|
| `hcloud` | `hcloud/terraform.tfstate` | the Hetzner Cloud project: the `prod` cluster's servers, network, firewall |
| `objectstorage` | `objectstorage/terraform.tfstate` | the Object Storage buckets - etcd snapshots and this state bucket - and their policies |
| `openbao` | `openbao/terraform.tfstate` | OpenBao's configuration: the `secret/` engine, Kubernetes auth, policies and roles |
| `infomaniak` | `infomaniak/terraform.tfstate` | registrar delegation and DNSSEC checks |
| `cloudflare` | `cloudflare/terraform.tfstate` | both Cloudflare accounts |

Keeping them separate means none can break another's plan, and each needs only its own
credentials.

## Credentials

Nothing needs exporting. Every module reads its own token from a gitignored
`terraform.tfvars` beside its `.tf` files, and the state backend reads the Object
Storage keys from an AWS profile.

| Where | Holds |
|---|---|
| `hcloud/terraform.tfvars` | `hcloud_token` |
| `objectstorage/terraform.tfvars` | `project_id` - the key itself is read from the profile below |
| `openbao/terraform.tfvars` | `openbao_token` - OpenBao's root token, for now |
| `infomaniak/terraform.tfvars` | `infomaniak_token` |
| `cloudflare/terraform.tfvars` | `cloudflare_token_personal`, `cloudflare_token_arepazo` |
| `~/.aws/credentials`, profile `[d3strukt0r-hetzner]` | the Object Storage access key and secret |

Each module ships a `terraform.tfvars.example` documenting what it needs and where to
get it. Copy it and fill it in:

```sh
cd hcloud && cp terraform.tfvars.example terraform.tfvars && $EDITOR terraform.tfvars
```

`*.tfvars` is gitignored; the `.example` files are not, because they hold no values.

The backend is the one thing tfvars cannot cover - backend blocks do not interpolate, so
no variable can reach them. Hence the profile:

```ini
# ~/.aws/credentials      chmod 600
[d3strukt0r-hetzner]
aws_access_key_id     = ...
aws_secret_access_key = ...
```

For the aws CLI, which is how buckets are inspected outside OpenTofu, the same profile in
`~/.aws/config` carries the endpoint and turns off the pager, so no command needs
`--endpoint-url` and output is printed rather than paged:

```ini
# ~/.aws/config
[profile d3strukt0r-hetzner]
endpoint_url = https://nbg1.your-objectstorage.com
cli_pager =
```

The backends set their endpoint explicitly, so this does not affect OpenTofu.

and `profile = "d3strukt0r-hetzner"` in each module's `backend "s3"` block. That is a
name, not a secret, so it is committed - which also means `tofu init` works without
anyone needing to know which environment variables to set. It is account-scoped because
profiles are global to `~/.aws`.

In the Hetzner console this key is labelled `admin (d3strukt0r-hetzner profile)`. It is
the only key the bucket policies let into tfstate - see objectstorage below - so it is
never handed to anything else. The label is not managed here; no provider has a resource
for Object Storage keys.

The `hcloud` CLI is unaffected; it keeps its own token in `~/.config/hcloud/cli.toml`.

## hcloud

OpenTofu config for the Hetzner Cloud project, named after the provider like the other two
modules. Today that is the `prod` cluster: three servers, their network, placement group
and firewall. Resources are named after the cluster, not after the Kubernetes distribution
running on it.

The infrastructure was created by hand first and adopted afterwards, so
`hcloud/imports.tf` holds the `import` blocks for every resource.

Needs `hcloud_token` in `hcloud/terraform.tfvars` - see Credentials above.

```sh
cd hcloud
tofu init
tofu plan
```

Two things the servers depend on that OpenTofu cannot see:

- `user_data` and `ssh_keys` are not returned by the Hetzner API, so both are
  under `ignore_changes` on `hcloud_server`. Changing either in the config has
  no effect on existing nodes - it only applies to newly created ones.
- The firewall is attached purely through the `cluster=prod` label selector. Adding
  a `hcloud_firewall_attachment` resource would conflict with it. Changing that label
  takes two applies - add the new label and a second `apply_to` first, remove the old ones
  after - or the servers can end up briefly unfirewalled.
- **The server name becomes the host's name**, and so the Kubernetes node name - but a
  rename here does not reach the host by itself: Hetzner's metadata, which cloud-init reads,
  keeps the name the server was created with. Ansible's `hostname` role applies it. Never
  rename a server while k3s runs on it.

The module used to be called `k3s`, with state key `k3s/terraform.tfstate`. `moved.tf`
maps the old resource addresses to the new ones; the state was copied to the new key with
`tofu init -migrate-state`.

The servers, the network and its subnet carry `prevent_destroy = true`. The
server types are cost-optimized and may not be available again once released,
so any plan that would destroy or replace one fails instead. Retiring a node
means removing that line first, deliberately.

## objectstorage

Buckets and bucket policies, through the `aminueza/minio` provider - the one Hetzner's
own docs use. The `aws` provider cannot even refresh a bucket here, because it reads
Accelerate, Website, Logging, Replication and Tagging settings that Hetzner does not
implement.

**Every Hetzner S3 key reaches every bucket in the project.** There are no per-bucket
keys. The only scoping is a bucket policy, so both policies here say "deny, unless it is
the admin key" (`Deny` with `NotPrincipal`), which also covers keys that do not exist yet.

| Bucket | Managed here | Policy |
|---|---|---|
| `d3strukt0r-tfstate` | bucket (created by hand, adopted through `imports.tf`) and policy | every action denied to every other key |
| `d3strukt0r-prod-etcd` | bucket, object lock, lifecycle, policy | other keys may upload, read and delete, but not bypass the lock or change the bucket's rules |

The etcd bucket locks every version for 7 days in GOVERNANCE mode, and a lifecycle rule
removes versions 7 days after they are replaced or deleted. GOVERNANCE, not COMPLIANCE, so
the admin key can still delete early - but a Hetzner key holds every permission, the
governance bypass included, so the policy denies that bypass to every other key. A leaked
cluster key can therefore add delete markers but cannot destroy a locked snapshot.

Needs only `project_id` in `objectstorage/terraform.tfvars`. The provider cannot read
AWS profiles, so `locals.tf` parses the `[d3strukt0r-hetzner]` section of
`~/.aws/credentials` itself (access key line first, then the secret). That is deliberate
beyond convenience: the tfstate policy allows exactly one key, and taking it from the file
the backends use means the policy cannot name a different one.

```sh
cd objectstorage
tofu init
tofu plan
```

### A wrong principal is a lockout

A policy that denies `s3:*` to everyone but `arn:aws:iam:::user/p<project_id>:<access_key>`
also denies it to the admin key if that string is off by one character - including
`PutBucketPolicy`, so OpenTofu cannot take the policy back. On `d3strukt0r-tfstate` that
stops every module's backend, and only Hetzner support can remove the policy. So any
change to how the principal is built is proven on a bucket that does not matter first:

1. **Stage A** - the etcd bucket with a policy denying `s3:*` to all but the admin key:
   the same statement as the tfstate policy, on an empty bucket.
2. **Prove it** with the admin key: an upload, a download, and a permanent delete of a
   locked version (see below). If any of them is denied, stop - the principal is wrong.
3. **Stage B** - narrow the etcd policy to what only the admin key may do, and add the
   tfstate policy.
4. `tofu plan` in every module, which proves the backends still reach their state.

The proof, with the aws CLI (independent of the provider):

```sh
echo test > /tmp/probe && aws --profile d3strukt0r-hetzner s3api put-object --bucket d3strukt0r-prod-etcd --key probe --body /tmp/probe
aws --profile d3strukt0r-hetzner s3api get-object --bucket d3strukt0r-prod-etcd --key probe /dev/stdout
aws --profile d3strukt0r-hetzner s3api list-object-versions --bucket d3strukt0r-prod-etcd --prefix probe
# refused - the version is locked:
aws --profile d3strukt0r-hetzner s3api delete-object --bucket d3strukt0r-prod-etcd --key probe --version-id=<id>
# succeeds - the admin key may bypass:
aws --profile d3strukt0r-hetzner s3api delete-object --bucket d3strukt0r-prod-etcd --key probe --version-id=<id> --bypass-governance-retention
```

`--version-id=<id>` with the `=`, because version IDs can start with `-`.

## openbao

OpenBao's own configuration - what the Helm values in `kubernetes/components/openbao/` do
not cover: the KV v2 secrets engine at `secret/`, Kubernetes auth, and the policy and role
External Secrets logs in with. Uses HashiCorp's `vault` provider; OpenBao keeps Vault's API.

OpenBao is only reachable inside the cluster, so open a port-forward first and keep it
running while you plan or apply:

```sh
kubectl --context d3strukt0r-prod-admin -n openbao port-forward svc/openbao 8200:8200
```

```sh
cd openbao
tofu init
tofu plan
```

`openbao/terraform.tfvars` holds `openbao_token`: the initial root token from 1Password item
`OpenBao | Prod | Recovery keys & root token`, until OpenBao has an admin login of its own.

**Secret values never go through this module.** Anything OpenTofu writes lands in its state,
so values are put in by hand:

```sh
BAO_ADDR=http://127.0.0.1:8200 \
BAO_TOKEN="$(op read --account my.1password.com 'op://Private/OpenBao | Prod | Recovery keys & root token/Initial Root Token')" \
  bao kv put secret/<path> key=value
```

The token is fetched per command, so it never sits in the shell's environment or history.

## infomaniak

Ten domains registered at Infomaniak, all delegated to Cloudflare. Infomaniak is
registrar only, so nothing about them is *managed* from here - `infomaniak/domains.tf`
declares the expected nameservers and DNSSEC state, and every `tofu plan` checks them
against the TLD registry and warns on drift.

Infomaniak's API can write nameservers but not read them, so detection and correction
are split: the check reads the registry over DNS, and a `terracurl_request` in
`infomaniak/domains_nameservers.tf` issues the `PUT` to fix it. That resource
only exists for domains that have drifted, so in a steady state it plans nothing.

Correcting drift needs an Infomaniak API token, created at
<https://manager.infomaniak.com/v3/ng/profile/user/token/list> with scopes
`domain:write` and `domain:read`. The write scope is what the `PUT` needs; the read
scope is there only so the token can be verified with a harmless `GET`.

The token is displayed once, and is deactivated after a year of inactivity - which
this one will reach, because it is used only when delegation drifts. A 401 later means
recreate it, not that something is broken.

It goes in `infomaniak/terraform.tfvars` as `infomaniak_token`. A plan needs it
only when delegation has actually drifted; the checks themselves read public DNS and
need no credential.

Check it before relying on it - 200 means token and scope are good, 401 a bad token,
403 a missing scope (set `INFOMANIAK_TOKEN` in your shell just for this check):

```sh
curl -sS -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer $INFOMANIAK_TOKEN" \
  https://api.infomaniak.com/2/domains/domains
```

The token is sent through write-only attributes, so it never reaches the state file.
It is only needed when a correction is actually pending; ordinary plans do not use it.

The DNSSEC probe shells out to `dig`, so it has to be on PATH.

## cloudflare

DNS for every zone lives in Cloudflare, across **two separate accounts** under two
separate logins.

| Account | Nameservers | Zones |
|---|---|---|
| personal | `brenda` / `wesley`.ns.cloudflare.com | the ten Infomaniak domains, plus `wundexpertinplus.com` (GoDaddy) |
| arepazo | `abdullah` / `fish`.ns.cloudflare.com | `arepazo.ch` |

Both providers are aliased and there is deliberately **no default provider**, so every
resource has to name its account. Getting it wrong is a configuration error rather than
a silent write to the wrong zone.

It manages the zones, every DNS record, and six security and TLS zone settings
(`ssl`, `always_use_https`, `min_tls_version`, `automatic_https_rewrites`, `tls_1_3`,
`security_level`). Page rules, WAF, Workers and the remaining ~54 zone settings stay in
the dashboard.

### Running it

Two API tokens, one per account, from <https://dash.cloudflare.com/profile/api-tokens>.
Each needs Zone/Zone **Read**, Zone/DNS **Edit**, Zone/Zone Settings **Edit**, and its
zone resources scoped to `All zones from an account` - not `All zones`, which would
reach both accounts and make the split meaningless.

Both go in `cloudflare/terraform.tfvars`.

```sh
cd cloudflare
tofu init
tofu plan
```

The provider's own `CLOUDFLARE_API_TOKEN` variable is intentionally unused - with two
accounts, one implicit token would authenticate against whichever it happens to belong
to.

### Regenerating from Cloudflare

`scripts/generate.sh` dumps HCL and import blocks for every zone in both accounts into
`generated/`, using [cf-terraforming](https://github.com/cloudflare/cf-terraforming)
(`brew install cf-terraforming`). Useful when adopting a new zone.

That output is raw material, not something to use directly: it names every resource
`terraform_managed_resource_<id>`, omits the `provider` attribute this module requires,
and Cloudflare themselves note that generated resources do not always pass
`terraform validate`. `generated/` is gitignored and is a subdirectory, so OpenTofu
never parses it.
