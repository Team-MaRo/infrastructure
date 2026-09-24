# s3_compat_mode stays off. It makes the provider swallow "not implemented" errors, object
# lock and lifecycle reads included, so an unsupported setting would look configured while
# protecting nothing. Without it, anything Hetzner rejects fails the apply loudly.
provider "minio" {
  minio_server   = "nbg1.your-objectstorage.com"
  minio_region   = "nbg1"
  minio_ssl      = true
  minio_user     = local.aws_profile.access_key
  minio_password = local.aws_profile.secret_key
}
