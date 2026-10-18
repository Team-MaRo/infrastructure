# Zitadel's mail - account invitations, email verification, password resets, OTP codes - leaves
# through mailomat, the smarthost for the cluster's mail (tofu/cloudflare/README.md, "Sending
# through mailomat"). The sender is auth@d3strukt0r.dev; mailomat signs as and bounces to
# mailomat.d3strukt0r.dev, which d3strukt0r.dev's relaxed DMARC record counts for the domain.
# The password is write-only: the state keeps only a hash that notices a new one.
resource "zitadel_email_provider_smtp" "mailomat" {
  description = "mailomat"
  host        = "smtp.mailomat.cloud:587"
  # Zitadel tries TLS from the first byte and, when the server answers in plain text as on 587,
  # falls back to STARTTLS with the certificate checked; false would send the login unencrypted.
  tls            = true
  user           = "zitadel@mailomat.d3strukt0r.dev"
  password       = var.smtp_password
  sender_address = "auth@d3strukt0r.dev"
  sender_name    = "d3strukt0r Login"
  set_active     = true
}
