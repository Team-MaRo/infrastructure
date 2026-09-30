# Adoption of what was set by hand in the console after the first start, kept in version
# control so the adoption itself is reviewable. The default policies take the placeholder ID
# "default", which Zitadel ignores.

import {
  to = zitadel_default_login_policy.this
  id = "default"
}

import {
  to = zitadel_default_domain_policy.this
  id = "default"
}

import {
  for_each = toset(["d3strukt0r.dev", "d3st.dev"])

  to = zitadel_organization_domain.d3strukt0r[each.key]
  id = "${local.org_id}:${each.key}"
}

import {
  to = zitadel_default_oidc_settings.this
  id = "default"
}
