# infomaniak

Nine domains registered at Infomaniak, all delegated to Cloudflare. Infomaniak is
registrar only, so nothing about them is *managed* from here - `infomaniak/domains.tf`
declares the expected nameservers and DNSSEC state, and every `tofu plan` checks them
against the TLD registry and warns on drift.

Shared rules, credentials and the state backend are in [`../README.md`](../README.md).

## Domains: verified against DNS, corrected through the API

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

## Two zones have no delegation check, on purpose

The `ns_delegation` check in `tofu/infomaniak` covers exactly the nine domains registered at
Infomaniak (`local.domains` is specifically that set). Two further zones live in the
Cloudflare accounts - `wundexpertinplus.com` (registered at GoDaddy) and `arepazo.ch` (an
unidentified `.ch` registrar) - but those domains are registered and managed by their
owners, and this repo's admin has no access at their registrars. A check could only warn,
never correct, so there deliberately is none: whether they point at Cloudflare is the
owners' concern. If one moves away, its zone and records in `tofu/cloudflare` are what to
clean up.

## Not managed here

Registrar contacts, transfers and renewals are out of scope, as is domain expiry monitoring
(readable via `expires_at`, but it needs a bearer token).

## Running it

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

```sh
cd tofu/infomaniak
tofu init
tofu plan
```
