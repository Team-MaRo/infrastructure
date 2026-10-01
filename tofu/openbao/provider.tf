# Two ways in, one of them per run:
# - day to day, an admin's own token from `bao login -method=oidc -no-print` (Zitadel): with
#   openbao_token unset, the provider reads it from ~/.vault-token, where bao stores it;
# - on a cluster rebuilt from scratch, where the OIDC login does not exist yet, the root token
#   from 1Password, as openbao_token in terraform.tfvars:
#   1Password "OpenBao | Prod | Recovery keys & root token"
#     https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=6ftev2p3fo3dc457whshzgn6jy&h=my.1password.com
# Either way the provider works with a short-lived child token, and no token is stored in state.
provider "vault" {
  address = var.openbao_address
  token   = var.openbao_token
}
