# Backups of the prod cluster's MariaDB: daily physical backups and the binary logs archived
# in between, which together allow a restore to any point in time. Not versioned: the operator
# writes each file once.
resource "minio_s3_bucket" "prod_mariadb_backups" {
  bucket = "d3strukt0r-prod-mariadb-backups"
}

# Unlike on the other unversioned buckets, the lifecycle rule also deletes: the operator removes
# physical backups past their retention (30 days) itself, but never the archived binary logs.
# 35 days keeps the binary logs of every backup still kept.
resource "minio_s3_bucket_lifecycle" "prod_mariadb_backups" {
  bucket = minio_s3_bucket.prod_mariadb_backups.bucket

  rule {
    id = "expire-after-35-days"

    expiration {
      days = 35
    }

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

# Only the MariaDB backup key and the admin key reach this bucket.
resource "minio_s3_bucket_policy" "prod_mariadb_backups" {
  bucket = minio_s3_bucket.prod_mariadb_backups.bucket
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid          = "OnlyTheMariaDBBackupAndAdminKeys"
      Effect       = "Deny"
      NotPrincipal = { AWS = [local.admin_principal, local.mariadb_backups_principal] }
      Action       = ["s3:*"]
      Resource = [
        "arn:aws:s3:::${minio_s3_bucket.prod_mariadb_backups.bucket}",
        "arn:aws:s3:::${minio_s3_bucket.prod_mariadb_backups.bucket}/*",
      ]
    }]
  })
}
