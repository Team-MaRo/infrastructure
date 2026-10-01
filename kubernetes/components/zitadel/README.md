# Zitadel

The identity provider (`kubernetes/clusters/prod/zitadel.yaml`, chart `zitadel` pinned, values
and extra objects in this directory, namespace `zitadel`): the one login (OIDC/SAML) for the
cluster's UIs, kubectl and later the apps, at **`https://auth.d3strukt0r.dev`** (its own
Cloudflare record, a CNAME to `prod`). How the cluster's manifests fit together is in
[`kubernetes/README.md`](../../README.md); what is configured inside Zitadel - policies, token
lifetimes, domains, the project `Infrastructure` with its role and apps, and the `groups`
claim's action - is `tofu/zitadel`, see [`tofu/zitadel/README.md`](../../../tofu/zitadel/README.md).

Chosen over Keycloak for its footprint (user decision, 2026-09-29): a Go server that stores
everything in the shared PostgreSQL (database `zitadel`). It runs as one replica of Zitadel and
one of its separate v4 login page (Next.js, path `/ui/v2/login`) until prod-03's resize; while it
is down, the UIs stay reachable by port-forward. Measured idle after the first start
(2026-09-30): Zitadel about 120-170Mi, the login page about 120Mi.

## Design

- **Its database** is `database.yaml` (namespace `postgres`, see
  [`../postgres/README.md`](../postgres/README.md));
  the init job runs only `zitadel init zitadel` (`initJob.command`), the schema, so Zitadel never
  needs a database admin. It connects with `sslmode=verify-full` against CloudNativePG's CA,
  which `postgres-ca.yaml` copies into `zitadel` through External Secrets' Kubernetes provider: a
  ServiceAccount whose Role in `postgres` may `get` only Secret `postgres-ca`, and an ExternalSecret
  taking only `ca.crt` - never the CA's key. The pattern for every Postgres app.
- **The masterkey** (OpenBao `secret/zitadel`, 1Password [`Zitadel | Prod | Masterkey`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=cpgucnzccroyrl5zwzdrmcbfau&h=my.1password.com)) encrypts
  secrets in the database; losing it makes them unreadable and it cannot be changed after the
  first start - like OpenBao's seal key. Ours, not the chart's, which would generate one in a Helm
  hook.
- **The chart's Helm hooks are replaced by sync waves.** Its ServiceAccounts, ConfigMaps, Role and
  the init and setup jobs are `pre-install`/`pre-upgrade` hooks, which Argo CD runs as PreSync -
  before this directory's Secrets and certificates exist, so the first sync hung on an init job
  waiting for `postgres-ca`. `values.yaml` nulls the hook annotations and sets Argo CD's order
  instead: this directory's objects in wave -1 (`commonAnnotations` in its kustomization), the
  chart's config, ServiceAccounts and RBAC in 0, the init job in 1 and the setup job in 2 - both
  Sync hooks, recreated on every sync and idempotent - then Zitadel and the login page in 3. Each
  wave waits for the previous one to be healthy. The chart's post-delete cleanup job is off. The
  setup job creates the first instance once - the organisation
  `D3strukt0r`, the human admin `auth-admin@d3strukt0r.dev` (an e-mail as the name, since Zitadel
  appends a domain to one without `@`; its password from `secret/zitadel` stays the login, kept in
  1Password - no forced change, since whoever can read the copies in the cluster is cluster admin
  anyway; the address counts as verified, Zitadel's default - there is no mail server yet) and
  the machine admin `iam-admin`, whose key its kubectl sidecar writes to Secret `iam-admin` (no
  personal access token, `Pat: null`).
- **The login page's key pair is ours** (`certificates.yaml`, a self-signed cert-manager
  Certificate valid ten years): the login page signs its API calls with the key, and Zitadel
  verifies them with the certificate as the system user `login-client`. The chart would generate
  the pair with Helm's `lookup`, which Argo CD's rendering never has - a new pair on every render,
  so the app would be permanently OutOfSync and the pair would keep rotating.
- **`ExternalPort: 443` is required**: without it Zitadel and the login page build every URL with
  the container port (`:8080`). TLS ends at Traefik; the chart's two Ingresses (`/` to Zitadel over
  h2c, `/ui/v2/login` to the login page) share the certificate `auth-tls`, issued by a separate
  Certificate rather than Ingress annotations, which would create two for one Secret. They are the
  cluster's first Ingresses; Traefik is the default IngressClass, so none is named.
- **Restricted Pod Security** through the chart-wide `podSecurityContext`/`securityContext`, which
  every container takes - Zitadel, the login page and its `wait4x` init container, the jobs and the
  setup job's `alpine/k8s` sidecars; the chart's defaults lack seccomp, no-escalation and dropped
  capabilities.
- **The `groups` claim comes from a webhook**, `groups-webhook.yaml`, which Zitadel calls
  through an Actions v2 target that `tofu/zitadel` creates - why and how is in
  [`tofu/zitadel/README.md`](../../../tofu/zitadel/README.md).
- **Zitadel refuses to call private addresses** (`HTTPClient.DenyList`, for actions, identity
  providers and SMTP alike), which includes every Service. `ZITADEL_HTTPCLIENT_DENYLIST` in the
  values replaces that list with the default entries and `10.0.0.0/8` split into 24 ranges that
  leave out exactly `10.43.255.250`, the webhook Service's fixed `clusterIP` - the Kubernetes
  API and every other Service stay unreachable from Zitadel. The target is checked against the
  list when it is created, so the webhook is deployed before `tofu/zitadel` creates the target.
- The license is AGPL-3.0; running it unmodified puts no obligation on the apps that log in
  through it.

## Before the first sync

Its secrets go into OpenBao (port-forward and `BAO_TOKEN` as in [OpenBao](../openbao/README.md));
the database password is set up as in "Adding an app" in
[`../postgres/README.md`](../postgres/README.md):

1. Two 1Password items: the first admin's login [`Zitadel | Prod | Admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=hwednr7k3ycfvmdgtbcylq5nwi&h=my.1password.com) (generated password),
   and [`Zitadel | Prod | Masterkey`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=cpgucnzccroyrl5zwzdrmcbfau&h=my.1password.com), **exactly 32 characters** (letters and digits), written
   once and never edited:

   ```shell
   op item create --account my.1password.com --vault Private --category login --title 'Zitadel | Prod | Admin' \
     --url https://auth.d3strukt0r.dev/ui/console --generate-password='letters,digits,symbols,20' \
     username=auth-admin@d3strukt0r.dev >/dev/null
   op item create --account my.1password.com --vault Private --category password --title 'Zitadel | Prod | Masterkey' \
     "password=$(LC_ALL=C tr -dc 'A-Za-z0-9' </dev/urandom | head -c 32)" >/dev/null
   ```
2. As JSON on stdin, with the empty check:

   ```shell
   jq -n --arg m "$(op item get 'Zitadel | Prod | Masterkey' --account my.1password.com --vault Private --fields password --reveal)" \
         --arg a "$(op item get 'Zitadel | Prod | Admin' --account my.1password.com --vault Private --fields password --reveal)" \
     'if ($m|length)!=32 or ($a|length)==0 then error("masterkey must be 32 characters, admin password non-empty") else {"masterkey":$m,"admin-password":$a} end' \
   | BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN="$(op item get 'OpenBao | Prod | Recovery keys & root token' --account my.1password.com --vault Private --fields credential --reveal)" \
     bao kv put secret/zitadel -
   ```

**The masterkey must never be lost or changed**: it encrypts the secrets in Zitadel's database.

## After the first start

The first start creates the organisation `D3strukt0r` with the admin `auth-admin@d3strukt0r.dev`
(the password from [`Zitadel | Prod | Admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=hwednr7k3ycfvmdgtbcylq5nwi&h=my.1password.com), kept as the login) at
`https://auth.d3strukt0r.dev/ui/console`, and the machine user `iam-admin`, whose key the setup
job stores as Secret `iam-admin` in `zitadel` - copy it into 1Password too, as the document
[`Zitadel | Prod | iam-admin key`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=jitp4bfd4oirrwza3vpl3fr6oi&h=my.1password.com), without printing it:

```shell
kubectl --context d3strukt0r-prod-admin -n zitadel get secret iam-admin -o jsonpath='{.data.iam-admin\.json}' | base64 -d > /tmp/iam-admin.json \
  && op document create /tmp/iam-admin.json --title 'Zitadel | Prod | iam-admin key' --account my.1password.com --vault Private \
  && rm /tmp/iam-admin.json
```

The instance's login and domain policies and the organisation's domains are in `tofu/zitadel`
(see [`tofu/zitadel/README.md`](../../../tofu/zitadel/README.md)): MFA required for Zitadel
passwords (authenticator app or security key/passkey, no e-mail or SMS codes), organisation
domains only after DNS verification, no login name suffix - so a username is the login name and
must be unique across all organisations (an e-mail address, or a handle nobody else will take,
like `D3strukt0r`). The domains `d3strukt0r.dev` (primary) and `d3st.dev` are verified by the
`_zitadel-challenge.<domain>` TXT records in `tofu/cloudflare`, which stay: Zitadel re-checks
them periodically, and a re-added domain gets a new code for the record's content.

Still by hand:

1. `auth-admin`: Password and Security → Multifactor Authentication → Authenticator App, the QR
   code scanned into [`Zitadel | Prod | Admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=hwednr7k3ycfvmdgtbcylq5nwi&h=my.1password.com), tested by logging out and in. On a fresh
   instance this comes **before** the first `tofu apply` of `tofu/zitadel`, which requires MFA.
2. Cloudflare dashboard → `d3strukt0r.dev` → Network → **gRPC on**: `tofu/zitadel`'s provider
   speaks gRPC, which Cloudflare refuses otherwise.

## Users

A new user gets a generated first password and the e-mail marked verified (there is no mail
server yet), and sets up the authenticator app at the first login. **If that first login loops**
with `mfa required (AUTHZ-Kl3p0)` instead of offering the second factor, start a new login
attempt: the first one for `D3strukt0r` (2026-09-30) let the user in without the setup and the
console refused the session over and over, while the next attempt showed the setup - in a
private window and in the normal one alike. The cause is unknown; the login page has several
such loops open upstream.

Losing the admin's second factor is recovered with the `iam-admin` key, which holds the same
instance rights and can remove the factor through the API.

```shell
kubectl --context d3strukt0r-prod-admin -n zitadel get pods,jobs,certificates,externalsecrets
```
