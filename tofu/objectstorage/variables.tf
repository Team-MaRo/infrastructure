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

# Access key IDs of the keys the cluster uses, so each bucket's policy can admit exactly its
# own. Only the ID - the secret half stays in 1Password and OpenBao. Kept in tfvars rather
# than git for the same reason as project_id.
variable "etcd_access_key_id" {
  description = "Access key ID of the S3 key k3s uploads etcd snapshots with (1Password: Hetzner | S3 | prod etcd snapshots, username). Set in terraform.tfvars."
  type        = string

  validation {
    condition     = can(regex("^[A-Z0-9]{20}$", var.etcd_access_key_id))
    error_message = "etcd_access_key_id must be a 20-character Hetzner access key ID."
  }
}

variable "loki_access_key_id" {
  description = "Access key ID of the S3 key Loki writes logs with (1Password: Hetzner | S3 | prod loki, username). Set in terraform.tfvars."
  type        = string

  validation {
    condition     = can(regex("^[A-Z0-9]{20}$", var.loki_access_key_id))
    error_message = "loki_access_key_id must be a 20-character Hetzner access key ID."
  }
}

variable "mariadb_backups_access_key_id" {
  description = "Access key ID of the S3 key the MariaDB operator writes backups with (1Password: Hetzner | S3 | prod mariadb backups, username). Set in terraform.tfvars."
  type        = string

  validation {
    condition     = can(regex("^[A-Z0-9]{20}$", var.mariadb_backups_access_key_id))
    error_message = "mariadb_backups_access_key_id must be a 20-character Hetzner access key ID."
  }
}

variable "postgres_backups_access_key_id" {
  description = "Access key ID of the S3 key CloudNativePG writes backups with (1Password: Hetzner | S3 | prod postgres backups, username). Set in terraform.tfvars."
  type        = string

  validation {
    condition     = can(regex("^[A-Z0-9]{20}$", var.postgres_backups_access_key_id))
    error_message = "postgres_backups_access_key_id must be a 20-character Hetzner access key ID."
  }
}
