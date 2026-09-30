terraform {
  required_version = ">= 1.5"

  required_providers {
    # 3.8.7 is the first release that can import the instance's default login policy.
    zitadel = {
      source  = "zitadel/zitadel"
      version = "~> 3.8, >= 3.8.7"
    }
  }

  backend "s3" {
    bucket = "d3strukt0r-tfstate"
    key    = "zitadel/terraform.tfstate"
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
