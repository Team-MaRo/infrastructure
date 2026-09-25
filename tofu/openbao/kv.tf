# Where the cluster's secrets live. Only the engine is managed here - the values are written
# with `bao kv put`, never through OpenTofu, because anything OpenTofu writes lands in its
# state file.
resource "vault_mount" "secret" {
  path    = "secret"
  type    = "kv"
  options = { version = "2" }
}
