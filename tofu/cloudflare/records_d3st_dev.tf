# DNS records for d3st.dev (personal account).

resource "cloudflare_dns_record" "d3st_dev_cname_wildcard" {
  provider = cloudflare.personal

  content = "d3st.dev"
  name    = "*.d3st.dev"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3st.dev"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3st_dev_cname_apex" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "d3st.dev"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3st.dev"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3st_dev_cname_dk1_domainkey" {
  provider = cloudflare.personal

  comment = "AnonAddy (Proxy not supported!)"
  content = "dk1._domainkey.anonaddy.me"
  name    = "dk1._domainkey.d3st.dev"
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3st.dev"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3st_dev_cname_dk2_domainkey" {
  provider = cloudflare.personal

  comment = "AnonAddy (Proxy not supported!)"
  content = "dk2._domainkey.anonaddy.me"
  name    = "dk2._domainkey.d3st.dev"
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3st.dev"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3st_dev_mx_apex" {
  provider = cloudflare.personal

  comment  = "AnonAddy"
  content  = "mail2.anonaddy.me"
  name     = "d3st.dev"
  priority = 20
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "MX"
  zone_id  = local.zone_ids["d3st.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3st_dev_mx_apex_2" {
  provider = cloudflare.personal

  comment  = "AnonAddy"
  content  = "mail.anonaddy.me"
  name     = "d3st.dev"
  priority = 10
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "MX"
  zone_id  = local.zone_ids["d3st.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3st_dev_txt_apex" {
  provider = cloudflare.personal

  comment  = "Have I Been Pwned Verify"
  content  = "\"have-i-been-pwned-verification=dweb_a5z0qvor1rjdwwtq981b19og\""
  name     = "d3st.dev"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3st.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3st_dev_txt_apex_2" {
  provider = cloudflare.personal

  comment  = "AnonAddy Verify"
  content  = "\"aa-verify=87b4e4ca1e1631bbd7c005871f9af76c91ac4c17\""
  name     = "d3st.dev"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3st.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3st_dev_txt_apex_3" {
  provider = cloudflare.personal

  comment  = "AnonAddy SPF"
  content  = "\"v=spf1 include:spf.anonaddy.me -all\""
  name     = "d3st.dev"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3st.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3st_dev_txt_apex_4" {
  provider = cloudflare.personal

  comment  = "box.com Verify"
  content  = "\"box-domain-verification=3470b4078aa49daffbd25ad6dfa6088cf90b2740804c2b514915b619a848c1b0\""
  name     = "d3st.dev"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3st.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3st_dev_txt_dmarc" {
  provider = cloudflare.personal

  comment  = "AnonAddy DMARC"
  content  = "\"v=DMARC1; p=quarantine; adkim=s\""
  name     = "_dmarc.d3st.dev"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3st.dev"]
  settings = {}
}
