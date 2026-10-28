# zot, the container registry: the login to its web UI at registry.d3strukt0r.dev
# (kubernetes/components/zot/). zot needs a client secret, like oauth2-proxy; the one Zitadel
# returns at creation lands in this state, so it is regenerated once in the console right after;
# the live secret is kept in 1Password and OpenBao secret/zot-oidc only.
# 1Password "Zitadel | Prod | zot"
#   https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=44rf2knxnk3y4wvjbn5yg7w44i&h=my.1password.com
resource "zitadel_application_oidc" "zot" {
  org_id     = local.org_id
  project_id = zitadel_project.infrastructure.id
  name       = "zot"

  app_type         = "OIDC_APP_TYPE_WEB"
  auth_method_type = "OIDC_AUTH_METHOD_TYPE_BASIC"
  grant_types      = ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE"]
  response_types   = ["OIDC_RESPONSE_TYPE_CODE"]

  redirect_uris             = ["https://registry.d3strukt0r.dev/zot/auth/callback/oidc"]
  post_logout_redirect_uris = ["https://registry.d3strukt0r.dev/"]

  # zot maps the `groups` claim of the ID token to its access rules (infra-admin).
  id_token_userinfo_assertion = true
  id_token_role_assertion     = true
}

output "zot_client_id" {
  value     = zitadel_application_oidc.zot.client_id
  sensitive = true
}
