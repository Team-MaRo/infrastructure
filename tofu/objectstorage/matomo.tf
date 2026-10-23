# Matomo's files that must outlive its pod (kubernetes/components/matomo): today the DB-IP city
# database for visitor locations, which a monthly job uploads and every pod start copies into
# Matomo's misc/. Versioned: a broken upload can be rolled back for 30 days.
resource "minio_s3_bucket" "prod_matomo" {
  bucket = "d3strukt0r-prod-matomo"

  lifecycle {
    prevent_destroy = true
  }
}

resource "minio_s3_bucket_versioning" "prod_matomo" {
  bucket = minio_s3_bucket.prod_matomo.bucket

  versioning_configuration {
    status = "Enabled"
  }
}

resource "minio_s3_bucket_lifecycle" "prod_matomo" {
  bucket = minio_s3_bucket.prod_matomo.bucket

  rule {
    id = "expire-old-versions"

    noncurrent_version_expiration {
      noncurrent_days = 30
    }

    expiration {
      expired_object_delete_marker = true
    }

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

resource "minio_s3_bucket_policy" "prod_matomo" {
  bucket = minio_s3_bucket.prod_matomo.bucket
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid          = "OnlyTheAppAndAdminKeys"
        Effect       = "Deny"
        NotPrincipal = { AWS = [local.admin_principal, local.matomo_principal] }
        Action       = ["s3:*"]
        Resource     = local.prod_matomo_resources
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
        Resource = local.prod_matomo_resources
      },
    ]
  })
}

locals {
  # From the name rather than the computed arn, so the whole policy shows in the first plan.
  prod_matomo_resources = [
    "arn:aws:s3:::${minio_s3_bucket.prod_matomo.bucket}",
    "arn:aws:s3:::${minio_s3_bucket.prod_matomo.bucket}/*",
  ]
}
