# DNS records for manuele-robine.wedding (personal account).

resource "cloudflare_dns_record" "manuele_robine_wedding_cname_wildcard" {
  provider = cloudflare.personal

  content = "manuele-robine.wedding"
  name    = "*.manuele-robine.wedding"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["manuele-robine.wedding"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "manuele_robine_wedding_cname_apex" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "manuele-robine.wedding"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["manuele-robine.wedding"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "manuele_robine_wedding_txt_apex" {
  provider = cloudflare.personal

  comment  = "Brave Creators"
  content  = "brave-ledger-verification=da7c817db9cea16e297a30e5b95940e4cd4a8f780c8272bbdfc50f73fd426bfd"
  name     = "manuele-robine.wedding"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["manuele-robine.wedding"]
  settings = {}
}
