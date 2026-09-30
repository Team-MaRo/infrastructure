# Two ways in, one of them per run:
# - day to day, an admin's own token from `bao login -method=oidc -no-print` (Zitadel): with
#   openbao_token unset, the provider reads it from ~/.vault-token, where bao stores it;
# - on a cluster rebuilt from scratch, where the OIDC login does not exist yet, the root token
#   from 1Password, as openbao_token in terraform.tfvars.
# Either way the provider works with a short-lived child token, and no token is stored in state.
provider "vault" {
  address = var.openbao_address
  token   = var.openbao_token
}
