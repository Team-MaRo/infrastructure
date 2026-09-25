# Where the port-forward listens:
#   kubectl --context d3strukt0r-prod-admin -n openbao port-forward svc/openbao 8200:8200
variable "openbao_address" {
  description = "OpenBao API address, as reached from this machine."
  type        = string
  default     = "http://127.0.0.1:8200"
}

# The initial root token for now, until OpenBao has a login method of its own for admins.
variable "openbao_token" {
  description = "OpenBao token with rights to manage mounts, auth methods and policies. Set in terraform.tfvars."
  type        = string
  sensitive   = true
}
