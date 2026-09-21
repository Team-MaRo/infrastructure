# cloud-init

```yaml
#include
https://raw.githubusercontent.com/Team-MaRo/infrastructure/refs/heads/master/cloud-init/k3s.yaml
```

# tofu

Three independent root modules, each with its own state key in the Hetzner Object
Storage bucket `d3strukt0r-tfstate` (nbg1), via the S3 backend with native locking:

| Module | State key | What it manages |
|---|---|---|
| `tofu/k3s` | `k3s/terraform.tfstate` | the Hetzner Cloud cluster |
| `tofu/infomaniak` | `infomaniak/terraform.tfstate` | registrar delegation and DNSSEC checks |
| `tofu/cloudflare` | `cloudflare/terraform.tfstate` | both Cloudflare accounts |

Keeping them separate means none can break another's plan, and each needs only its own
credentials.

## Credentials

Nothing needs exporting. Every module reads its own token from a gitignored
`terraform.tfvars` beside its `.tf` files, and the state backend reads the Object
Storage keys from an AWS profile.

| Where | Holds |
|---|---|
| `tofu/k3s/terraform.tfvars` | `hcloud_token` |
| `tofu/infomaniak/terraform.tfvars` | `infomaniak_token` |
| `tofu/cloudflare/terraform.tfvars` | `cloudflare_token_personal`, `cloudflare_token_arepazo` |
| `~/.aws/credentials`, profile `[d3strukt0r-hetzner]` | the Object Storage access key and secret |

Each module ships a `terraform.tfvars.example` documenting what it needs and where to
get it. Copy it and fill it in:

```sh
cd tofu/k3s && cp terraform.tfvars.example terraform.tfvars && $EDITOR terraform.tfvars
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

and `profile = "d3strukt0r-hetzner"` in each module's `backend "s3"` block. That is a
name, not a secret, so it is committed - which also means `tofu init` works without
anyone needing to know which environment variables to set. It is account-scoped because
profiles are global to `~/.aws`.

The `hcloud` CLI is unaffected; it keeps its own token in `~/.config/hcloud/cli.toml`.

# tofu/k3s

OpenTofu config for the Hetzner Cloud side of the cluster.

The infrastructure was created by hand first and adopted afterwards, so
`tofu/k3s/imports.tf` holds the `import` blocks for every resource.

Needs `hcloud_token` in `tofu/k3s/terraform.tfvars` - see Credentials above.

```sh
cd tofu/k3s
tofu init
tofu plan
```

Two things the servers depend on that OpenTofu cannot see:

- `user_data` and `ssh_keys` are not returned by the Hetzner API, so both are
  under `ignore_changes` on `hcloud_server`. Changing either in the config has
  no effect on existing nodes - it only applies to newly created ones.
- The firewall is attached purely through the `role=k3s` label selector. Adding
  a `hcloud_firewall_attachment` resource would conflict with it.

The servers, the network and its subnet carry `prevent_destroy = true`. The
server types are cost-optimized and may not be available again once released,
so any plan that would destroy or replace one fails instead. Retiring a node
means removing that line first, deliberately.

# tofu/infomaniak

Ten domains registered at Infomaniak, all delegated to Cloudflare. Infomaniak is
registrar only, so nothing about them is *managed* from here - `tofu/infomaniak/domains.tf`
declares the expected nameservers and DNSSEC state, and every `tofu plan` checks them
against the TLD registry and warns on drift.

Infomaniak's API can write nameservers but not read them, so detection and correction
are split: the check reads the registry over DNS, and a `terracurl_request` in
`tofu/infomaniak/domains_nameservers.tf` issues the `PUT` to fix it. That resource
only exists for domains that have drifted, so in a steady state it plans nothing.

Correcting drift needs an Infomaniak API token, created at
<https://manager.infomaniak.com/v3/ng/profile/user/token/list> with scopes
`domain:write` and `domain:read`. The write scope is what the `PUT` needs; the read
scope is there only so the token can be verified with a harmless `GET`.

The token is displayed once, and is deactivated after a year of inactivity - which
this one will reach, because it is used only when delegation drifts. A 401 later means
recreate it, not that something is broken.

It goes in `tofu/infomaniak/terraform.tfvars` as `infomaniak_token`. A plan needs it
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

# tofu/cloudflare

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

## Running it

Two API tokens, one per account, from <https://dash.cloudflare.com/profile/api-tokens>.
Each needs Zone/Zone **Read**, Zone/DNS **Edit**, Zone/Zone Settings **Edit**, and its
zone resources scoped to `All zones from an account` - not `All zones`, which would
reach both accounts and make the split meaningless.

Both go in `tofu/cloudflare/terraform.tfvars`.

```sh
cd tofu/cloudflare
tofu init
tofu plan
```

The provider's own `CLOUDFLARE_API_TOKEN` variable is intentionally unused - with two
accounts, one implicit token would authenticate against whichever it happens to belong
to.

## Regenerating from Cloudflare

`scripts/generate.sh` dumps HCL and import blocks for every zone in both accounts into
`generated/`, using [cf-terraforming](https://github.com/cloudflare/cf-terraforming)
(`brew install cf-terraforming`). Useful when adopting a new zone.

That output is raw material, not something to use directly: it names every resource
`terraform_managed_resource_<id>`, omits the `provider` attribute this module requires,
and Cloudflare themselves note that generated resources do not always pass
`terraform validate`. `generated/` is gitignored and is a subdirectory, so OpenTofu
never parses it.
