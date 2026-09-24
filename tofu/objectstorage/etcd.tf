# Off-node copies of the prod cluster's etcd snapshots. k3s takes them twice a day onto
# each server's own disk - the disk they are meant to protect. Uploads start once the
# cluster has its own key, delivered through OpenBao.
#
# The name carries the account prefix because bucket names are unique across all Hetzner
# customers, and it can never be changed.
resource "minio_s3_bucket" "prod_etcd" {
  bucket = "d3strukt0r-prod-etcd"

  # Only possible at creation, and it switches versioning on permanently.
  object_locking = true

  lifecycle {
    prevent_destroy = true
  }
}

# Every new version is locked for 7 days. GOVERNANCE rather than COMPLIANCE so that the
# admin key can still delete early; the policy below denies that bypass to every other
# key, which a Hetzner key would otherwise hold like every other permission.
resource "minio_s3_bucket_object_lock_configuration" "prod_etcd" {
  bucket              = minio_s3_bucket.prod_etcd.bucket
  object_lock_enabled = "Enabled"

  rule {
    default_retention {
      mode = "GOVERNANCE"
      days = 7
    }
  }
}

# k3s prunes old snapshots itself, which on a versioned bucket only adds a delete marker.
# This removes the hidden version 7 days later - never earlier than its lock allows - and
# then the orphaned marker, so storage stays bounded.
resource "minio_s3_bucket_lifecycle" "prod_etcd" {
  bucket = minio_s3_bucket.prod_etcd.bucket

  rule {
    id = "expire-pruned-snapshots"

    noncurrent_version_expiration {
      noncurrent_days = 7
    }

    expiration {
      expired_object_delete_marker = true
    }
  }
}

# Other keys - the cluster's, once it has one - may upload, read and delete, which on this
# bucket only adds a delete marker, so k3s's own pruning works. What stays with the admin
# key is everything that could destroy a locked version or loosen the rules protecting it.
resource "minio_s3_bucket_policy" "prod_etcd" {
  bucket = minio_s3_bucket.prod_etcd.bucket
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Sid          = "OnlyTheAdminKeyMayLoosenProtection"
      Effect       = "Deny"
      NotPrincipal = { AWS = [local.admin_principal] }
      Action = [
        "s3:BypassGovernanceRetention",
        "s3:PutBucketObjectLockConfiguration",
        "s3:PutLifecycleConfiguration",
        "s3:PutBucketPolicy",
        "s3:DeleteBucketPolicy",
        "s3:DeleteBucket",
      ]
      # Built from the name rather than the bucket's computed arn, so the whole policy -
      # principal included - is visible in the plan before the first apply.
      Resource = [
        "arn:aws:s3:::${minio_s3_bucket.prod_etcd.bucket}",
        "arn:aws:s3:::${minio_s3_bucket.prod_etcd.bucket}/*",
      ]
    }]
  })
}
