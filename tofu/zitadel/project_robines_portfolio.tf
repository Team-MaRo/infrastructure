# The WordPress site robines-portfolio (kubernetes/components/robines-portfolio): its own project,
# so who may log in to it is decided here and nowhere else. A role is named after the WordPress
# role it gives - the site's mu-plugin sets that role at every login (charts/wordpress, README
# "Login through Zitadel"); without one Zitadel refuses the login.
resource "zitadel_project" "robines_portfolio" {
  org_id = local.org_id
  name   = "robines-portfolio"

  # The roles go into userinfo, where WordPress reads them.
  project_role_assertion = true
  # Zitadel refuses anyone without a role here, before WordPress is reached.
  project_role_check = true
  # And anyone whose organisation neither owns nor was granted the project.
  has_project_check = true
}

resource "zitadel_project_role" "robines_portfolio_administrator" {
  org_id       = local.org_id
  project_id   = zitadel_project.robines_portfolio.id
  role_key     = "robines-portfolio:administrator"
  display_name = "robines-portfolio administrator"
}

resource "zitadel_project_role" "robines_portfolio_editor" {
  org_id       = local.org_id
  project_id   = zitadel_project.robines_portfolio.id
  role_key     = "robines-portfolio:editor"
  display_name = "robines-portfolio editor"
}

# WordPress's plugin (OpenID Connect Generic Client) has no PKCE, so the app needs a client
# secret. The one Zitadel returns at creation lands in this state, so it is regenerated once in
# the console right after; the live secret is kept in 1Password and OpenBao
# secret/robines-portfolio-oidc only. The plugin sends it in the request body
# (client_secret_post), which Zitadel accepts for BASIC apps as well.
resource "zitadel_application_oidc" "robines_portfolio" {
  org_id     = local.org_id
  project_id = zitadel_project.robines_portfolio.id
  name       = "robines-portfolio"

  app_type         = "OIDC_APP_TYPE_WEB"
  auth_method_type = "OIDC_AUTH_METHOD_TYPE_BASIC"
  grant_types      = ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE"]
  response_types   = ["OIDC_RESPONSE_TYPE_CODE"]

  redirect_uris = ["https://old.robines.space/wp-admin/admin-ajax.php?action=openid-connect-authorize"]
  # Where WordPress sends people after logging out (charts/wordpress files/oidc.php trims it to
  # exactly this).
  post_logout_redirect_uris = ["https://old.robines.space/wp-login.php?loggedout=true"]
}

output "robines_portfolio_client_id" {
  value     = zitadel_application_oidc.robines_portfolio.client_id
  sensitive = true
}

resource "zitadel_user_grant" "d3strukt0r_robines_portfolio" {
  org_id     = local.org_id
  project_id = zitadel_project.robines_portfolio.id
  user_id    = one(data.zitadel_human_users.d3strukt0r.user_ids)
  role_keys  = [zitadel_project_role.robines_portfolio_administrator.role_key]
}

# The user's wife, administrator like him: she designs the site too, and editors cannot touch
# the theme (edit_theme_options). Her WordPress account `ruby` is linked to this user by hand
# (charts/wordpress, README "Linking an existing account").
data "zitadel_human_users" "ruby" {
  org_id           = local.org_id
  user_name        = "Ruby"
  user_name_method = "TEXT_QUERY_METHOD_EQUALS"
}

resource "zitadel_user_grant" "ruby_robines_portfolio" {
  org_id     = local.org_id
  project_id = zitadel_project.robines_portfolio.id
  user_id    = one(data.zitadel_human_users.ruby.user_ids)
  role_keys  = [zitadel_project_role.robines_portfolio_administrator.role_key]
}
