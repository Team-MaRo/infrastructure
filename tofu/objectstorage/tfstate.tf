# The state of every module lives here - this module's own included - and every Hetzner
# key reaches every bucket, so without the policy below any key handed to the cluster could
# read or rewrite it.
#
# Created by hand before this repo existed and adopted through imports.tf. The name has no
# cluster in it on purpose: this bucket belongs to the account, and a second cluster would
# add a state key here, not a second bucket.
resource "minio_s3_bucket" "tfstate" {
  bucket = "d3strukt0r-tfstate"

  # Enabled when the bucket was created by hand; it cannot be switched off.
  object_locking = true

  # Never set acl here: an acl change clears the bucket policy, and the provider already
  # reads the custom policy below as the default "private".

  lifecycle {
    prevent_destroy = true
  }
}

# This denies the admin key nothing only as long as admin_principal is right. It was proven
# on the etcd bucket with this same statement before being applied here; see the staged
# apply in tofu/README.md before changing how the principal is built.
resource "minio_s3_bucket_policy" "tfstate" {
  bucket = minio_s3_bucket.tfstate.bucket
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid          = "OnlyTheAdminKey"
      Effect       = "Deny"
      NotPrincipal = { AWS = [local.admin_principal] }
      Action       = ["s3:*"]
      Resource = [
        "arn:aws:s3:::${minio_s3_bucket.tfstate.bucket}",
        "arn:aws:s3:::${minio_s3_bucket.tfstate.bucket}/*",
      ]
    }]
  })
}
