# cloudflare

DNS for every zone lives in Cloudflare, across **two separate accounts** under two
separate logins.

| Account | Nameservers | Zones |
|---|---|---|
| personal | `brenda` / `wesley`.ns.cloudflare.com | the nine Infomaniak domains, plus `wundexpertinplus.com` (registered at GoDaddy and managed by its owner) |
| arepazo | `abdullah` / `fish`.ns.cloudflare.com | `arepazo.ch` (registered and managed by its owner) |

It manages the zones, every DNS record, and six security and TLS zone settings
(`ssl_automatic_mode`, `always_use_https`, `min_tls_version`, `automatic_https_rewrites`,
`tls_1_3`, `security_level`). Page rules, WAF, Workers and the remaining ~54 zone settings
stay in the dashboard.

The SSL mode itself (`ssl`: flexible, full, strict) is **not** managed. Every zone uses
Automatic SSL/TLS: Cloudflare scans the origin and picks the strictest mode that works, so
`ssl` is only the latest scan's result. Writing it would switch the zone to manual mode;
managing `ssl_automatic_mode = "auto"` instead makes such a switch visible as drift. Every
zone requires TLS 1.2 from visitors.

What the scanner has chosen is still visible, read-only, through the `ssl_modes` output:

```sh
tofu output ssl_modes
```

Shared rules, credentials and the state backend are in [`../README.md`](../README.md).

## Two accounts, no default provider

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

## The old server and the cluster share the names

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
  [`kubernetes/components/traefik/README.md`](../../kubernetes/components/traefik/README.md)): k3s itself runs single-stack IPv4 (pods
  `10.42.0.0/16`, services `10.43.0.0/16`), dual-stack can only be chosen when a cluster is
  created, and on Hetzner it
  would also move pod traffic onto the public interface, because private networks are
  IPv4-only and Flannel uses one interface for both families. So pods reach IPv4 only; an
  IPv6-only destination is out of their reach. The node IPs are
  written there by hand (no other module's state is readable from here), so **adding or
  replacing a node means updating that map**, until a Hetzner Load Balancer gives the
  cluster one address.
- The rename was done without an interruption: prod-old was created with the old IPs and the
  CNAMEs were repointed to it first, in a separate apply before `prod` itself changed.

## CAA: only Let's Encrypt

Every zone has `0 issue "letsencrypt.org"` and `0 issuewild "letsencrypt.org"`
(`records_caa.tf`), since every certificate issued for these names comes from Let's Encrypt
- the old server's Traefik, GitHub Pages and cert-manager. Cloudflare's edge certificates
come from Google Trust Services and SSL.com (crt.sh showed nothing else in use when this was
set); **Cloudflare adds CAA records for its own CAs automatically** once a zone has any, and
hides them from the dashboard and API, so they never appear as drift - after the apply that
was `comodoca.com`, `digicert.com`, `pki.goog` and `ssl.com`, `issue` and `issuewild` each.
`dig CAA <zone> @1.1.1.1` shows the full set. A new certificate source using another CA needs that CA added here first.

## Records Cloudflare owns but OpenTofu now tracks

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
these records itself (see [`kubernetes/components/cert-manager/README.md`](../../kubernetes/components/cert-manager/README.md));
OpenTofu must not track them.

## Running it

Two API tokens, one per account, from <https://dash.cloudflare.com/profile/api-tokens>.
Each needs Zone/Zone **Read**, Zone/DNS **Edit**, Zone/Zone Settings **Edit**, and its
zone resources scoped to `All zones from an account` - not `All zones`, which would
reach both accounts and make the split meaningless.

Both go in `cloudflare/terraform.tfvars`.

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
