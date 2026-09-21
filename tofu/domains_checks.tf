# The registrar cannot be asked what the nameservers are - Infomaniak's API exposes
# PUT /2/domains/{domain}/nameservers but no matching GET. The TLD registry is queried
# instead, which is both readable and authoritative for delegation.
data "dns_ns_record_set" "delegation" {
  for_each = local.domains

  host = each.key
}

# The dns provider has no DS data source, so DNSSEC is probed with dig. Infomaniak does
# offer GET /2/domains/{domain}/dnssec/check, but reaching it needs a bearer token and
# data "http" persists its request headers into the state file.
data "external" "ds" {
  for_each = local.domains

  program = ["${path.module}/scripts/ds-lookup.sh"]
  query   = { domain = each.key }
}

# Check failures surface as warnings on every plan, not as errors. That is deliberate:
# a transient resolution failure must not block unrelated cluster work.
check "ns_delegation" {
  assert {
    condition     = length(local.delegation_drift) == 0
    error_message = "NS delegation is no longer pointing at Cloudflare for: ${join(", ", local.delegation_drift)}"
  }
}

check "dnssec_state" {
  assert {
    condition     = length(local.dnssec_drift) == 0
    error_message = "DNSSEC differs from the declared state for: ${join(", ", local.dnssec_drift)}"
  }
}
