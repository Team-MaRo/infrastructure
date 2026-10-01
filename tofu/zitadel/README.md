# zitadel

What is inside Zitadel - everything the Helm values in `kubernetes/components/zitadel/` do not
cover: the instance's default login and domain policies, token lifetimes and the organisation
`D3strukt0r`'s domains, adopted in `imports.tf`; the groups webhook's action; and the project
`Infrastructure` with its role and the apps of every admin UI and of kubectl. It reaches Zitadel at
`https://auth.d3strukt0r.dev` directly, no port-forward. How Zitadel itself runs on the cluster is
in [`kubernetes/components/zitadel/README.md`](../../kubernetes/components/zitadel/README.md).

Shared rules, credentials and the state backend are in [`../README.md`](../README.md).

## What the module configures

- **What is inside Zitadel is `tofu/zitadel`** (provider `zitadel/zitadel`, at least 3.8.7 - the
  first release that imports the default login policy): the instance's login and domain
  policies and the organisation's domains, adopted with import blocks after they had been set
  in the console, and the instance's token lifetimes (ID and access tokens 1 hour instead of
  12, since Kubernetes cannot revoke a token before it expires; refresh tokens at Zitadel's
  defaults). The lifetimes are instance-wide, so the admin UIs' sessions also renew hourly,
  usually unnoticed through a silent re-login. The project `Infrastructure` holds the admin
  UIs' and kubectl's applications and the role `infra-admin` (role keys carry their project's prefix, since the `groups` claim mixes all
  projects' roles); role assertion puts roles into tokens and role check makes Zitadel itself
  refuse users without a role. Role assignments are here too - the personal user is looked up
  by username. It logs in as the machine user `iam-admin` with the
  key its setup job wrote (`zitadel_jwt_profile` in the gitignored tfvars; the key expires
  2029-01-01). Human users stay in the console - their passwords and second factors are theirs.
  Kubernetes runs one `components/zitadel` however many organisations or instances exist; they
  are data in Zitadel, so per-tenant files belong in `tofu/zitadel`.
- **The `groups` claim comes from a webhook** (`components/zitadel/groups-webhook.yaml`, targets
  and executions in `tofu/zitadel/groups_claim.tf`). Zitadel sends roles only as a nested map,
  `urn:zitadel:iam:org:project:roles`, which Argo CD, OpenBao and oauth2-proxy cannot read; an
  Actions v2 target (`REST_CALL`) on the functions `preuserinfo` and `preaccesstoken` calls a
  short Python script (stock `python` image, the script in a ConfigMap - the repo builds no
  images) that answers the role keys as a list. Actions v1, a script inside Zitadel, would need
  no service, but is deprecated and goes with Zitadel v5 (user decision). Roles only arrive for
  projects with role assertion on. `interrupt_on_error = false`: with the webhook down, tokens
  lack `groups`, Zitadel logins still work and the UIs refuse (fail closed). The request
  signature is not checked - the answer only matters to Zitadel, and a NetworkPolicy admits
  only Zitadel's pods. Zitadel only calls the webhook because its deny list leaves out the
  webhook Service's address, and it checks a target against that list when the target is
  created - see "Zitadel refuses to call private addresses" in
  [`kubernetes/components/zitadel/README.md`](../../kubernetes/components/zitadel/README.md).
- **The provider speaks native gRPC**, which Cloudflare's proxy refuses with `403` unless the
  zone's gRPC switch (dashboard → Network → gRPC) is on - turned on for `d3strukt0r.dev` on
  2026-09-30. No provider manages that switch, so it is set by hand; the symptom of it being
  off is `server closed the stream without sending trailers`. It opens nothing new: the same
  API is public over REST and gRPC-web.

## Running it

`zitadel/terraform.tfvars` holds `zitadel_jwt_profile`: the whole JSON of the machine user
`iam-admin`'s key, pasted between the heredoc markers from

```sh
op document get 'Zitadel | Prod | iam-admin key' --account my.1password.com --vault Private | jq .
```

```sh
cd tofu/zitadel
tofu init
tofu plan
```

Adopting something set by hand: an `import` block, then `tofu plan -generate-config-out=generated.tf`
writes its live values; move them into
the module, delete `generated.tf`, and plan again until only the import remains. Human users are
not managed here.
