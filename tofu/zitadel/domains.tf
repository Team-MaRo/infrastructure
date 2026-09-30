# The organisation's own domains, verified through the _zitadel-challenge TXT records in
# tofu/cloudflare, which Zitadel re-checks periodically. d3strukt0r.dev is primary.
resource "zitadel_organization_domain" "d3strukt0r" {
  for_each = toset(["d3strukt0r.dev", "d3st.dev"])

  organization_id = local.org_id
  domain          = each.key
}
