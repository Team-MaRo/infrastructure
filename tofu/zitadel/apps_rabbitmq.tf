# RabbitMQ's management UI. The UI is a single-page app: the browser itself runs the
# authorization code flow with PKCE and sends the access token to RabbitMQ, so this is a public
# user-agent client without a secret. RabbitMQ reads the user's rights from that access token -
# it must be a JWT, carrying the roles and the groups webhook's flat `groups` list, which
# RabbitMQ maps onto its own scopes (kubernetes/components/rabbitmq). The client ID is not
# secret; it is also RabbitMQ's resource_server_id, so only tokens issued for this app count.
resource "zitadel_application_oidc" "rabbitmq" {
  org_id     = local.org_id
  project_id = zitadel_project.infrastructure.id
  name       = "RabbitMQ"

  app_type          = "OIDC_APP_TYPE_USER_AGENT"
  auth_method_type  = "OIDC_AUTH_METHOD_TYPE_NONE"
  grant_types       = ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE"]
  response_types    = ["OIDC_RESPONSE_TYPE_CODE"]
  access_token_type = "OIDC_TOKEN_TYPE_JWT"

  redirect_uris             = ["https://rabbitmq.d3strukt0r.dev/js/oidc-oauth/login-callback.html"]
  post_logout_redirect_uris = ["https://rabbitmq.d3strukt0r.dev/"]

  access_token_role_assertion = true
  id_token_userinfo_assertion = true
  id_token_role_assertion     = true
}

output "rabbitmq_client_id" {
  value     = zitadel_application_oidc.rabbitmq.client_id
  sensitive = true
}
