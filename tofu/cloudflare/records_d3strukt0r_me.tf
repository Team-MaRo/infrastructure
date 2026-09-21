# DNS records for d3strukt0r.me (personal account).

resource "cloudflare_dns_record" "d3strukt0r_me_cname_wildcard" {
  provider = cloudflare.personal

  content = "d3strukt0r.me"
  name    = "*.d3strukt0r.me"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.me"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3strukt0r_me_cname_apex" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "d3strukt0r.me"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.me"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3strukt0r_me_cname_protonmail2_domainkey" {
  provider = cloudflare.personal

  comment = "ProtonMail (Proxy not supported!)"
  content = "protonmail2.domainkey.dnkpp4rjxgne6fnd22kwhwduuptqsey7jz7hioquggk5hr4ocdnza.domains.proton.ch"
  name    = "protonmail2._domainkey.d3strukt0r.me"
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.me"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3strukt0r_me_cname_protonmail3_domainkey" {
  provider = cloudflare.personal

  comment = "ProtonMail (Proxy not supported!)"
  content = "protonmail3.domainkey.dnkpp4rjxgne6fnd22kwhwduuptqsey7jz7hioquggk5hr4ocdnza.domains.proton.ch"
  name    = "protonmail3._domainkey.d3strukt0r.me"
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.me"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3strukt0r_me_cname_protonmail_domainkey" {
  provider = cloudflare.personal

  comment = "ProtonMail (Proxy not supported!)"
  content = "protonmail.domainkey.dnkpp4rjxgne6fnd22kwhwduuptqsey7jz7hioquggk5hr4ocdnza.domains.proton.ch"
  name    = "protonmail._domainkey.d3strukt0r.me"
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.me"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3strukt0r_me_mx_apex" {
  provider = cloudflare.personal

  comment  = "ProtonMail"
  content  = "mail.protonmail.ch"
  name     = "d3strukt0r.me"
  priority = 10
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "MX"
  zone_id  = local.zone_ids["d3strukt0r.me"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_me_mx_apex_2" {
  provider = cloudflare.personal

  comment  = "ProtonMail"
  content  = "mailsec.protonmail.ch"
  name     = "d3strukt0r.me"
  priority = 20
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "MX"
  zone_id  = local.zone_ids["d3strukt0r.me"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_me_txt_apex" {
  provider = cloudflare.personal

  comment  = "ProtonMail SPF"
  content  = "v=spf1 include:_spf.protonmail.ch mx ~all"
  name     = "d3strukt0r.me"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3strukt0r.me"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_me_txt_apex_2" {
  provider = cloudflare.personal

  comment  = "ProtonMail Verify"
  content  = "protonmail-verification=5a33bc1e9b0ec499fa297b4f067069d0da2fc0d1"
  name     = "d3strukt0r.me"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3strukt0r.me"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_me_txt_dmarc" {
  provider = cloudflare.personal

  comment  = "ProtonMail DMARC"
  content  = "v=DMARC1; p=none"
  name     = "_dmarc.d3strukt0r.me"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3strukt0r.me"]
  settings = {}
}
