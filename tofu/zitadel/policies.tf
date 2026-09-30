# The instance's defaults, which every organisation inherits unless it sets its own. Every
# argument is required by the provider, so the values Zitadel shipped with are written out too.

resource "zitadel_default_login_policy" "this" {
  # A second factor for everyone who logs in with a Zitadel password; users of an external
  # identity provider rely on that provider's MFA. Allowed factors: authenticator app (OTP) and
  # security key or passkey (U2F) - no e-mail or SMS codes, there is no provider for either.
  force_mfa            = false
  force_mfa_local_only = true
  second_factors       = ["SECOND_FACTOR_TYPE_OTP", "SECOND_FACTOR_TYPE_U2F"]
  multi_factors        = ["MULTI_FACTOR_TYPE_U2F_WITH_VERIFICATION"]
  passwordless_type    = "PASSWORDLESS_TYPE_ALLOWED"

  # Nobody signs up on their own: accounts are created by an admin. Zitadel ships with it on,
  # which let anyone on the internet create an (unverified) account.
  allow_register = false

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
  mfa_init_skip_lifetime        = "720h0m0s"
  second_factor_check_lifetime  = "18h0m0s"
  multi_factor_check_lifetime   = "12h0m0s"
}

resource "zitadel_default_domain_policy" "this" {
  # No organisation may claim a domain it does not control. Login names get no organisation
  # suffix, so a username is the login name and must be unique across all organisations.
  validate_org_domains                        = true
  user_login_must_be_domain                   = false
  smtp_sender_address_matches_instance_domain = false
}

# Token lifetimes for every app. Kubernetes never asks Zitadel whether a token is still good,
# so the ID token's lifetime is how long a removed role or a deactivated user keeps working in
# kubectl: 1 hour instead of Zitadel's 12. Clients renew silently with their refresh token,
# which lapses after 30 days unused (the default) and after 90 days in any case (the default).
resource "zitadel_default_oidc_settings" "this" {
  access_token_lifetime         = "1h0m0s"
  id_token_lifetime             = "1h0m0s"
  refresh_token_idle_expiration = "720h0m0s"
  refresh_token_expiration      = "2160h0m0s"
}
