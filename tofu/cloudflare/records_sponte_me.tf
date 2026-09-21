# DNS records for sponte.me (personal account).

resource "cloudflare_dns_record" "sponte_me_txt_dmarc" {
  provider = cloudflare.personal

  content  = "\"v=DMARC1; p=reject;\""
  name     = "_dmarc.sponte.me"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["sponte.me"]
  settings = {}
}

resource "cloudflare_dns_record" "sponte_me_txt_apex" {
  provider = cloudflare.personal

  content  = "\"v=spf1 -all\""
  name     = "sponte.me"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["sponte.me"]
  settings = {}
}
