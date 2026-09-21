terraform {
  required_version = ">= 1.5"

  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.69"
    }

    # Reads NS delegation straight from the TLD registry.
    dns = {
      source  = "hashicorp/dns"
      version = "~> 3.6"
    }

    # Shells out to dig for DS records, which the dns provider cannot read.
    external = {
      source  = "hashicorp/external"
      version = "~> 2.4"
    }

    # Corrects nameserver delegation. Infomaniak's own provider cannot: its domain
    # surface is only zones and records hosted at Infomaniak.
    terracurl = {
      source  = "devops-rob/terracurl"
      version = "~> 2.11"
    }
  }

  # Hetzner Object Storage, bucket has Object Lock enabled.
  # Credentials come from AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY.
  backend "s3" {
    bucket = "d3strukt0r-tfstate"
    key    = "k3s/terraform.tfstate"
    region = "nbg1"

    endpoints = {
      s3 = "https://nbg1.your-objectstorage.com"
    }

    use_lockfile = true

    # Hetzner Object Storage is S3-compatible but not AWS, so every
    # AWS-specific preflight has to be turned off.
    skip_credentials_validation = true
    skip_region_validation      = true
    skip_requesting_account_id  = true
    skip_metadata_api_check     = true

    # Without this the lock object write fails against Hetzner.
    # See opentofu/opentofu#2605.
    skip_s3_checksum = true
  }
}
