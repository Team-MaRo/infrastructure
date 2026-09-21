# DNS records for robines.space (personal account).

resource "cloudflare_dns_record" "robines_space_cname_dk1_domainkey" {
  provider = cloudflare.personal

  comment = "AnonAddy (Proxy not supported!)"
  content = "dk1._domainkey.anonaddy.me"
  name    = "dk1._domainkey.robines.space"
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["robines.space"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "robines_space_cname_dk2_domainkey" {
  provider = cloudflare.personal

  comment = "AnonAddy (Proxy not supported!)"
  content = "dk2._domainkey.anonaddy.me"
  name    = "dk2._domainkey.robines.space"
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["robines.space"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "robines_space_cname_old" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "old.robines.space"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["robines.space"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "robines_space_cname_wildcard" {
  provider = cloudflare.personal

  content = "robines.space"
  name    = "*.robines.space"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["robines.space"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "robines_space_mx_apex" {
  provider = cloudflare.personal

  comment  = "AnonAddy"
  content  = "mail2.anonaddy.me"
  name     = "robines.space"
  priority = 20
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "MX"
  zone_id  = local.zone_ids["robines.space"]
  settings = {}
}

resource "cloudflare_dns_record" "robines_space_mx_apex_2" {
  provider = cloudflare.personal

  comment  = "AnonAddy"
  content  = "mail.anonaddy.me"
  name     = "robines.space"
  priority = 10
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "MX"
  zone_id  = local.zone_ids["robines.space"]
  settings = {}
}

resource "cloudflare_dns_record" "robines_space_txt_dmarc" {
  provider = cloudflare.personal

  comment  = "AnonAddy DMARC"
  content  = "\"v=DMARC1; p=quarantine; adkim=s\""
  name     = "_dmarc.robines.space"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["robines.space"]
  settings = {}
}

resource "cloudflare_dns_record" "robines_space_txt_apex" {
  provider = cloudflare.personal

  comment  = "Have i been pwned"
  content  = "\"hibp-verify=dweb_o8whz76xdp1qvmz8qzpo7mr0\""
  name     = "robines.space"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["robines.space"]
  settings = {}
}

resource "cloudflare_dns_record" "robines_space_txt_apex_2" {
  provider = cloudflare.personal

  comment  = "AnonAddy SPF"
  content  = "\"v=spf1 include:spf.anonaddy.me -all\""
  name     = "robines.space"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["robines.space"]
  settings = {}
}

resource "cloudflare_dns_record" "robines_space_txt_apex_3" {
  provider = cloudflare.personal

  comment  = "AnonAddy Verify"
  content  = "\"aa-verify=16ba508bade4a3c12adb31c79ec44837f2cd07d5\""
  name     = "robines.space"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["robines.space"]
  settings = {}
}

resource "cloudflare_dns_record" "robines_space_aaaa_apex" {
  provider = cloudflare.personal

  content  = "100::"
  name     = "robines.space"
  proxied  = true
  tags     = []
  ttl      = 1
  type     = "AAAA"
  zone_id  = local.zone_ids["robines.space"]
  settings = {}
}

resource "cloudflare_dns_record" "robines_space_aaaa_www" {
  provider = cloudflare.personal

  content  = "100::"
  name     = "www.robines.space"
  proxied  = true
  tags     = []
  ttl      = 1
  type     = "AAAA"
  zone_id  = local.zone_ids["robines.space"]
  settings = {}
}
