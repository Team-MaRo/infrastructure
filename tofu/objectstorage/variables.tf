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
# 1Password "Hetzner | S3 | prod etcd snapshots"
#   https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=w2po7jqmxren7y7eruj73p5w4a&h=my.1password.com
variable "etcd_access_key_id" {
  description = "Access key ID of the S3 key k3s uploads etcd snapshots with (1Password: Hetzner | S3 | prod etcd snapshots, username). Set in terraform.tfvars."
  type        = string

  validation {
    condition     = can(regex("^[A-Z0-9]{20}$", var.etcd_access_key_id))
    error_message = "etcd_access_key_id must be a 20-character Hetzner access key ID."
  }
}

# 1Password "Hetzner | S3 | prod loki"
#   https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=obsu3brgoh5l3yxawnbbvq4wi4&h=my.1password.com
variable "loki_access_key_id" {
  description = "Access key ID of the S3 key Loki writes logs with (1Password: Hetzner | S3 | prod loki, username). Set in terraform.tfvars."
  type        = string

  validation {
    condition     = can(regex("^[A-Z0-9]{20}$", var.loki_access_key_id))
    error_message = "loki_access_key_id must be a 20-character Hetzner access key ID."
  }
}

# 1Password "Hetzner | S3 | prod mariadb backups"
#   https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=qttplkz3cky6llsjgw7zhww5bu&h=my.1password.com
variable "mariadb_backups_access_key_id" {
  description = "Access key ID of the S3 key the MariaDB operator writes backups with (1Password: Hetzner | S3 | prod mariadb backups, username). Set in terraform.tfvars."
  type        = string

  validation {
    condition     = can(regex("^[A-Z0-9]{20}$", var.mariadb_backups_access_key_id))
    error_message = "mariadb_backups_access_key_id must be a 20-character Hetzner access key ID."
  }
}

# 1Password "Hetzner | S3 | prod postgres backups"
#   https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=xwoxypf5ibmqtcfl7gwjuxx4re&h=my.1password.com
variable "postgres_backups_access_key_id" {
  description = "Access key ID of the S3 key CloudNativePG writes backups with (1Password: Hetzner | S3 | prod postgres backups, username). Set in terraform.tfvars."
  type        = string

  validation {
    condition     = can(regex("^[A-Z0-9]{20}$", var.postgres_backups_access_key_id))
    error_message = "postgres_backups_access_key_id must be a 20-character Hetzner access key ID."
  }
}

# 1Password "Hetzner | S3 | prod openbao snapshots"
#   https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=7wzbdy35bhtgqn6ahbqyoaiuum&h=my.1password.com
variable "openbao_snapshots_access_key_id" {
  description = "Access key ID of the S3 key OpenBao's snapshot agent uploads with (1Password: Hetzner | S3 | prod openbao snapshots, username). Set in terraform.tfvars."
  type        = string

  validation {
    condition     = can(regex("^[A-Z0-9]{20}$", var.openbao_snapshots_access_key_id))
    error_message = "openbao_snapshots_access_key_id must be a 20-character Hetzner access key ID."
  }
}

# 1Password "Hetzner | S3 | prod wedding-manuele-robine"
#   https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=ytvt5yczmlghqtf7ki6orppn6e&h=my.1password.com
variable "wedding_manuele_robine_access_key_id" {
  description = "Access key ID of the S3 key wedding-manuele-robine stores its media with (1Password: Hetzner | S3 | prod wedding-manuele-robine, username). Set in terraform.tfvars."
  type        = string

  validation {
    condition     = can(regex("^[A-Z0-9]{20}$", var.wedding_manuele_robine_access_key_id))
    error_message = "wedding_manuele_robine_access_key_id must be a 20-character Hetzner access key ID."
  }
}

# 1Password "Hetzner | S3 | prod robines-portfolio"
#   https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=j3phtimnb5zbwonzf655meqrb4&h=my.1password.com
variable "robines_portfolio_access_key_id" {
  description = "Access key ID of the S3 key robines-portfolio's WordPress stores its uploads with (1Password: Hetzner | S3 | prod robines-portfolio, username). Set in terraform.tfvars."
  type        = string

  validation {
    condition     = can(regex("^[A-Z0-9]{20}$", var.robines_portfolio_access_key_id))
    error_message = "robines_portfolio_access_key_id must be a 20-character Hetzner access key ID."
  }
}

# 1Password "Hetzner | S3 | prod robines-portfolio uploads-proxy"
#   https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=3icb7ypn2ub7bk7x2yzbwhkrfm&h=my.1password.com
variable "robines_portfolio_uploads_proxy_access_key_id" {
  description = "Access key ID of the read-only S3 key robines-portfolio's uploads proxy serves the uploads with (1Password: Hetzner | S3 | prod robines-portfolio uploads-proxy, username). Set in terraform.tfvars."
  type        = string

  validation {
    condition     = can(regex("^[A-Z0-9]{20}$", var.robines_portfolio_uploads_proxy_access_key_id))
    error_message = "robines_portfolio_uploads_proxy_access_key_id must be a 20-character Hetzner access key ID."
  }
}

# 1Password "Hetzner | S3 | prod arepazo"
#   https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=t6onf7inzynbh6oqvdggcrdzoy&h=my.1password.com
variable "arepazo_access_key_id" {
  description = "Access key ID of the S3 key arepazo's WordPress stores its uploads with (1Password: Hetzner | S3 | prod arepazo, username). Set in terraform.tfvars."
  type        = string

  validation {
    condition     = can(regex("^[A-Z0-9]{20}$", var.arepazo_access_key_id))
    error_message = "arepazo_access_key_id must be a 20-character Hetzner access key ID."
  }
}

# 1Password "Hetzner | S3 | prod arepazo uploads-proxy"
#   https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=gs5mn5w3sfbqsljokq3ukhjtjy&h=my.1password.com
variable "arepazo_uploads_proxy_access_key_id" {
  description = "Access key ID of the read-only S3 key arepazo's uploads proxy serves the uploads with (1Password: Hetzner | S3 | prod arepazo uploads-proxy, username). Set in terraform.tfvars."
  type        = string

  validation {
    condition     = can(regex("^[A-Z0-9]{20}$", var.arepazo_uploads_proxy_access_key_id))
    error_message = "arepazo_uploads_proxy_access_key_id must be a 20-character Hetzner access key ID."
  }
}
