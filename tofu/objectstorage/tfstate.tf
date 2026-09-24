# The state of every module lives in d3strukt0r-tfstate, and every Hetzner key reaches
# every bucket - so without this, any key handed to the cluster could read or rewrite it.
#
# Only the policy is managed. The bucket was created by hand and holds OpenTofu's own state,
# so it is deliberately not imported: nothing here can ever plan to destroy it.
#
# This denies the admin key nothing only as long as admin_principal is right. It was proven
# on the etcd bucket with this same statement before being applied here; see the staged
# apply in tofu/README.md before changing how the principal is built.
resource "minio_s3_bucket_policy" "tfstate" {
  bucket = "d3strukt0r-tfstate"
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid          = "OnlyTheAdminKey"
      Effect       = "Deny"
      NotPrincipal = { AWS = [local.admin_principal] }
      Action       = ["s3:*"]
      Resource = [
        "arn:aws:s3:::d3strukt0r-tfstate",
        "arn:aws:s3:::d3strukt0r-tfstate/*",
      ]
    }]
  })
}
