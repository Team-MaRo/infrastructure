# Adoption of the hand-created state bucket. Kept in version control instead of using
# `tofu import`, so the adoption itself is reviewable.

import {
  to = minio_s3_bucket.tfstate
  id = "d3strukt0r-tfstate"
}
