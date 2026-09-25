# External Secrets reads values from secret/ and turns them into Kubernetes Secrets. It may
# read all of secret/: every namespace that references the ClusterSecretStore can therefore
# reach every value. Acceptable while one admin runs this cluster; narrow it per namespace
# once others deploy to it.
resource "vault_policy" "external_secrets" {
  name   = "external-secrets"
  policy = <<-EOT
    path "${vault_mount.secret.path}/data/*" {
      capabilities = ["read"]
    }

    path "${vault_mount.secret.path}/metadata/*" {
      capabilities = ["read", "list"]
    }
  EOT
}

# The service account the external-secrets chart creates, in the namespace its Application
# deploys to.
resource "vault_kubernetes_auth_backend_role" "external_secrets" {
  backend                          = vault_auth_backend.kubernetes.path
  role_name                        = "external-secrets"
  bound_service_account_names      = ["external-secrets"]
  bound_service_account_namespaces = ["external-secrets"]
  token_policies                   = [vault_policy.external_secrets.name]
}
