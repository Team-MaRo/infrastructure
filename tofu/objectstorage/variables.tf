# Not a secret, but together with the access key it identifies the account, and this repo
# is public.
variable "project_id" {
  description = "Numeric ID of the Hetzner Cloud project the buckets belong to. Set in terraform.tfvars."
  type        = string

  # It goes straight into the principal, where an empty or mistyped value locks the admin
  # key out instead of failing.
  validation {
    condition     = can(regex("^[0-9]+$", var.project_id))
    error_message = "project_id must be the numeric project ID from the console URL."
  }
}
