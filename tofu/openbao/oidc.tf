# Admins log in through Zitadel (the app in tofu/zitadel/apps_openbao.tf), in the UI at
# https://openbao.d3strukt0r.dev and with `bao login -method=oidc`. Rights come only from
# Zitadel's role: the `groups` claim value infra-admin maps to the external group below, which
# carries the admin policy. The root token stays the break-glass login.

# OpenBao refuses an OIDC login without a client secret. It is kept in OpenBao itself
# (secret/openbao-oidc, put in by hand from 1Password "Zitadel | Prod | OpenBao"), read here
# ephemerally and written write-only: it never enters this state.
ephemeral "vault_kv_secret_v2" "oidc" {
  mount = vault_mount.secret.path
  name  = "openbao-oidc"
}

resource "vault_jwt_auth_backend" "oidc" {
  path               = "oidc"
  type               = "oidc"
  oidc_discovery_url = "https://auth.d3strukt0r.dev"
  oidc_client_id     = "393067097493602657"
  # Write-only values are only sent when the version changes: bump it after rotating the
  # secret in Zitadel, 1Password and secret/openbao-oidc.
  oidc_client_secret_wo         = ephemeral.vault_kv_secret_v2.oidc.data["client-secret"]
  oidc_client_secret_wo_version = 1
  # A plain name rather than a reference: the role depends on this backend, not the other way.
  default_role = "default"
}

resource "vault_jwt_auth_backend_role" "default" {
  backend         = vault_jwt_auth_backend.oidc.path
  role_name       = "default"
  role_type       = "oidc"
  user_claim      = "sub"
  groups_claim    = "groups"
  bound_audiences = ["393067097493602657"]
  oidc_scopes     = ["openid", "profile", "email"]
  allowed_redirect_uris = [
    "https://openbao.d3strukt0r.dev/ui/vault/auth/oidc/oidc/callback",
    "http://localhost:8250/oidc/callback",
  ]
}

# Everything, including the sudo-protected paths - what the root token can do, but tied to a
# person's login and revocable by taking the role away in Zitadel.
resource "vault_policy" "admin" {
  name   = "admin"
  policy = <<-EOT
    path "*" {
      capabilities = ["create", "read", "update", "patch", "delete", "list", "sudo"]
    }
  EOT
}

resource "vault_identity_group" "infra_admin" {
  name     = "infra-admin"
  type     = "external"
  policies = [vault_policy.admin.name]
}

resource "vault_identity_group_alias" "infra_admin" {
  name           = "infra-admin"
  mount_accessor = vault_jwt_auth_backend.oidc.accessor
  canonical_id   = vault_identity_group.infra_admin.id
}
