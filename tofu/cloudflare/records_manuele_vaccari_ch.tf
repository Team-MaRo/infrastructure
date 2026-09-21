# DNS records for manuele-vaccari.ch (personal account).

resource "cloudflare_dns_record" "manuele_vaccari_ch_cname_wildcard" {
  provider = cloudflare.personal

  content = "manuele-vaccari.ch"
  name    = "*.manuele-vaccari.ch"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["manuele-vaccari.ch"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "manuele_vaccari_ch_cname_apex" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "manuele-vaccari.ch"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["manuele-vaccari.ch"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "manuele_vaccari_ch_cname_webcenter_text_export" {
  provider = cloudflare.personal

  content = "d3strukt0r.github.io"
  name    = "webcenter-text-export.manuele-vaccari.ch"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["manuele-vaccari.ch"]
  settings = {
    flatten_cname = false
  }
}
