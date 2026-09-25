# OpenBao is cluster-internal, so this reaches it through a port-forward from the admin's
# machine (see tofu/README.md). The token is never stored in state.
provider "vault" {
  address = var.openbao_address
  token   = var.openbao_token
}
