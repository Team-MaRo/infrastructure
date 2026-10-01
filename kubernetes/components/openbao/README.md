# OpenBao

Back to [`kubernetes/README.md`](../../README.md).

The secret store: one replica, Raft, a static seal from 1Password.
`kubernetes/clusters/prod/openbao.yaml` deploys the `openbao/openbao` Helm chart (pinned,
0.29.6 = OpenBao 2.6.3) with values from `kubernetes/components/openbao/values.yaml` - a
two-source Application, because OpenBao publishes only a chart. The pattern for Helm-only
upstreams: chart pinned in the cluster's Application, values in `components/`. Besides the
values, this directory holds the certificate for `openbao.d3strukt0r.dev`
(`certificates.yaml`), the snapshot agent's S3 key (`external-secrets.yaml`) and its alert
(`rules.yaml`).

**Its configuration is `tofu/openbao`** - the `secret/` engine, Kubernetes auth, the External
Secrets policy and role and the Zitadel login for admins; see
[`tofu/openbao/README.md`](../../../tofu/openbao/README.md). Secrets reach workloads through
[External Secrets](../external-secrets/README.md).

## How it runs

- **One replica, no HA preparation** (no `retry_join`), a deliberate simplicity choice like
  Argo CD's non-HA. External Secrets copies values into Kubernetes Secrets, so OpenBao being
  down while its pod moves delays changes but breaks no running workload.
- **Raft storage**, not the chart's default `file`, even with one node: Raft snapshots are
  the backup format. It lives on a Hetzner Volume.
- **`tls_disable = 1`** on the listener. Traffic to OpenBao is still encrypted between
  nodes, by the pod network's WireGuard tunnels (see "Pod traffic between nodes is
  encrypted" in [`ansible/roles/k3s/README.md`](../../../ansible/roles/k3s/README.md)); TLS on
  the listener would only add server authentication - clients could verify they talk to the
  real OpenBao.
- **The StatefulSet updates `OnDelete`** (the chart's default): a change to the pod template
  syncs, but the running pod keeps the old one until it is deleted -
  `kubectl --context d3strukt0r-prod-admin -n openbao delete pod openbao-0` - and it unseals
  itself again on start.
- The injector is off; secrets reach workloads through External Secrets.
- **The UI and API are public at `https://openbao.d3strukt0r.dev`** (user decision; the chart's
  Ingress, certificate from `components/openbao/certificates.yaml`, TLS ending at Traefik).
  Every call still needs a token, but the login endpoints face the internet - the accepted
  trade-off for convenience.

## The static seal

**Static seal** (`seal "static"`, OpenBao 2.4+): a 32-byte key, hex, in Secret
`openbao/openbao-seal` - not in git; written by `ansible/secrets.yml` from 1Password item
[`OpenBao | Prod | Seal key`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=n3blscroul4eancakippscoyhq&h=my.1password.com). OpenBao unseals itself on every start, and the cluster never
talks to 1Password.

**The seal key is the one thing that must never be lost.** Without it, the data on the
volume - and every snapshot - is unreadable. It lives in 1Password; rotating it means adding
the new key alongside the old one (`previous_key`), never replacing it.

## Initialisation, recovery keys and the root token

- **Initialised once, by hand** (`bao operator init` via `kubectl exec`), done 2026-09-24. The
  five recovery keys and the initial root token are in their own 1Password item,
  [`OpenBao | Prod | Recovery keys & root token`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=6ftev2p3fo3dc457whshzgn6jy&h=my.1password.com), apart from the seal key - which is written
  once and never edited, while the root token will be revoked and regenerated. There is no
  automation for this on purpose: it happens once per storage lifetime and its output must go
  straight into 1Password.
- **The root token stays the break-glass login** (user decision), kept only in 1Password - not
  in any tfvars day to day. The recovery keys (reusable Shamir shares, 3 of 5) can generate a new
  one (`bao operator generate-root`); with the static seal they unseal nothing.

## Backups

**Backed up every 6 hours** (00, 06, 12, 18 UTC) by the chart's `snapshotAgent`: a CronJob
(`openbao-snapshot`, image `ghcr.io/openbao/openbao-snapshot-agent`, bao CLI + s3cmd) that logs
in through Kubernetes auth (role and policy `snapshot` in `tofu/openbao/snapshot.tf`, `read` on
`sys/storage/raft/snapshot` only), saves a snapshot and uploads it to
`d3strukt0r-prod-openbao-snapshots` with its own key (ExternalSecret `openbao-snapshot-s3`),
then deletes snapshots older than 30 days (the bucket keeps a deleted one 7 more days).
OpenBao itself has no scheduled snapshots (openbao#795). `s3Uri` must stay the bucket root
with a trailing slash: the agent's pruning fails on sub-prefixes. s3cmd uses path-style
requests (the chart's `--host-bucket` is the bucket name); virtual-host style answers
`NoSuchBucket` at Hetzner. A fresh S3 key answered `404 NoSuchBucket` for its first minutes,
not `403` - the propagation delay again.

- **A snapshot is barrier-encrypted**: restoring needs the static seal key **and its key
  ID** (`1`); after a key rotation, older snapshots need the old key as `previous_key`. The
  recovery keys and root token are not needed to restore - the restore replaces the whole
  storage, and afterwards the original root token and recovery keys apply again.
- **Restore drill passed 2026-09-30**, offline in Docker (runbook below): a fresh OpenBao
  2.6.3 with the same static key, initialised, restored the first snapshot (60 KB) and then
  showed every auth method, all 14 secret paths and all policies to the original root token.
- The token of the run that took a snapshot comes back with a restore and cannot be revoked
  (openbao#522), so the role's tokens live 10 minutes.
- `OpenBaoSnapshotMissing` (`components/openbao/rules.yaml`) fires after 13 hours without a
  successful run - a Job started by hand from the CronJob counts too, since the CronJob owns
  it; a failed Job alerts earlier through the chart's `KubeJobFailed`.

## First install

1. Push; Argo CD creates the namespace and the StatefulSet. The pod waits for its Secret.
2. `cd ansible && ansible-playbook secrets.yml` writes `openbao-seal`; the pod starts.
3. Initialise it, **once, ever** for this storage:

   ```shell
   kubectl --context d3strukt0r-prod-admin -n openbao exec -ti openbao-0 -- bao operator init
   ```

   It prints recovery keys and the initial root token, **once**. Store all of them in
   1Password before closing the terminal. With an auto-unseal like the static seal there are
   no unseal keys; the recovery keys are for break-glass operations such as generating a new
   root token.
4. `kubectl ... -n openbao exec openbao-0 -- bao status` shows `Initialized true`,
   `Sealed false`. Deleting the pod proves the seal: it comes back unsealed by itself.

## Logging in

**OpenBao** is at `https://openbao.d3strukt0r.dev` - method OIDC, role empty (the default),
"Sign in with OIDC Provider". The CLI - `-no-print`, or `bao login` prints the new token, a
valid admin token, to the terminal:

```shell
BAO_ADDR=https://openbao.d3strukt0r.dev bao login -method=oidc -no-print
```

Break-glass: the root token (1Password [`OpenBao | Prod | Recovery keys & root token`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=6ftev2p3fo3dc457whshzgn6jy&h=my.1password.com)), in the
UI's Token method or through the port-forward below.

### Reaching it from this machine

```shell
kubectl --context d3strukt0r-prod-admin -n openbao port-forward svc/openbao 8200:8200
export BAO_ADDR=http://127.0.0.1:8200   # in the shell that runs bao
bao status
```

The UI is then at `http://127.0.0.1:8200/ui`.

## Setting up the snapshot key

Its S3 key, before the first run:

1. Hetzner console → Object Storage → S3 credentials, label `prod openbao snapshots`. Copy the
   secret key and store both in 1Password (the access key is not secret):

   ```shell
   op item create --account my.1password.com --vault Private --category 'API Credential' \
     --title 'Hetzner | S3 | prod openbao snapshots' \
     "username=<access key>" "credential=$(pbpaste)" >/dev/null
   ```

2. Into OpenBao, and the access key ID into `tofu/objectstorage/terraform.tfvars`
   (`openbao_snapshots_access_key_id`):

   ```shell
   jq -n --arg a "$(op item get 'Hetzner | S3 | prod openbao snapshots' --account my.1password.com --vault Private --fields username)" \
         --arg s "$(op item get 'Hetzner | S3 | prod openbao snapshots' --account my.1password.com --vault Private --fields credential --reveal)" \
     'if ($a|length)!=20 or ($s|length)==0 then error("access key must be 20 characters, secret non-empty - 1Password lookup failed?") else {"access-key":$a,"secret-key":$s} end' \
   | bao kv put secret/openbao-snapshot-s3 -
   ```

A snapshot by hand, and what is there:

```shell
kubectl --context d3strukt0r-prod-admin -n openbao create job --from=cronjob/openbao-snapshot openbao-snapshot-manual
kubectl --context d3strukt0r-prod-admin -n openbao logs job/openbao-snapshot-manual
aws --profile d3strukt0r-hetzner s3 ls s3://d3strukt0r-prod-openbao-snapshots/
kubectl --context d3strukt0r-prod-admin -n openbao delete job openbao-snapshot-manual
```

## Restoring

A restore needs the static seal key and its ID `1` (1Password [`OpenBao | Prod | Seal key`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=n3blscroul4eancakippscoyhq&h=my.1password.com)), a
snapshot, and an initialised, unsealed OpenBao to restore into - on a lost volume, a fresh one:
push, `ansible-playbook secrets.yml`, `bao operator init` as in "First install" (keep its output
only until the restore is done). Then, with the new root token:

```shell
aws --profile d3strukt0r-hetzner s3 cp s3://d3strukt0r-prod-openbao-snapshots/<newest>.snapshot /tmp/
kubectl --context d3strukt0r-prod-admin -n openbao cp /tmp/<newest>.snapshot openbao-0:/tmp/restore.snapshot
kubectl --context d3strukt0r-prod-admin -n openbao exec openbao-0 -- \
  env BAO_TOKEN=<new root token> bao operator raft snapshot restore /tmp/restore.snapshot
```

The restore replaces everything, the new root token and recovery keys included: from then on
the original root token and recovery keys in 1Password apply again. Check with
`bao kv list secret/` and `bao auth list`, logged in with the original root token.

**The drill** (done 2026-09-30) runs the same restore in Docker on this machine, without
touching the cluster: download a snapshot, write the seal key to a file with `printf '%s'`,
start `quay.io/openbao/openbao:2.6.3` with a config of `storage "raft"` and the same
`seal "static"` block (`current_key_id = "1"`), `bao operator init`, restore with the new root
token, then list secrets with the original one. Delete the container and the key file
afterwards.
