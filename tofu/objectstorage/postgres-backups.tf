# Backups of the prod cluster's PostgreSQL (CloudNativePG, for Zitadel): nightly base backups and
# the WAL archived in between, which together allow a restore to any point in time. Not versioned:
# the Barman Cloud plugin writes each file once.
resource "minio_s3_bucket" "prod_postgres_backups" {
  bucket = "d3strukt0r-prod-postgres-backups"
}

# Like the other unversioned buckets, only aborting: the plugin deletes base backups past their
# retention together with the WAL they no longer need.
resource "minio_s3_bucket_lifecycle" "prod_postgres_backups" {
  bucket = minio_s3_bucket.prod_postgres_backups.bucket

  rule {
    id = "abort-incomplete-uploads"

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

# Only the PostgreSQL backup key and the admin key reach this bucket.
resource "minio_s3_bucket_policy" "prod_postgres_backups" {
  bucket = minio_s3_bucket.prod_postgres_backups.bucket
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid          = "OnlyThePostgresBackupAndAdminKeys"
      Effect       = "Deny"
      NotPrincipal = { AWS = [local.admin_principal, local.postgres_backups_principal] }
      Action       = ["s3:*"]
      Resource = [
        "arn:aws:s3:::${minio_s3_bucket.prod_postgres_backups.bucket}",
        "arn:aws:s3:::${minio_s3_bucket.prod_postgres_backups.bucket}/*",
      ]
    }]
  })
}
