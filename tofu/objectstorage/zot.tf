# The container registry's images (kubernetes/components/zot): our own and the mirrored ones.
# Versioned for 14 days: zot's garbage collection deletes what no image references any more, and
# a bug there (zot#4550 deleted freshly pushed layers) or a mistake must stay undoable - a rebuilt
# image gets a new digest, so whatever pins the old one would not find it. The mirrored images
# would simply be fetched again.
resource "minio_s3_bucket" "prod_zot" {
  bucket = "d3strukt0r-prod-zot"

  lifecycle {
    prevent_destroy = true
  }
}

resource "minio_s3_bucket_versioning" "prod_zot" {
  bucket = minio_s3_bucket.prod_zot.bucket

  versioning_configuration {
    status = "Enabled"
  }
}

resource "minio_s3_bucket_lifecycle" "prod_zot" {
  bucket = minio_s3_bucket.prod_zot.bucket

  rule {
    id = "expire-old-versions"

    noncurrent_version_expiration {
      noncurrent_days = 14
    }

    expiration {
      expired_object_delete_marker = true
    }

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

resource "minio_s3_bucket_policy" "prod_zot" {
  bucket = minio_s3_bucket.prod_zot.bucket
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid          = "OnlyTheAppAndAdminKeys"
        Effect       = "Deny"
        NotPrincipal = { AWS = [local.admin_principal, local.zot_principal] }
        Action       = ["s3:*"]
        Resource     = local.prod_zot_resources
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
        Resource = local.prod_zot_resources
      },
    ]
  })
}

locals {
  # From the name rather than the computed arn, so the whole policy shows in the first plan.
  prod_zot_resources = [
    "arn:aws:s3:::${minio_s3_bucket.prod_zot.bucket}",
    "arn:aws:s3:::${minio_s3_bucket.prod_zot.bucket}/*",
  ]
}
