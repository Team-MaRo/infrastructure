# Declared as a variable rather than left to the provider's own HCLOUD_TOKEN lookup, so
# it can come from terraform.tfvars like the tokens in the other two modules.
variable "hcloud_token" {
  description = "Hetzner Cloud API token. Set in terraform.tfvars, which is gitignored."
  type        = string
  sensitive   = true
}

# CIDRs allowed to reach the Kubernetes API on 6443. Deliberately a variable rather than
# a literal: this repo is public and a home address is not something to publish. Given as
# full CIDRs rather than bare addresses so an IPv6 /128 can be added without special
# casing. No default - an empty list would produce an invalid firewall rule, and silently
# opening the API would be worse.
variable "admin_ips" {
  description = "CIDRs allowed to reach the Kubernetes API on 6443, e.g. [\"203.0.113.7/32\"]. Set in terraform.tfvars."
  type        = list(string)
}
