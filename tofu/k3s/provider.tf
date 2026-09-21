# Declared as a variable rather than left to the provider's own HCLOUD_TOKEN lookup, so
# it can come from terraform.tfvars like the tokens in the other two modules.
variable "hcloud_token" {
  description = "Hetzner Cloud API token. Set in terraform.tfvars, which is gitignored."
  type        = string
  sensitive   = true
}

provider "hcloud" {
  token = var.hcloud_token
}
