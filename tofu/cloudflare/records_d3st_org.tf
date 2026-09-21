# DNS records for d3st.org (personal account).

resource "cloudflare_dns_record" "d3st_org_cname_wildcard" {
  provider = cloudflare.personal

  content = "d3st.org"
  name    = "*.d3st.org"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3st.org"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3st_org_cname_apex" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "d3st.org"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3st.org"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3st_org_cname_dk1_domainkey" {
  provider = cloudflare.personal

  comment = "AnonAddy (Proxy not supported!)"
  content = "dk1._domainkey.anonaddy.me"
  name    = "dk1._domainkey.d3st.org"
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3st.org"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3st_org_cname_dk2_domainkey" {
  provider = cloudflare.personal

  comment = "AnonAddy (Proxy not supported!)"
  content = "dk2._domainkey.anonaddy.me"
  name    = "dk2._domainkey.d3st.org"
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3st.org"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3st_org_mx_apex" {
  provider = cloudflare.personal

  comment  = "AnonAddy"
  content  = "mail2.anonaddy.me"
  name     = "d3st.org"
  priority = 20
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "MX"
  zone_id  = local.zone_ids["d3st.org"]
  settings = {}
}

resource "cloudflare_dns_record" "d3st_org_mx_apex_2" {
  provider = cloudflare.personal

  comment  = "AnonAddy"
  content  = "mail.anonaddy.me"
  name     = "d3st.org"
  priority = 10
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "MX"
  zone_id  = local.zone_ids["d3st.org"]
  settings = {}
}

resource "cloudflare_dns_record" "d3st_org_txt_apex" {
  provider = cloudflare.personal

  comment  = "Have I Been Pwned Verify"
  content  = "\"have-i-been-pwned-verification=dweb_2wmj0l4ncrfeiapkyo1pj6e2\""
  name     = "d3st.org"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3st.org"]
  settings = {}
}

resource "cloudflare_dns_record" "d3st_org_txt_apex_2" {
  provider = cloudflare.personal

  comment  = "AnonAddy SPF"
  content  = "v=spf1 include:spf.anonaddy.me -all"
  name     = "d3st.org"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3st.org"]
  settings = {}
}

resource "cloudflare_dns_record" "d3st_org_txt_apex_3" {
  provider = cloudflare.personal

  comment  = "AnonAddy Verify"
  content  = "aa-verify=2ccd78c27a71efbd432a4a83f2e9ec31a0b3d526"
  name     = "d3st.org"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3st.org"]
  settings = {}
}

resource "cloudflare_dns_record" "d3st_org_txt_dmarc" {
  provider = cloudflare.personal

  comment  = "AnonAddy DMARC"
  content  = "v=DMARC1; p=quarantine; adkim=s"
  name     = "_dmarc.d3st.org"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3st.org"]
  settings = {}
}
