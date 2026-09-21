# Both tokens are created at https://dash.cloudflare.com/profile/api-tokens, each
# scoped to its own account and all zones in it, with:
#   Zone / Zone           Read
#   Zone / DNS            Edit
#   Zone / Zone Settings  Edit

variable "cloudflare_token_personal" {
  description = "API token for the Cloudflare account holding the domains registered at Infomaniak. Set in terraform.tfvars."
  type        = string
  sensitive   = true
}

variable "cloudflare_token_arepazo" {
  description = "API token for the Cloudflare account holding arepazo.ch. Set in terraform.tfvars."
  type        = string
  sensitive   = true
}
