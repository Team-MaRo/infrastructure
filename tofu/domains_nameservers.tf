# Created at https://manager.infomaniak.com/v3/ng/profile/user/token/list with scopes
# domain:write (what the PUT needs) and domain:read (only so the token can be verified
# with a harmless GET). Shown once on creation, and deactivated after a year of
# inactivity - which this token will hit, since it is used only when delegation drifts.
variable "infomaniak_token" {
  description = "Infomaniak API token with scopes domain:write and domain:read, used only to correct nameserver delegation. Set via TF_VAR_infomaniak_token. Create at https://manager.infomaniak.com/v3/ng/profile/user/token/list"
  type        = string
  sensitive   = true
  default     = ""
}

# Only exists for domains whose delegation has actually drifted, so a steady state
# plans nothing and no request is ever sent needlessly. When a correction lands, the
# instance disappears again on the next plan.
#
# Infomaniak has no GET for nameservers, so there is nothing for the provider to read
# back - skip_read stays on and drift detection is the job of the ns_delegation check.
# skip_destroy is on because "destroying" a delegation setting is meaningless.
resource "terracurl_request" "nameservers" {
  for_each = toset(local.delegation_drift)

  name = "nameservers-${each.key}"

  # The {domain} path segment takes the domain name. Infomaniak also assigns numeric
  # domain ids and the API accepts either, verified against /dnssec/check which takes
  # the same parameter - both forms return 200. The name is used here so the config
  # needs no id lookup and stays readable.
  url    = "https://api.infomaniak.com/2/domains/${each.key}/nameservers"
  method = "PUT"

  # Write-only: the token is sent but never written to state. That matters here
  # because the state bucket has Object Lock, so a leaked secret could not be purged,
  # only rotated.
  headers_wo = {
    Authorization  = "Bearer ${var.infomaniak_token}"
    "Content-Type" = "application/json"
  }
  headers_wo_version = 1

  # Also write-only, purely so no permanent "use the WriteOnly version" warning is
  # emitted. Drift detection here is the ns_delegation check, which reports through
  # warnings, so an unrelated standing warning would be actively harmful. The body is
  # not secret: it is local.expected_nameservers.
  request_body_wo = jsonencode({
    nameservers            = local.expected_nameservers
    verify_ns_availability = true
  })
  request_body_wo_version = 1

  response_codes = ["200"]
  skip_read      = true
  skip_destroy   = true

  lifecycle {
    precondition {
      condition     = var.infomaniak_token != ""
      error_message = "Delegation for ${each.key} has drifted and needs correcting, but no Infomaniak token is set. Export TF_VAR_infomaniak_token and re-run."
    }
  }
}
