terraform {
  required_version = ">= 1.5"

  required_providers {
    hcloud = {
      source  = "hetznercloud/hcloud"
      version = "~> 1.69"
    }
  }

  # Hetzner Object Storage, bucket has Object Lock enabled.
  # Credentials come from AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY.
  #
  # The key is deliberately unchanged from when this was the only module, so moving
  # these files into k3s/ left the state object and every resource address untouched.
  backend "s3" {
    bucket = "d3strukt0r-tfstate"
    key    = "k3s/terraform.tfstate"
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
