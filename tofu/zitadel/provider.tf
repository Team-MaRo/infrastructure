# Zitadel is public at its own address, so no port-forward is needed. It authenticates as the
# machine user iam-admin, the instance's second owner, with the key its setup job created.
provider "zitadel" {
  domain           = "auth.d3strukt0r.dev"
  jwt_profile_json = var.zitadel_jwt_profile
}
