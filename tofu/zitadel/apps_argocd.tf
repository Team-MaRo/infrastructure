# Argo CD logs in through Zitadel directly (no Dex). Both clients are public: authorization
# code with PKCE and no client secret, which Argo CD supports for the UI and the CLI alike -
# so there is no secret to store anywhere. The client IDs are not secret; they go into
# kubernetes/components/argocd/patches/argocd-cm.yaml.

resource "zitadel_application_oidc" "argocd" {
  org_id     = local.org_id
  project_id = zitadel_project.infrastructure.id
  name       = "Argo CD"

  app_type         = "OIDC_APP_TYPE_WEB"
  auth_method_type = "OIDC_AUTH_METHOD_TYPE_NONE"
  grant_types      = ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE"]
  response_types   = ["OIDC_RESPONSE_TYPE_CODE"]

  redirect_uris             = ["https://argocd.d3strukt0r.dev/auth/callback"]
  post_logout_redirect_uris = ["https://argocd.d3strukt0r.dev"]

  # Argo CD reads the user from the ID token: profile, roles and the webhook's `groups` must be
  # in it, not only behind the userinfo endpoint.
  id_token_userinfo_assertion = true
  id_token_role_assertion     = true
}

# `argocd login --sso` opens the browser and receives the code on a local port. Native apps may
# redirect to http://localhost.
resource "zitadel_application_oidc" "argocd_cli" {
  org_id     = local.org_id
  project_id = zitadel_project.infrastructure.id
  name       = "Argo CD CLI"

  app_type         = "OIDC_APP_TYPE_NATIVE"
  auth_method_type = "OIDC_AUTH_METHOD_TYPE_NONE"
  grant_types      = ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE"]
  response_types   = ["OIDC_RESPONSE_TYPE_CODE"]

  redirect_uris = ["http://localhost:8085/auth/callback"]

  id_token_userinfo_assertion = true
  id_token_role_assertion     = true
}

# The provider marks client IDs sensitive; read them with `tofu output -raw <name>`.
output "argocd_client_id" {
  value     = zitadel_application_oidc.argocd.client_id
  sensitive = true
}

output "argocd_cli_client_id" {
  value     = zitadel_application_oidc.argocd_cli.client_id
  sensitive = true
}
