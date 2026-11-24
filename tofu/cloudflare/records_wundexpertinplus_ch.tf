# DNS records for wundexpertinplus.ch (personal account), a former name of the old site: apex and
# www redirect to https://www.wundexpertinplus.com (kubernetes/components/domain-redirects).

resource "cloudflare_dns_record" "wundexpertinplus_ch_cname_apex" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "wundexpertinplus.ch"
  proxied = true
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["wundexpertinplus.ch"]
}

resource "cloudflare_dns_record" "wundexpertinplus_ch_cname_www" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "www.wundexpertinplus.ch"
  proxied = true
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["wundexpertinplus.ch"]
}

# No mail, in either direction (the domain once carried the old site's mail): a null MX (RFC 7505)
# so nobody tries to deliver to it, and SPF and DMARC that tell receivers to refuse anything sent in
# its name.
resource "cloudflare_dns_record" "wundexpertinplus_ch_mx_apex" {
  provider = cloudflare.personal

  content  = "."
  name     = "wundexpertinplus.ch"
  priority = 0
  ttl      = 1
  type     = "MX"
  zone_id  = local.zone_ids["wundexpertinplus.ch"]
}

resource "cloudflare_dns_record" "wundexpertinplus_ch_txt_apex" {
  provider = cloudflare.personal

  content = "v=spf1 -all"
  name    = "wundexpertinplus.ch"
  ttl     = 1
  type    = "TXT"
  zone_id = local.zone_ids["wundexpertinplus.ch"]
}

resource "cloudflare_dns_record" "wundexpertinplus_ch_txt_dmarc" {
  provider = cloudflare.personal

  content = "v=DMARC1; p=reject"
  name    = "_dmarc.wundexpertinplus.ch"
  ttl     = 1
  type    = "TXT"
  zone_id = local.zone_ids["wundexpertinplus.ch"]
}
