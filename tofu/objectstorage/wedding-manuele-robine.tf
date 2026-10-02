# The media of wedding-manuele-robine (the API stores uploads here through flysystem; private,
# served by the API itself). The only copy of the guests' uploads, so versioned: a deleted or
# overwritten file stays recoverable for 30 days, then the lifecycle rule removes the old version
# and, after that, its orphaned delete marker.
resource "minio_s3_bucket" "prod_wedding_manuele_robine" {
  bucket = "d3strukt0r-prod-wedding-manuele-robine"

  lifecycle {
    prevent_destroy = true
  }
}

resource "minio_s3_bucket_versioning" "prod_wedding_manuele_robine" {
  bucket = minio_s3_bucket.prod_wedding_manuele_robine.bucket

  versioning_configuration {
    status = "Enabled"
  }
}

resource "minio_s3_bucket_lifecycle" "prod_wedding_manuele_robine" {
  bucket = minio_s3_bucket.prod_wedding_manuele_robine.bucket

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

# Only the app's key and the admin key reach this bucket.
resource "minio_s3_bucket_policy" "prod_wedding_manuele_robine" {
  bucket = minio_s3_bucket.prod_wedding_manuele_robine.bucket
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid          = "OnlyTheAppAndTheAdminKey"
      Effect       = "Deny"
      NotPrincipal = { AWS = [local.admin_principal, local.wedding_manuele_robine_principal] }
      Action       = ["s3:*"]
      Resource = [
        "arn:aws:s3:::${minio_s3_bucket.prod_wedding_manuele_robine.bucket}",
        "arn:aws:s3:::${minio_s3_bucket.prod_wedding_manuele_robine.bucket}/*",
      ]
    }]
  })
}
