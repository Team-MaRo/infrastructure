# Both tokens are created at https://dash.cloudflare.com/profile/api-tokens, each
# scoped to its own account and all zones in it, with:
#   Zone / Zone           Read
#   Zone / DNS            Edit
#   Zone / Zone Settings  Edit

variable "cloudflare_token_personal" {
  description = "API token for the Cloudflare account holding the domains registered at Infomaniak. Set via TF_VAR_cloudflare_token_personal."
  type        = string
  sensitive   = true
}

variable "cloudflare_token_arepazo" {
  description = "API token for the Cloudflare account holding arepazo.ch. Set via TF_VAR_cloudflare_token_arepazo."
  type        = string
  sensitive   = true
}
