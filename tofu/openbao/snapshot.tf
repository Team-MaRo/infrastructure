# The chart's snapshot agent (kubernetes/components/openbao/values.yaml, snapshotAgent) logs in
# with its service account every 6 hours and saves a Raft snapshot. Reading the snapshot is all
# it may do - no sudo needed for that path.
resource "vault_policy" "snapshot" {
  name   = "snapshot"
  policy = <<-EOT
    path "sys/storage/raft/snapshot" {
      capabilities = ["read"]
    }
  EOT
}

# A snapshot contains the token that took it, and a restore brings that token back without a
# way to revoke it (openbao#522) - so it lives only as long as one run needs.
resource "vault_kubernetes_auth_backend_role" "snapshot" {
  backend                          = vault_auth_backend.kubernetes.path
  role_name                        = "snapshot"
  bound_service_account_names      = ["openbao-snapshot"]
  bound_service_account_namespaces = ["openbao"]
  token_policies                   = [vault_policy.snapshot.name]
  token_ttl                        = 600
}
