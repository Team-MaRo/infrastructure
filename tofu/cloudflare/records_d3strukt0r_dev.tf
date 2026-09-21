# DNS records for d3strukt0r.dev (personal account).

resource "cloudflare_dns_record" "d3strukt0r_dev_a_prod" {
  provider = cloudflare.personal

  content  = "161.35.16.9"
  name     = "prod.d3strukt0r.dev"
  proxied  = true
  tags     = []
  ttl      = 1
  type     = "A"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_dev_aaaa_prod" {
  provider = cloudflare.personal

  content  = "2a03:b0c0:3:d0::f65:2001"
  name     = "prod.d3strukt0r.dev"
  proxied  = true
  tags     = []
  ttl      = 1
  type     = "AAAA"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_dev_cname_wildcard" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "*.d3strukt0r.dev"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.dev"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3strukt0r_dev_cname_dk1_domainkey" {
  provider = cloudflare.personal

  comment = "AnonAddy (Proxy not supported!)"
  content = "dk1._domainkey.anonaddy.me"
  name    = "dk1._domainkey.d3strukt0r.dev"
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.dev"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3strukt0r_dev_cname_dk2_domainkey" {
  provider = cloudflare.personal

  comment = "AnonAddy (Proxy not supported!)"
  content = "dk2._domainkey.anonaddy.me"
  name    = "dk2._domainkey.d3strukt0r.dev"
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.dev"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3strukt0r_dev_cname_ssh" {
  provider = cloudflare.personal

  comment = "SSH for Gitea"
  content = "prod.d3strukt0r.dev"
  name    = "ssh.d3strukt0r.dev"
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.dev"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3strukt0r_dev_cname_wundexpertinplus" {
  provider = cloudflare.personal

  content = "team-maro.github.io"
  name    = "wundexpertinplus.d3strukt0r.dev"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.dev"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3strukt0r_dev_mx_apex" {
  provider = cloudflare.personal

  comment  = "AnonAddy"
  content  = "mail2.anonaddy.me"
  name     = "d3strukt0r.dev"
  priority = 20
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "MX"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_dev_mx_apex_2" {
  provider = cloudflare.personal

  comment  = "AnonAddy"
  content  = "mail.anonaddy.me"
  name     = "d3strukt0r.dev"
  priority = 10
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "MX"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_dev_txt_acme_challenge_portainer" {
  provider = cloudflare.personal

  content  = "\"MyBjbveP1ARRoyTVAa44k5NfnWm0Jb5Npe1TGIBZyqI\""
  name     = "_acme-challenge.portainer.d3strukt0r.dev"
  proxied  = false
  tags     = []
  ttl      = 120
  type     = "TXT"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_dev_txt_acme_challenge_portainer_2" {
  provider = cloudflare.personal

  content  = "\"cA-vwoFtEbGds71F-aUfCqj2gAJydjMb_LvbeSe3lU4\""
  name     = "_acme-challenge.portainer.d3strukt0r.dev"
  proxied  = false
  tags     = []
  ttl      = 120
  type     = "TXT"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_dev_txt_atproto" {
  provider = cloudflare.personal

  comment  = "BlueSky Verify"
  content  = "\"did=did:plc:hwgplhlt5rreem3bclu63umf\""
  name     = "_atproto.d3strukt0r.dev"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_dev_txt_apex" {
  provider = cloudflare.personal

  comment  = "Have I Been Pwned Verify"
  content  = "\"have-i-been-pwned-verification=dweb_lyn6vdhsmazisl84225s18d3\""
  name     = "d3strukt0r.dev"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_dev_txt_apex_2" {
  provider = cloudflare.personal

  comment  = "AnonAddy SPF"
  content  = "\"v=spf1 include:spf.anonaddy.me -all\""
  name     = "d3strukt0r.dev"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_dev_txt_apex_3" {
  provider = cloudflare.personal

  comment  = "OpenAI/ChatGPT Verify"
  content  = "\"openai-domain-verification=dv-iGnk42gpu7REPIIMHXN9H5xz\""
  name     = "d3strukt0r.dev"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_dev_txt_apex_4" {
  provider = cloudflare.personal

  comment  = "AnonAddy Verify"
  content  = "\"aa-verify=2e2cf60f7d0fc0c21beaacf519fc39b74ff4c258\""
  name     = "d3strukt0r.dev"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_dev_txt_dmarc" {
  provider = cloudflare.personal

  comment  = "AnonAddy DMARC"
  content  = "\"v=DMARC1; p=quarantine; adkim=s\""
  name     = "_dmarc.d3strukt0r.dev"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_dev_aaaa_apex" {
  provider = cloudflare.personal

  content  = "100::"
  name     = "d3strukt0r.dev"
  proxied  = true
  tags     = []
  ttl      = 1
  type     = "AAAA"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_dev_aaaa_weleda_webcenter_text_export" {
  provider = cloudflare.personal

  content  = "100::"
  name     = "weleda-webcenter-text-export.d3strukt0r.dev"
  proxied  = true
  tags     = []
  ttl      = 1
  type     = "AAAA"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_dev_aaaa_www" {
  provider = cloudflare.personal

  content  = "100::"
  name     = "www.d3strukt0r.dev"
  proxied  = true
  tags     = []
  ttl      = 1
  type     = "AAAA"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}
