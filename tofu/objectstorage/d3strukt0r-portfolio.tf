# The uploads of d3strukt0r-portfolio (kubernetes/components/d3strukt0r-portfolio): WordPress writes them
# through S3-Uploads with its own key; the site's uploads proxy reads them with a second, read-only
# key and serves them, so the bucket is never public. Versioned: a deleted or overwritten upload
# stays recoverable for 30 days.
resource "minio_s3_bucket" "prod_d3strukt0r_portfolio" {
  bucket = "d3strukt0r-prod-d3strukt0r-portfolio"

  lifecycle {
    prevent_destroy = true
  }
}

resource "minio_s3_bucket_versioning" "prod_d3strukt0r_portfolio" {
  bucket = minio_s3_bucket.prod_d3strukt0r_portfolio.bucket

  versioning_configuration {
    status = "Enabled"
  }
}

resource "minio_s3_bucket_lifecycle" "prod_d3strukt0r_portfolio" {
  bucket = minio_s3_bucket.prod_d3strukt0r_portfolio.bucket

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

resource "minio_s3_bucket_policy" "prod_d3strukt0r_portfolio" {
  bucket = minio_s3_bucket.prod_d3strukt0r_portfolio.bucket
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid          = "OnlyTheAppProxyAndAdminKeys"
        Effect       = "Deny"
        NotPrincipal = { AWS = [local.admin_principal, local.d3strukt0r_portfolio_principal, local.d3strukt0r_portfolio_uploads_proxy_principal] }
        Action       = ["s3:*"]
        Resource     = local.prod_d3strukt0r_portfolio_resources
      },
      {
        # The proxy key only reads.
        Sid          = "OnlyTheAppAndAdminKeysMayWrite"
        Effect       = "Deny"
        NotPrincipal = { AWS = [local.admin_principal, local.d3strukt0r_portfolio_principal] }
        Action = [
          "s3:PutObject",
          "s3:DeleteObject",
          "s3:PutObjectAcl",
          "s3:AbortMultipartUpload",
        ]
        Resource = local.prod_d3strukt0r_portfolio_resources
      },
      {
        Sid          = "OnlyTheAdminKeyMayChangeTheBucketOrDeleteVersions"
        Effect       = "Deny"
        NotPrincipal = { AWS = [local.admin_principal] }
        Action = [
          "s3:DeleteObjectVersion",
          "s3:PutLifecycleConfiguration",
          "s3:PutBucketVersioning",
          "s3:PutBucketPolicy",
          "s3:DeleteBucketPolicy",
          "s3:DeleteBucket",
        ]
        Resource = local.prod_d3strukt0r_portfolio_resources
      },
    ]
  })
}

locals {
  # From the name rather than the computed arn, so the whole policy shows in the first plan.
  prod_d3strukt0r_portfolio_resources = [
    "arn:aws:s3:::${minio_s3_bucket.prod_d3strukt0r_portfolio.bucket}",
    "arn:aws:s3:::${minio_s3_bucket.prod_d3strukt0r_portfolio.bucket}/*",
  ]
}

# What Gatus fetches through the site's uploads proxy (kubernetes/components/gatus): proves that
# signing, the read-only key and the bucket answer. WordPress does not know it, so nothing there
# deletes it.
resource "minio_s3_object" "prod_d3strukt0r_portfolio_healthcheck" {
  bucket_name  = minio_s3_bucket.prod_d3strukt0r_portfolio.bucket
  object_name  = "uploads/healthcheck.txt"
  content      = "ok"
  content_type = "text/plain"
}
