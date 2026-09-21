# DNS records for rubyn.li (personal account).

resource "cloudflare_dns_record" "rubyn_li_cname_dk1_domainkey" {
  provider = cloudflare.personal

  content = "dk1._domainkey.anonaddy.me"
  name    = "dk1._domainkey.rubyn.li"
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["rubyn.li"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "rubyn_li_cname_dk2_domainkey" {
  provider = cloudflare.personal

  content = "dk2._domainkey.anonaddy.me"
  name    = "dk2._domainkey.rubyn.li"
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["rubyn.li"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "rubyn_li_mx_apex" {
  provider = cloudflare.personal

  content  = "mail2.anonaddy.me"
  name     = "rubyn.li"
  priority = 20
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "MX"
  zone_id  = local.zone_ids["rubyn.li"]
  settings = {}
}

resource "cloudflare_dns_record" "rubyn_li_mx_apex_2" {
  provider = cloudflare.personal

  content  = "mail.anonaddy.me"
  name     = "rubyn.li"
  priority = 10
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "MX"
  zone_id  = local.zone_ids["rubyn.li"]
  settings = {}
}

resource "cloudflare_dns_record" "rubyn_li_txt_dmarc" {
  provider = cloudflare.personal

  content  = "\"v=DMARC1; p=quarantine; adkim=s\""
  name     = "_dmarc.rubyn.li"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["rubyn.li"]
  settings = {}
}

resource "cloudflare_dns_record" "rubyn_li_txt_apex" {
  provider = cloudflare.personal

  content  = "\"v=spf1 include:spf.anonaddy.me -all\""
  name     = "rubyn.li"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["rubyn.li"]
  settings = {}
}

resource "cloudflare_dns_record" "rubyn_li_txt_apex_2" {
  provider = cloudflare.personal

  comment  = "AnonAddy Verify"
  content  = "\"aa-verify=02d53c25c95f7b99f771693efe1d88cf738cedb7\""
  name     = "rubyn.li"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["rubyn.li"]
  settings = {}
}
