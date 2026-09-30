# Grafana's generic OAuth login. A public client like Argo CD's: authorization code with PKCE
# and no client secret. The client ID goes into
# kubernetes/components/kube-prometheus-stack/values.yaml.
resource "zitadel_application_oidc" "grafana" {
  org_id     = local.org_id
  project_id = zitadel_project.infrastructure.id
  name       = "Grafana"

  app_type         = "OIDC_APP_TYPE_WEB"
  auth_method_type = "OIDC_AUTH_METHOD_TYPE_NONE"
  grant_types      = ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE"]
  response_types   = ["OIDC_RESPONSE_TYPE_CODE"]

  redirect_uris             = ["https://grafana.d3strukt0r.dev/login/generic_oauth"]
  post_logout_redirect_uris = ["https://grafana.d3strukt0r.dev/login"]

  # Grafana reads the role from the ID token's `groups`, added by the groups webhook.
  id_token_userinfo_assertion = true
  id_token_role_assertion     = true
}

output "grafana_client_id" {
  value     = zitadel_application_oidc.grafana.client_id
  sensitive = true
}
