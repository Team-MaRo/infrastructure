# kubectl logs in through Zitadel with kubelogin (`kubectl oidc-login`), a public client with
# PKCE and no secret. The API server checks the ID token itself (the AuthenticationConfiguration
# written by ansible/roles/k3s, whose audience is this client ID); rights come from the `groups`
# claim through the ClusterRoleBinding in kubernetes/components/cluster-rbac/.

# kubelogin receives the code on http://localhost:8000, or on 18000 when 8000 is taken. Native
# apps may redirect to http://localhost. Refresh tokens keep kubectl logged in after the ID
# token expires, without a browser.
resource "zitadel_application_oidc" "kubernetes" {
  org_id     = local.org_id
  project_id = zitadel_project.infrastructure.id
  name       = "Kubernetes"

  app_type         = "OIDC_APP_TYPE_NATIVE"
  auth_method_type = "OIDC_AUTH_METHOD_TYPE_NONE"
  grant_types      = ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE", "OIDC_GRANT_TYPE_REFRESH_TOKEN"]
  response_types   = ["OIDC_RESPONSE_TYPE_CODE"]

  redirect_uris = ["http://localhost:8000", "http://localhost:18000"]

  # The API server only sees the ID token: the username and `groups` must be in it.
  id_token_userinfo_assertion = true
  id_token_role_assertion     = true
}

output "kubernetes_client_id" {
  value     = zitadel_application_oidc.kubernetes.client_id
  sensitive = true
}
