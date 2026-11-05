# The last good signatures of the cluster's clamd (kubernetes/components/clamav): its pod uploads
# them after each update and copies them back at every start, so a start does not depend on
# ClamAV's mirror, which blocks an address that downloads too often. Not versioned: the copy is
# replaced as a whole, and an older one is worth nothing.
resource "minio_s3_bucket" "prod_clamav" {
  bucket = "d3strukt0r-prod-clamav"

  lifecycle {
    prevent_destroy = true
  }
}

resource "minio_s3_bucket_lifecycle" "prod_clamav" {
  bucket = minio_s3_bucket.prod_clamav.bucket

  rule {
    id = "abort-incomplete-uploads"

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

resource "minio_s3_bucket_policy" "prod_clamav" {
  bucket = minio_s3_bucket.prod_clamav.bucket
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid          = "OnlyTheAppAndAdminKeys"
        Effect       = "Deny"
        NotPrincipal = { AWS = [local.admin_principal, local.clamav_principal] }
        Action       = ["s3:*"]
        Resource     = local.prod_clamav_resources
      },
      {
        Sid          = "OnlyTheAdminKeyMayChangeTheBucket"
        Effect       = "Deny"
        NotPrincipal = { AWS = [local.admin_principal] }
        Action = [
          "s3:PutLifecycleConfiguration",
          "s3:PutBucketVersioning",
          "s3:PutBucketPolicy",
          "s3:DeleteBucketPolicy",
          "s3:DeleteBucket",
        ]
        Resource = local.prod_clamav_resources
      },
    ]
  })
}

locals {
  # From the name rather than the computed arn, so the whole policy shows in the first plan.
  prod_clamav_resources = [
    "arn:aws:s3:::${minio_s3_bucket.prod_clamav.bucket}",
    "arn:aws:s3:::${minio_s3_bucket.prod_clamav.bucket}/*",
  ]
}
