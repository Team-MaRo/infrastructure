# The iam-admin key file's content (1Password document "Zitadel | Prod | iam-admin key"). It
# expires on 2029-01-01; a new key is created in the console and replaces this value.
variable "zitadel_jwt_profile" {
  description = "Content of the iam-admin machine key JSON. Set in terraform.tfvars."
  type        = string
  sensitive   = true
}
