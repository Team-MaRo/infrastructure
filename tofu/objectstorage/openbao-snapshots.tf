# OpenBao's Raft snapshots - every secret, policy and auth method, off its one Hetzner volume,
# which Hetzner does not back up. The chart's snapshot agent takes one every 6 hours and uploads
# it here with its own key, delivered through OpenBao itself. The snapshots are encrypted with
# OpenBao's barrier: without the static seal key (1Password) they are unreadable.
#
# The name carries the account prefix because bucket names are unique across all Hetzner
# customers, and it can never be changed.
resource "minio_s3_bucket" "prod_openbao_snapshots" {
  bucket = "d3strukt0r-prod-openbao-snapshots"

  # Only possible at creation, and it switches versioning on permanently.
  object_locking = true

  lifecycle {
    prevent_destroy = true
  }
}

# Every new version is locked for 7 days. GOVERNANCE rather than COMPLIANCE so that the
# admin key can still delete early; the policy below denies that bypass to every other
# key, which a Hetzner key would otherwise hold like every other permission.
resource "minio_s3_bucket_object_lock_configuration" "prod_openbao_snapshots" {
  bucket              = minio_s3_bucket.prod_openbao_snapshots.bucket
  object_lock_enabled = "Enabled"

  rule {
    default_retention {
      mode = "GOVERNANCE"
      days = 7
    }
  }
}

# The agent deletes snapshots older than 30 days itself (Hetzner's lifecycle never expires
# current objects), which on a versioned bucket only adds a delete marker. This removes the
# hidden version 7 days later - never earlier than its lock allows - and then the orphaned
# marker, so storage stays bounded. Parts of an upload that never completed
# go after 7 days, as on every bucket.
resource "minio_s3_bucket_lifecycle" "prod_openbao_snapshots" {
  bucket = minio_s3_bucket.prod_openbao_snapshots.bucket

  rule {
    id = "expire-pruned-snapshots"

    noncurrent_version_expiration {
      noncurrent_days = 7
    }

    expiration {
      expired_object_delete_marker = true
    }

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }
}

# Two layers. No key but the admin's and the snapshot agent's reaches the bucket at all - any
# other key the cluster holds is shut out. The agent's key may upload, read and delete, which
# on this bucket only adds a delete marker, so its own pruning works but a stolen key cannot
# destroy a snapshot. What stays with the admin key alone is everything that could destroy a
# locked version or loosen the rules protecting it.
resource "minio_s3_bucket_policy" "prod_openbao_snapshots" {
  bucket = minio_s3_bucket.prod_openbao_snapshots.bucket
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid          = "OnlyTheSnapshotAndAdminKeys"
        Effect       = "Deny"
        NotPrincipal = { AWS = [local.admin_principal, local.openbao_snapshots_principal] }
        Action       = ["s3:*"]
        Resource     = local.prod_openbao_snapshots_resources
      },
      {
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
        Resource = local.prod_openbao_snapshots_resources
      },
    ]
  })
}

locals {
  # Built from the name rather than the bucket's computed arn, so the whole policy -
  # principals included - is visible in the plan before the first apply.
  prod_openbao_snapshots_resources = [
    "arn:aws:s3:::${minio_s3_bucket.prod_openbao_snapshots.bucket}",
    "arn:aws:s3:::${minio_s3_bucket.prod_openbao_snapshots.bucket}/*",
  ]
}
