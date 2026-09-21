terraform {
  required_version = ">= 1.5"

  required_providers {
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

  # Own state, so a lapsed domain or a DNS resolution failure cannot break a plan for
  # the cluster. Data sources error rather than warn, which is exactly what used to
  # couple the two.
  backend "s3" {
    bucket = "d3strukt0r-tfstate"
    key    = "infomaniak/terraform.tfstate"
    region = "nbg1"

    # Credentials come from the [d3strukt0r-hetzner] profile in ~/.aws/credentials,
    # not from AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY. Backend blocks cannot
    # interpolate, so a literal profile name is the only way to keep these out of the
    # environment. The name is account-scoped because profiles are global to ~/.aws.
    profile = "d3strukt0r-hetzner"

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
