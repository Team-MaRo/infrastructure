# The WooCommerce shop arepazo (kubernetes/components/arepazo), owned and run by the user's brother
# Eduardo. Its people - staff and customers - live in their own organisation `Arepazo`, which
# Eduardo owns; the project stays in ours and is granted to it. Customers register themselves
# (login policy below), and the registration webhook gives every new member of Arepazo the role
# `arepazo:customer` (registration_roles below, kubernetes/components/zitadel README
# "Registration").
resource "zitadel_organization" "arepazo" {
  name = "Arepazo"
}

output "arepazo_org_id" {
  value = zitadel_organization.arepazo.id
}

# The instance's login policy (policies.tf) with registration on and no second factor required:
# a shopper should not need an authenticator app to buy something. Anyone may still add one in
# their account, and Eduardo and his staff should - this policy cannot require it for them only.
resource "zitadel_login_policy" "arepazo" {
  org_id = zitadel_organization.arepazo.id

  allow_register       = true
  force_mfa            = false
  force_mfa_local_only = false
  # Never asks after a login to set up a second factor.
  mfa_init_skip_lifetime = "0s"

  second_factors                = ["SECOND_FACTOR_TYPE_OTP", "SECOND_FACTOR_TYPE_U2F"]
  multi_factors                 = ["MULTI_FACTOR_TYPE_U2F_WITH_VERIFICATION"]
  passwordless_type             = "PASSWORDLESS_TYPE_ALLOWED"
  user_login                    = true
  allow_external_idp            = true
  allow_domain_discovery        = true
  disable_login_with_email      = false
  disable_login_with_phone      = false
  hide_password_reset           = false
  ignore_unknown_usernames      = false
  default_redirect_uri          = ""
  idps                          = []
  password_check_lifetime       = "240h0m0s"
  external_login_check_lifetime = "240h0m0s"
  second_factor_check_lifetime  = "18h0m0s"
  multi_factor_check_lifetime   = "12h0m0s"
}

resource "zitadel_project" "arepazo" {
  org_id = local.org_id
  name   = "arepazo"

  # The roles go into userinfo, where WordPress reads them.
  project_role_assertion = true
  # Zitadel refuses anyone without a role here, before WordPress is reached.
  project_role_check = true
  # And anyone whose organisation neither owns nor was granted the project.
  has_project_check = true
}

# A WordPress role per key (WordPress takes every one it knows, charts/wordpress README "Login
# through Zitadel"); another one is one more entry.
resource "zitadel_project_role" "arepazo" {
  for_each = toset(["administrator", "shop_manager", "customer"])

  org_id       = local.org_id
  project_id   = zitadel_project.arepazo.id
  role_key     = "arepazo:${each.key}"
  display_name = "arepazo ${each.key}"
}

# Arepazo may give its people every role of the project; its owner does that in the console.
resource "zitadel_project_grant" "arepazo" {
  org_id         = local.org_id
  project_id     = zitadel_project.arepazo.id
  granted_org_id = zitadel_organization.arepazo.id
  role_keys      = [for role in zitadel_project_role.arepazo : role.role_key]
}

# As for robines-portfolio: no PKCE in the plugin, so a client secret, regenerated in the console
# after creation and kept in 1Password and OpenBao secret/arepazo-oidc only.
resource "zitadel_application_oidc" "arepazo" {
  org_id     = local.org_id
  project_id = zitadel_project.arepazo.id
  name       = "arepazo"

  app_type         = "OIDC_APP_TYPE_WEB"
  auth_method_type = "OIDC_AUTH_METHOD_TYPE_BASIC"
  grant_types      = ["OIDC_GRANT_TYPE_AUTHORIZATION_CODE"]
  response_types   = ["OIDC_RESPONSE_TYPE_CODE"]

  redirect_uris             = ["https://www.arepazo.ch/wp-admin/admin-ajax.php?action=openid-connect-authorize"]
  post_logout_redirect_uris = ["https://www.arepazo.ch/wp-login.php?loggedout=true"]
}

output "arepazo_client_id" {
  value     = zitadel_application_oidc.arepazo.client_id
  sensitive = true
}

resource "zitadel_user_grant" "d3strukt0r_arepazo" {
  org_id     = local.org_id
  project_id = zitadel_project.arepazo.id
  user_id    = one(data.zitadel_human_users.d3strukt0r.user_ids)
  role_keys  = [zitadel_project_role.arepazo["administrator"].role_key]
}

# What every new member of Arepazo gets, read by the registration webhook.
resource "zitadel_organization_metadata" "arepazo_registration_roles" {
  organization_id = zitadel_organization.arepazo.id
  key             = "registration-roles"
  value = jsonencode({
    project = zitadel_project.arepazo.id
    roles   = [zitadel_project_role.arepazo["customer"].role_key]
  })
}

# The registration webhook's user (registration.tf) may read Arepazo, its metadata and its users
# and give them roles of the granted project - and nothing outside it.
resource "zitadel_org_member" "registration_arepazo" {
  org_id  = zitadel_organization.arepazo.id
  user_id = zitadel_machine_user.registration.id
  roles   = ["ORG_USER_MANAGER"]
}

moved {
  from = zitadel_org_member.arepazo_registration
  to   = zitadel_org_member.registration_arepazo
}
