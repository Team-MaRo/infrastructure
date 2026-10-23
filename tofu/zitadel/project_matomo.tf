# Matomo (web analytics): its own project, so who sees which site's statistics is decided here.
# A role `matomo:<siteID>:<view|write|admin>` grants that right on that Matomo site; the groups
# webhook turns these keys into the `matomo-permission-path` claim, from which Matomo's
# RebelOIDC plugin sets the user's site rights at every login (README "Matomo").
resource "zitadel_project" "matomo" {
  org_id = local.org_id
  name   = "Matomo"

  # The roles go into the tokens and reach the groups webhook.
  project_role_assertion = true
  # Zitadel refuses anyone without a role here, before Matomo is reached.
  project_role_check = true
  # And anyone whose organisation neither owns nor was granted the project.
  has_project_check = true
}

# `matomo:superuser` only lets its holder past Zitadel's role check; the webhook makes no site
# right of it, and the superuser flag itself is set once in Matomo (README "Matomo").
resource "zitadel_project_role" "matomo" {
  # A site's roles join when it is added to Matomo (README "Matomo"), e.g.
  # "matomo:3:admin" = "arepazo.ch admin".
  for_each = {
    "matomo:superuser" = "Superuser"
  }

  org_id       = local.org_id
  project_id   = zitadel_project.matomo.id
  role_key     = each.key
  display_name = each.value
}

resource "zitadel_user_grant" "d3strukt0r_matomo" {
  org_id     = local.org_id
  project_id = zitadel_project.matomo.id
  user_id    = one(data.zitadel_human_users.d3strukt0r.user_ids)
  role_keys  = [zitadel_project_role.matomo["matomo:superuser"].role_key]
}

# Matomo's login (RebelOIDC, kubernetes/components/matomo). The plugin has no PKCE and sends the
# client secret in the request body, which Zitadel accepts for BASIC apps. The secret Zitadel
# returns at creation lands in this state, so it is regenerated in the console right after; the
# live secret is kept in 1Password and in Matomo's plugin settings only. The plugin reads roles
# and site rights by decoding the access token, so that must be a JWT.
resource "zitadel_application_oidc" "matomo" {
  org_id     = local.org_id
  project_id = zitadel_project.matomo.id
  name       = "matomo"

  app_type          = "OIDC_APP_TYPE_WEB"
  auth_method_type  = "OIDC_AUTH_METHOD_TYPE_BASIC"
  grant_types       = ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE"]
  response_types    = ["OIDC_RESPONSE_TYPE_CODE"]
  access_token_type = "OIDC_TOKEN_TYPE_JWT"

  redirect_uris = ["https://matomo.d3strukt0r.dev/index.php?module=RebelOIDC&action=callback&provider=oidc"]
  # Matomo's login_logout_url (common.config.ini.php), which the plugin passes on at logout.
  post_logout_redirect_uris = ["https://matomo.d3strukt0r.dev/"]

  access_token_role_assertion = true
  id_token_userinfo_assertion = true
  id_token_role_assertion     = true
}

output "matomo_client_id" {
  value     = zitadel_application_oidc.matomo.client_id
  sensitive = true
}
