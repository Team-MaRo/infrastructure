# The iam-admin key file's content (1Password document "Zitadel | Prod | iam-admin key"). It
# expires on 2029-01-01; a new key is created in the console and replaces this value.
#   https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=jitp4bfd4oirrwza3vpl3fr6oi&h=my.1password.com
variable "zitadel_jwt_profile" {
  description = "Content of the iam-admin machine key JSON. Set in terraform.tfvars."
  type        = string
  sensitive   = true
}

# The password of mailomat's SMTP user zitadel@mailomat.d3strukt0r.dev (email.tf). Ephemeral, and
# written to a write-only attribute: it never enters a plan or the state.
# 1Password "Mailomat | Prod | zitadel"
#   https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=7vpv7gubslfi4qfcvhqaod42zy&h=my.1password.com
variable "smtp_password" {
  description = "Password of the SMTP user Zitadel sends mail through. Set in terraform.tfvars."
  type        = string
  sensitive   = true
  ephemeral   = true
}
