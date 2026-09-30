# The public API. On a rebuilt cluster whose Ingress is not up yet, a port-forward instead:
#   kubectl --context d3strukt0r-prod-admin -n openbao port-forward svc/openbao 8200:8200
# with openbao_address = "http://127.0.0.1:8200" in terraform.tfvars.
variable "openbao_address" {
  description = "OpenBao API address, as reached from this machine."
  type        = string
  default     = "https://openbao.d3strukt0r.dev"
}

# Only needed while no admin can log in through Zitadel - a cluster rebuilt from scratch. Left
# unset, the token of `bao login -method=oidc` is used (provider.tf).
variable "openbao_token" {
  description = "OpenBao token with rights to manage mounts, auth methods and policies - the root token, only when no OIDC login exists yet. Unset: the token from ~/.vault-token."
  type        = string
  sensitive   = true
  default     = null
}
