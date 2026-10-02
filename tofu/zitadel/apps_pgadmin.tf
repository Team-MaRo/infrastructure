# pgAdmin's OAuth2 login. A public client like Grafana's: authorization code with PKCE and no
# client secret. The client ID goes into kubernetes/components/pgadmin/config_system.py.
resource "zitadel_application_oidc" "pgadmin" {
  org_id     = local.org_id
  project_id = zitadel_project.infrastructure.id
  name       = "pgAdmin"

  app_type         = "OIDC_APP_TYPE_WEB"
  auth_method_type = "OIDC_AUTH_METHOD_TYPE_NONE"
  grant_types      = ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE"]
  response_types   = ["OIDC_RESPONSE_TYPE_CODE"]

  redirect_uris             = ["https://pgadmin.d3strukt0r.dev/oauth2/authorize"]
  post_logout_redirect_uris = ["https://pgadmin.d3strukt0r.dev/"]

  # pgAdmin checks the ID token's `groups`, added by the groups webhook, for infra-admin.
  id_token_userinfo_assertion = true
  id_token_role_assertion     = true
}

output "pgadmin_client_id" {
  value     = zitadel_application_oidc.pgadmin.client_id
  sensitive = true
}
