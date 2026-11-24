# DNS records for wundexpertin-bl.ch (personal account), a former name of the old site: apex and
# www redirect to https://www.wundexpertinplus.com (kubernetes/components/domain-redirects).

resource "cloudflare_dns_record" "wundexpertin_bl_ch_cname_apex" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "wundexpertin-bl.ch"
  proxied = true
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["wundexpertin-bl.ch"]
}

resource "cloudflare_dns_record" "wundexpertin_bl_ch_cname_www" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "www.wundexpertin-bl.ch"
  proxied = true
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["wundexpertin-bl.ch"]
}

# No mail, in either direction (the domain once carried the old site's mail): a null MX (RFC 7505)
# so nobody tries to deliver to it, and SPF and DMARC that tell receivers to refuse anything sent in
# its name.
resource "cloudflare_dns_record" "wundexpertin_bl_ch_mx_apex" {
  provider = cloudflare.personal

  content  = "."
  name     = "wundexpertin-bl.ch"
  priority = 0
  ttl      = 1
  type     = "MX"
  zone_id  = local.zone_ids["wundexpertin-bl.ch"]
}

resource "cloudflare_dns_record" "wundexpertin_bl_ch_txt_apex" {
  provider = cloudflare.personal

  content = "v=spf1 -all"
  name    = "wundexpertin-bl.ch"
  ttl     = 1
  type    = "TXT"
  zone_id = local.zone_ids["wundexpertin-bl.ch"]
}

resource "cloudflare_dns_record" "wundexpertin_bl_ch_txt_dmarc" {
  provider = cloudflare.personal

  content = "v=DMARC1; p=reject"
  name    = "_dmarc.wundexpertin-bl.ch"
  ttl     = 1
  type    = "TXT"
  zone_id = local.zone_ids["wundexpertin-bl.ch"]
}
