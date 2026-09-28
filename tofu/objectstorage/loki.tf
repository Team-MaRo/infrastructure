# Log storage for the prod cluster's Loki. Not versioned and not locked: logs are
# disposable, and Loki's compactor deletes whatever is older than its retention itself, so
# the lifecycle rule only clears parts of uploads that never completed.
resource "minio_s3_bucket" "prod_loki" {
  bucket = "d3strukt0r-prod-loki"
}

resource "minio_s3_bucket_lifecycle" "prod_loki" {
  bucket = minio_s3_bucket.prod_loki.bucket

  rule {
    id = "abort-incomplete-uploads"

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

# Only Loki's own key and the admin key reach this bucket. It was the proving ground for a
# NotPrincipal naming two keys, before the etcd policy used the same form: the admin key kept
# full access, and the etcd key got AccessDenied here.
resource "minio_s3_bucket_policy" "prod_loki" {
  bucket = minio_s3_bucket.prod_loki.bucket
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid          = "OnlyLokiAndTheAdminKey"
      Effect       = "Deny"
      NotPrincipal = { AWS = [local.admin_principal, local.loki_principal] }
      Action       = ["s3:*"]
      Resource = [
        "arn:aws:s3:::${minio_s3_bucket.prod_loki.bucket}",
        "arn:aws:s3:::${minio_s3_bucket.prod_loki.bucket}/*",
      ]
    }]
  })
}
