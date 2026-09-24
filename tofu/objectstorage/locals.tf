locals {
  # The key from the [d3strukt0r-hetzner] profile the backends use. The provider cannot
  # read ~/.aws itself, and taking the key from the same file - rather than a copy in
  # tfvars - means the policies below can only ever name the key the backends hold.
  # Expects aws_access_key_id before aws_secret_access_key within the section. Neither
  # value reaches the state: locals and provider configuration are not stored.
  aws_profile = regex(
    "\\[d3strukt0r-hetzner\\][^\\[]*aws_access_key_id\\s*=\\s*(?P<access_key>\\S+)[^\\[]*aws_secret_access_key\\s*=\\s*(?P<secret_key>\\S+)",
    file(pathexpand("~/.aws/credentials")),
  )

  # How Hetzner names a key in a bucket policy. Every key reaches every bucket in the
  # project, so the policies are written as "deny everyone but this key". If this string
  # is wrong, the admin key is "everyone" too - see the staged apply in tofu/README.md.
  admin_principal = "arn:aws:iam:::user/p${var.project_id}:${local.aws_profile.access_key}"
}
