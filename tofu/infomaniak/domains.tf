locals {
  # Every domain is registered at Infomaniak and delegated to Cloudflare. Infomaniak
  # is registrar only ("Externe DNS-Zone" in their console).
  cloudflare_nameservers = ["brenda.ns.cloudflare.com.", "wesley.ns.cloudflare.com."]

  # dnssec is the expected state at the registry, i.e. whether a DS record exists in
  # the parent zone. None of these have DNSSEC enabled today.
  domains = {
    "d3st.dev"               = { dnssec = false }
    "d3st.org"               = { dnssec = false }
    "d3strukt0r.dev"         = { dnssec = false }
    "d3strukt0r.me"          = { dnssec = false }
    "manuele-robine.wedding" = { dnssec = false }
    "manuele-vaccari.ch"     = { dnssec = false }
    "robines.space"          = { dnssec = false }
    "rubyn.li"               = { dnssec = false }
    "sponte.me"              = { dnssec = false }
  }

  # Normalised once so the checks below compare like with like - the dns provider is
  # not documented to return a trailing dot either way.
  expected_nameservers = [for ns in local.cloudflare_nameservers : trimsuffix(ns, ".")]

  actual_nameservers = {
    for domain, records in data.dns_ns_record_set.delegation :
    domain => [for ns in records.nameservers : trimsuffix(ns, ".")]
  }

  delegation_drift = [
    for domain, nameservers in local.actual_nameservers :
    domain if nameservers != local.expected_nameservers
  ]

  dnssec_drift = [
    for domain, config in local.domains :
    domain if tobool(data.external.ds[domain].result.has_ds) != config.dnssec
  ]
}
