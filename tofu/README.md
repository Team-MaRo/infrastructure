# tofu

Six independent root modules, each with its own state key in the Hetzner Object
Storage bucket `d3strukt0r-tfstate` (nbg1), via the S3 backend with native locking:

| Module | State key | What it manages |
|---|---|---|
| [`hcloud`](hcloud/README.md) | `hcloud/terraform.tfstate` | the Hetzner Cloud project: the `prod` cluster's servers, network, firewall |
| [`objectstorage`](objectstorage/README.md) | `objectstorage/terraform.tfstate` | the Object Storage buckets - etcd snapshots and this state bucket - and their policies |
| [`openbao`](openbao/README.md) | `openbao/terraform.tfstate` | OpenBao's configuration: the `secret/` engine, Kubernetes auth, policies and roles |
| [`zitadel`](zitadel/README.md) | `zitadel/terraform.tfstate` | what is inside Zitadel: instance policies, organisation domains, projects and apps |
| [`infomaniak`](infomaniak/README.md) | `infomaniak/terraform.tfstate` | registrar delegation and DNSSEC checks |
| [`cloudflare`](cloudflare/README.md) | `cloudflare/terraform.tfstate` | both Cloudflare accounts |

Keeping them separate means none can break another's plan, and each needs only its own
credentials. What each module manages, why it is built the way it is and how to run it is in
its own README; this one covers only what they share.

## Rules

- **Never run `tofu destroy`.**
- **Never `tofu apply` a plan that is not clean.** A clean plan is
  `0 to add, 0 to change, 0 to destroy`. If a plan shows `forces replacement`,
  `must be replaced` or `will be destroyed`, that is a bug in the config - fix the
  config and re-plan.
- Adoption of existing resources goes through `import` blocks in the module's `imports.tf`,
  not `tofu import` CLI calls, so it stays reviewable in git.

## Running a module

```sh
cd hcloud         # or objectstorage, openbao, zitadel, infomaniak, cloudflare
tofu fmt            # must produce no output
tofu validate
tofu init
tofu plan
```

`tofu init -backend=false` initialises providers only and needs no credentials -
useful for `fmt`/`validate` when the S3 keys are not to hand.

## Credentials

Nothing needs exporting, and no value belongs in a `.tf` file. Every module reads its own
token from a gitignored `terraform.tfvars` beside its `.tf` files, and the state backend reads
the Object Storage keys from an AWS profile.

| Where | Holds |
|---|---|
| `hcloud/terraform.tfvars` | `hcloud_token`, and `admin_ips` - not a secret, but a home address, and this repo is public |
| `objectstorage/terraform.tfvars` | `project_id` - the key itself is read from the profile below |
| `openbao/terraform.tfvars` | `openbao_token` - OpenBao's root token, only on a cluster rebuilt from scratch; day to day the provider reads `~/.vault-token` (see [`openbao`](openbao/README.md)) |
| `zitadel/terraform.tfvars` | `zitadel_jwt_profile` |
| `infomaniak/terraform.tfvars` | `infomaniak_token` |
| `cloudflare/terraform.tfvars` | `cloudflare_token_personal`, `cloudflare_token_arepazo` |
| `~/.aws/credentials`, profile `[d3strukt0r-hetzner]` | the Object Storage access key and secret |

Each module ships a `terraform.tfvars.example` documenting what it needs and where to
get it. Copy it and fill it in:

```sh
cd hcloud && cp terraform.tfvars.example terraform.tfvars && $EDITOR terraform.tfvars
```

`*.tfvars` is gitignored; the `.example` files are not, because they hold no values.

`tofu/hcloud` declares `variable "hcloud_token"` and passes it to the provider explicitly
rather than relying on the provider's own `HCLOUD_TOKEN` lookup, so all modules
get their credentials the same way.

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
profiles are global to `~/.aws`, so a bare `hetzner` would collide with any second Hetzner
account. If a plan cannot reach the state bucket, check that file first: there is no
environment variable to fall back on by design.

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

In the Hetzner console this key is labelled `admin (d3strukt0r-hetzner profile)`. It is
the only key the bucket policies let into tfstate - see [`objectstorage`](objectstorage/README.md)
- so it is never handed to anything else. The label is not managed here; no provider has a
resource for Object Storage keys.

The `hcloud` CLI is unaffected; it keeps its own token in `~/.config/hcloud/cli.toml`.

## State backend

Hetzner Object Storage bucket `d3strukt0r-tfstate` in **nbg1** (not fsn1), via the S3
backend with `use_lockfile = true` and `profile = "d3strukt0r-hetzner"`. Each module's key is
`<module>/terraform.tfstate`. Changing anything in a backend block means
`tofu init -reconfigure` on the next run.

Hetzner is S3-compatible but not AWS, so all the `skip_*` flags in each module's
`versions.tf` are required; `skip_s3_checksum = true` specifically is what makes lock-object
writes succeed.

Versioning and Object Lock are both enabled on the bucket, but
`get-object-lock-configuration` returns no `Rule`, so there is **no default retention**
- and the S3 backend never sends per-object retention headers. Every apply adds a version
and every plan leaves its lock file behind as a version plus a delete marker; a lifecycle
rule (`objectstorage/tfstate.tf`) removes versions 90 days after they are replaced,
then the orphaned markers. So an old state can be restored for 90 days, not longer. Old
versions can also be pruned early with `aws s3api delete-object --version-id`; nothing is
locked.

```sh
aws --profile d3strukt0r-hetzner s3api list-object-versions --bucket d3strukt0r-tfstate --prefix hcloud/
```

That relies on `endpoint_url` (and `cli_pager =`) in the profile's `~/.aws/config`
section - see Credentials above. Without it, add
`--endpoint-url https://nbg1.your-objectstorage.com`.

Freshly generated Hetzner S3 credentials propagate across their gateways over several
minutes. During that window `tofu init` fails with
`operation error S3: HeadObject ... StatusCode: 403` while the key already works for
listing buckets, and other clients may answer `404 NoSuchBucket` instead. That is not a
permissions problem - wait and retry before touching ACLs, bucket policies or
`use_path_style`. When a 403 makes the aws CLI crash rather than report it, `--debug` shows
the actual answer. s3cmd needs path-style addressing; with virtual-host style Hetzner answers
`NoSuchBucket`.
