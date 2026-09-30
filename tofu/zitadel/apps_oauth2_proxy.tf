# oauth2-proxy guards the UIs without a login of their own - today the Traefik dashboard - as
# Traefik's forwardAuth (kubernetes/components/oauth2-proxy/). Unlike the other
# clients it needs a client secret. The one Zitadel returns at creation lands in this state, so
# it is regenerated once in the console right after; the live secret is kept in 1Password and
# OpenBao secret/oauth2-proxy only.
resource "zitadel_application_oidc" "oauth2_proxy" {
  org_id     = local.org_id
  project_id = zitadel_project.infrastructure.id
  name       = "oauth2-proxy"

  app_type         = "OIDC_APP_TYPE_WEB"
  auth_method_type = "OIDC_AUTH_METHOD_TYPE_BASIC"
  grant_types      = ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE"]
  response_types   = ["OIDC_RESPONSE_TYPE_CODE"]

  redirect_uris = ["https://oauth2-proxy.d3strukt0r.dev/oauth2/callback"]

  # oauth2-proxy checks the `groups` claim of the ID token (--allowed-group).
  id_token_userinfo_assertion = true
  id_token_role_assertion     = true
}

output "oauth2_proxy_client_id" {
  value     = zitadel_application_oidc.oauth2_proxy.client_id
  sensitive = true
}
