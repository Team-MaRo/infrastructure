# OpenBao's OIDC login (tofu/openbao/oidc.tf), for the UI at openbao.d3strukt0r.dev and for
# `bao login -method=oidc`. OpenBao needs a client secret: the one Zitadel returns at creation
# lands in this state, so it is regenerated once in the console right after; the live secret is
# kept in 1Password and OpenBao secret/openbao-oidc, from where tofu/openbao reads it
# ephemerally - it never enters a state.
# 1Password "Zitadel | Prod | OpenBao"
#   https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=66smfmdqmznzjmlv4pv4znccjq&h=my.1password.com
resource "zitadel_application_oidc" "openbao" {
  org_id     = local.org_id
  project_id = zitadel_project.infrastructure.id
  name       = "OpenBao"

  app_type         = "OIDC_APP_TYPE_WEB"
  auth_method_type = "OIDC_AUTH_METHOD_TYPE_BASIC"
  grant_types      = ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE"]
  response_types   = ["OIDC_RESPONSE_TYPE_CODE"]

  # The UI's callback, and the local one of `bao login -method=oidc`. A web app may only
  # register an http localhost address in development mode; exact matching stays.
  redirect_uris = [
    "https://openbao.d3strukt0r.dev/ui/vault/auth/oidc/oidc/callback",
    "http://localhost:8250/oidc/callback",
  ]
  dev_mode = true

  # OpenBao maps the ID token's `groups` to its external group infra-admin.
  id_token_userinfo_assertion = true
  id_token_role_assertion     = true
}

output "openbao_client_id" {
  value     = zitadel_application_oidc.openbao.client_id
  sensitive = true
}
