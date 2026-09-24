terraform {
  required_version = ">= 1.5"

  required_providers {
    # What Hetzner's own Object Storage docs use. The aws provider reads every bucket
    # through calls Hetzner does not implement (Accelerate, Website, Logging, Replication,
    # Tagging, RequestPayment), so it cannot even refresh one.
    minio = {
      source  = "aminueza/minio"
      version = "~> 3.43"
    }
  }

  # Its own state, like the other modules - and in the bucket whose policy it manages.
  # That is why the policy allows exactly the key this backend and the provider both use.
  backend "s3" {
    bucket = "d3strukt0r-tfstate"
    key    = "objectstorage/terraform.tfstate"
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
