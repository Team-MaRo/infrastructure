# openbao

OpenBao's own configuration - what the Helm values in `kubernetes/components/openbao/` do
not cover: the KV v2 secrets engine at `secret/`, Kubernetes auth, and the policy and role
External Secrets logs in with. Uses HashiCorp's `vault` provider; OpenBao keeps Vault's API.
How OpenBao itself runs on the cluster is in
[`kubernetes/components/openbao/README.md`](../../kubernetes/components/openbao/README.md).

Shared rules, credentials and the state backend are in [`../README.md`](../README.md).

## What the module configures

- **Its configuration is `tofu/openbao`**: the KV v2 engine `secret/`, Kubernetes auth, and
  the `external-secrets` policy and role, and the OIDC login (below). It reaches OpenBao at its
  public address and authenticates with either of two tokens: day to day an admin's own, from
  `bao login -method=oidc` (`~/.vault-token`, read by the provider when `openbao_token` is
  unset); on a cluster rebuilt from scratch, where no OIDC login exists yet, the root token in
  the gitignored tfvars - with `-target=vault_mount.secret` first, since the OIDC login reads its
  client secret from the mount the module creates. Secret **values** never go
  through OpenTofu - they would land in state - but in with `bao kv put`. The ESO policy reads
  all of `secret/`, so every namespace referencing the ClusterSecretStore reaches every value:
  fine for a single-admin cluster, to be narrowed per namespace once others deploy. Audit
  devices, if wanted, belong in the server config, not the API.
- **Admins log in through Zitadel** (`tofu/openbao/oidc.tf`, app in
  `tofu/zitadel/apps_openbao.tf`): auth method `oidc` at `auth/oidc`, role `default` (the
  default, so the UI's role field stays empty), `groups_claim = "groups"`. The claim value
  `infra-admin` matches the alias of the external identity group `infra-admin`, which carries
  the policy `admin` (everything, `sudo` included) - rights come only from Zitadel's role. One
  app serves the UI's callback and the CLI's (`bao login -method=oidc`, `localhost:8250`),
  which is why that app runs in Zitadel's development mode: only it lets a web app register an
  http localhost address.
- **OpenBao refuses OIDC without a client secret** (`both 'oidc_client_id' and
  'oidc_client_secret' must be set`), although it uses PKCE too. The secret lives in 1Password
  [`Zitadel | Prod | OpenBao`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=66smfmdqmznzjmlv4pv4znccjq&h=my.1password.com) and OpenBao `secret/openbao-oidc`; `tofu/openbao` reads it with an
  **ephemeral** `vault_kv_secret_v2` and hands it over as the write-only
  `oidc_client_secret_wo`, so it is in no state and no tfvars. Write-only values are only sent
  when `oidc_client_secret_wo_version` changes - bump it after a rotation.

## Running it

**Day to day**, log in through Zitadel first; the provider takes that token from
`~/.vault-token` and reaches OpenBao at its public address:

```sh
bao login -method=oidc -no-print     # with BAO_ADDR=https://openbao.d3strukt0r.dev set
cd tofu/openbao
tofu init
tofu plan
```

**On a cluster rebuilt from scratch** no OIDC login exists yet, so the root token from 1Password
item [`OpenBao | Prod | Recovery keys & root token`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=6ftev2p3fo3dc457whshzgn6jy&h=my.1password.com) goes into `openbao/terraform.tfvars` as
`openbao_token` (see `terraform.tfvars.example`), and - while `openbao.d3strukt0r.dev` does not
answer yet - `openbao_address = "http://127.0.0.1:8200"` with a port-forward:

```sh
kubectl --context d3strukt0r-prod-admin -n openbao port-forward svc/openbao 8200:8200
```

The order there: the OIDC login reads its client secret from `secret/`, the mount this module
creates, so the mount comes first on its own; then the secret values go in (below, `openbao-oidc`
with a freshly regenerated Zitadel secret among them); then the full apply. Remove
`openbao_token` from the tfvars once the Zitadel login works.

```sh
tofu apply -target=vault_mount.secret
tofu apply
```

The root token stays valid in 1Password as the break-glass login. Were it ever lost, the recovery
keys in the same item (reusable, 3 of 5) create a new one: `bao operator generate-root`.

## Putting secret values in

**Secret values never go through this module.** Anything OpenTofu writes lands in its state,
so values are put in by hand:

```sh
BAO_ADDR=http://127.0.0.1:8200 \
BAO_TOKEN="$(op item get 'OpenBao | Prod | Recovery keys & root token' --account my.1password.com --vault Private --fields credential --reveal)" \
  bao kv put secret/<path> key=value
```

The token is fetched per command, so it never sits in the shell's environment or history.
`op item get` rather than `op read`: `op://` references reject the `|` in the item title.
Logged in through Zitadel (`bao login -method=oidc -no-print`), the same commands work without
`BAO_TOKEN` and against `BAO_ADDR=https://openbao.d3strukt0r.dev` instead of the port-forward.

Two traps with `key="$(op item get ...)"`, both met while setting up MariaDB:

- **A failed lookup still writes.** If the 1Password item or field is not found, `op` prints
  an error and the substitution is empty - and `bao` stores an empty value without
  complaint.
- **A value starting with `@` is read as a file name** (`key=@file`), so a generated password
  that starts with `@` fails - and the error message prints it.

For generated passwords, hand the values over as JSON on stdin instead, with a check that
none is empty:

```sh
jq -n --arg a "$(op item get '<item>' --account my.1password.com --vault Private --fields <field-a> --reveal)" \
      --arg b "$(op item get '<item>' --account my.1password.com --vault Private --fields <field-b> --reveal)" \
  'if ($a|length)==0 or ($b|length)==0 then error("empty value - 1Password lookup failed") else {"key-a":$a,"key-b":$b} end' \
| BAO_ADDR=http://127.0.0.1:8200 \
  BAO_TOKEN="$(op item get 'OpenBao | Prod | Recovery keys & root token' --account my.1password.com --vault Private --fields credential --reveal)" \
  bao kv put secret/<path> -
```

API tokens and access keys have a fixed format without `@`, so the plain `key=value` form
stays fine for them.

## The OIDC login's client secret

**The OIDC login** (`oidc.tf`) needs `secret/openbao-oidc` before its first apply: the client
secret of the Zitadel app `OpenBao` (`tofu/zitadel/apps_openbao.tf`), regenerated in the console
right after that app was created, since the first one is in `tofu/zitadel`'s state.

```sh
op item create --account my.1password.com --vault Private --category password \
  --title 'Zitadel | Prod | OpenBao' "password=$(pbpaste)" >/dev/null
jq -n --arg c "$(op item get 'Zitadel | Prod | OpenBao' --account my.1password.com --vault Private --fields password --reveal)" \
  'if ($c|length)==0 then error("empty value - 1Password lookup failed") else {"client-secret":$c} end' \
| BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN="$(op item get 'OpenBao | Prod | Recovery keys & root token' --account my.1password.com --vault Private --fields credential --reveal)" \
  bao kv put secret/openbao-oidc -
```

The module reads it ephemerally and writes it write-only - it never enters the state. After a
rotation, raise `oidc_client_secret_wo_version`, or the new value is never sent.
