# Where the port-forward listens:
#   kubectl --context d3strukt0r-prod-admin -n openbao port-forward svc/openbao 8200:8200
variable "openbao_address" {
  description = "OpenBao API address, as reached from this machine."
  type        = string
  default     = "http://127.0.0.1:8200"
}

# The initial root token for now. Admins log in through Zitadel (oidc.tf); once the root token
# is revoked, this module needs a credential of its own.
variable "openbao_token" {
  description = "OpenBao token with rights to manage mounts, auth methods and policies. Set in terraform.tfvars."
  type        = string
  sensitive   = true
}
