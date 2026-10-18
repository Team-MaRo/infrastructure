# Trace storage for the prod cluster's Tempo. Not versioned and not locked: traces are
# disposable, and Tempo's compactor deletes whatever is older than its retention itself, so
# the lifecycle rule only clears parts of uploads that never completed.
resource "minio_s3_bucket" "prod_tempo" {
  bucket = "d3strukt0r-prod-tempo"
}

resource "minio_s3_bucket_lifecycle" "prod_tempo" {
  bucket = minio_s3_bucket.prod_tempo.bucket

  rule {
    id = "abort-incomplete-uploads"

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

# Only Tempo's own key and the admin key reach this bucket.
resource "minio_s3_bucket_policy" "prod_tempo" {
  bucket = minio_s3_bucket.prod_tempo.bucket
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid          = "OnlyTempoAndTheAdminKey"
      Effect       = "Deny"
      NotPrincipal = { AWS = [local.admin_principal, local.tempo_principal] }
      Action       = ["s3:*"]
      Resource = [
        "arn:aws:s3:::${minio_s3_bucket.prod_tempo.bucket}",
        "arn:aws:s3:::${minio_s3_bucket.prod_tempo.bucket}/*",
      ]
    }]
  })
}
