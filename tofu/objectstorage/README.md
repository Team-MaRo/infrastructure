# objectstorage

Buckets and bucket policies, through the `aminueza/minio` provider - the one Hetzner's
own docs use.

| Bucket | Managed here | Policy |
|---|---|---|
| `d3strukt0r-tfstate` | bucket (created by hand, adopted through `imports.tf`) and policy | every action denied to every other key |
| `d3strukt0r-prod-etcd` | bucket, object lock, lifecycle, policy | only the snapshot key and the admin key; the snapshot key may upload, read and delete, but not bypass the lock or change the bucket's rules |
| `d3strukt0r-prod-loki` | bucket, lifecycle, policy | only Loki's key and the admin key |
| `d3strukt0r-prod-mariadb-backups` | bucket, lifecycle (expires after 35 days), policy | only the MariaDB backup key and the admin key |
| `d3strukt0r-prod-postgres-backups` | bucket, lifecycle, policy | only the PostgreSQL backup key and the admin key |
| `d3strukt0r-prod-openbao-snapshots` | bucket, object lock, lifecycle, policy | only OpenBao's snapshot key and the admin key; like the etcd bucket, the key cannot bypass the lock or change the bucket's rules |
| `d3strukt0r-prod-wedding-manuele-robine` | bucket, versioning, lifecycle, policy | only the app's key and the admin key |

Shared rules, credentials and the state backend are in [`../README.md`](../README.md).

## Every key reaches every bucket

Hetzner S3 keys are valid for every bucket in the project, with every permission. The only
way to scope one is a bucket policy, so `tofu/objectstorage` writes every policy as
`Deny` + `NotPrincipal` naming the admin key, `arn:aws:iam:::user/p<project_id>:<access_key>`
(`local.admin_principal`), plus at most the one cluster key that bucket is for
(`local.etcd_principal`, `local.loki_principal`, `local.mariadb_backups_principal`,
`local.postgres_backups_principal`, `local.openbao_snapshots_principal`,
`local.wedding_manuele_robine_principal`). That covers keys that do not exist yet, so
**each key the cluster holds reaches only its own bucket**. The cluster keys' access key IDs
(not their secrets) are `etcd_access_key_id`, `loki_access_key_id`,
`mariadb_backups_access_key_id` and `postgres_backups_access_key_id` in the module's tfvars; a
mistyped one shuts only that key out of its bucket. A `NotPrincipal` naming two keys was proven
on the Loki bucket first (2026-09-28): the admin key kept full access, and the etcd key got
`AccessDenied` there.

**A new bucket's policy takes about 15 minutes to hold everywhere.** Hetzner's gateways learn a
new bucket and its policy one by one; until all have, each request lands on one that may not
know them yet. Measured on 2026-10-03 with a key the policy shuts out: in the first ~13 minutes
after the apply it could read and write in the bucket on some requests, others answered 404 for
the bucket itself; from then on, an hour of tests every 5 minutes denied it every time. A bucket
that existed before showed no such gap. So **nothing goes into a new bucket within 15 minutes of
its apply**, and a policy test run right after the apply proves nothing - repeat it after that.

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
- **A wrong principal is a lockout** - see [below](#a-wrong-principal-is-a-lockout) for the
  staged procedure any change to how the principal is built goes through.
- **`d3strukt0r-prod-etcd`: object lock, GOVERNANCE, 7 days, plus a lifecycle rule** that
  removes versions 7 days after they become noncurrent, and orphaned delete markers after
  that. GOVERNANCE rather than COMPLIANCE so the admin key can still delete early; since a
  Hetzner key holds the governance bypass like any permission, the policy denies
  `s3:BypassGovernanceRetention` and all lock, lifecycle and policy changes to every key but
  the admin's - the snapshot key included. A first statement shuts out every key but those
  two entirely. Object lock was only possible at creation and made versioning permanent. A
  leaked cluster key can therefore add delete markers but cannot destroy a locked snapshot.
- **`d3strukt0r-prod-loki`: unversioned, no lock**, lifecycle rule only for incomplete
  uploads; Loki's compactor deletes old logs. Only the admin key and Loki's key (console label
  `prod loki`, 1Password [`Hetzner | S3 | prod loki`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=obsu3brgoh5l3yxawnbbvq4wi4&h=my.1password.com), OpenBao `secret/loki-s3`) reach it.
- **`d3strukt0r-prod-mariadb-backups`: unversioned, objects expire after 35 days.** It holds the
  MariaDB operator's physical backups and archived binary logs. The operator deletes backups
  past their 30-day retention itself but never the binary logs, so the lifecycle rule expires
  everything after 35 days - long enough that every kept backup still has its binary logs.
  Only the admin key and the backup key (console label `prod mariadb backups`, 1Password
  [`Hetzner | S3 | prod mariadb backups`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=qttplkz3cky6llsjgw7zhww5bu&h=my.1password.com), OpenBao `secret/mariadb-backups-s3`) reach it.
- **`d3strukt0r-prod-postgres-backups`: unversioned, lifecycle rule only for incomplete
  uploads.** It holds CloudNativePG's base backups and archived WAL (Barman Cloud plugin) of
  the shared PostgreSQL. The plugin deletes base backups past their retention together with the
  WAL they no longer need, so nothing else expires here. Only the admin key and the backup key
  (console label `prod postgres backups`, 1Password [`Hetzner | S3 | prod postgres backups`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=xwoxypf5ibmqtcfl7gwjuxx4re&h=my.1password.com),
  OpenBao `secret/postgres-backups-s3`) reach it.
- **`d3strukt0r-prod-openbao-snapshots`: object lock, GOVERNANCE, 7 days, like the etcd
  bucket** (same lifecycle rule and two-statement policy, `prevent_destroy`). OpenBao's
  snapshot agent deletes snapshots past 30 days itself - only delete markers here, the versions
  stay locked and are removed 7 days later - so a stolen snapshot key cannot destroy a backup.
  Only the admin key and the snapshot key (console label `prod openbao snapshots`, 1Password
  [`Hetzner | S3 | prod openbao snapshots`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=7wzbdy35bhtgqn6ahbqyoaiuum&h=my.1password.com), OpenBao `secret/openbao-snapshot-s3`) reach it.
- **`d3strukt0r-prod-wedding-manuele-robine`: versioned, no lock.** The media of
  wedding-manuele-robine (uploads through the API's flysystem, private - the API serves them).
  The only copy of the guests' uploads, so a deleted or overwritten file stays recoverable for 30
  days (`noncurrent_version_expiration`), then the orphaned delete markers go;
  `prevent_destroy`. Only the admin key and the app's key (console label
  `prod wedding-manuele-robine`, 1Password
  [`Hetzner | S3 | prod wedding-manuele-robine`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=ytvt5yczmlghqtf7ki6orppn6e&h=my.1password.com), OpenBao
  `secret/wedding-manuele-robine-s3`) reach it. The app signs with region `eu-west-1`
  (hard-coded in its code), which Hetzner accepts.
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

## Not managed here

The Object Storage keys - no provider has a resource for them, so they and their console
labels live only in the Hetzner console.

## Running it

Needs `project_id`, `etcd_access_key_id`, `loki_access_key_id`,
`mariadb_backups_access_key_id`, `postgres_backups_access_key_id`,
`openbao_snapshots_access_key_id` and `wedding_manuele_robine_access_key_id` in
`objectstorage/terraform.tfvars` - the key IDs are the username fields of the keys'
1Password items, never the secrets. The provider cannot read
AWS profiles, so `locals.tf` parses the `[d3strukt0r-hetzner]` section of
`~/.aws/credentials` itself (access key line first, then the secret). That is deliberate
beyond convenience: the tfstate policy allows exactly one key, and taking it from the file
the backends use means the policy cannot name a different one.

```sh
cd tofu/objectstorage
tofu init
tofu plan
```

## A wrong principal is a lockout

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
