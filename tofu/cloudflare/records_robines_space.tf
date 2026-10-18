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

# The old WordPress portfolio on the cluster (kubernetes/components/robines-portfolio); robines.space
# and www are the Cloudflare Worker (the AAAA records below).
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

  comment  = "DMARC, DKIM relaxed (no adkim=s): mailomat signs as mailomat.robines.space"
  content  = "\"v=DMARC1; p=quarantine\""
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

# mailomat.swiss sends mail for this domain (smarthost, see "Sending through mailomat" in the
# README). Its records sit on a subdomain because the apex already has a mail service; the
# subdomain is the return path for bounces (SPF, MX) and the DKIM domain.

resource "cloudflare_dns_record" "robines_space_txt_mailomat" {
  provider = cloudflare.personal

  content  = "v=spf1 include:mailomat.cloud -all"
  name     = "mailomat.robines.space"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["robines.space"]
  settings = {}
}

resource "cloudflare_dns_record" "robines_space_txt_mom1_domainkey_mailomat" {
  provider = cloudflare.personal

  content  = "v=DKIM1; k=rsa; p=MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAig0FbaLqQGVytHlrAD23vUM+CsIvrckVBCa+CAVqSO9Hntu2zvv3/OR0s8vesqU54QPoXB6/TRo2mudRZMT/rvlmh6MeqI2Mc4XxZYZ8EUHUFMwBUmX5HUgDAxvyRN+QXFIxTYMNfaL5N2TH6UUv+vfdof+SPrS2ICekMv3mUwFEV5PJRVbSHLPnXiM8H2r+NCv2jP+8qhWUX0BIF14jJ+ZzLF420kEAt85uv2yGDXIQ7HKJHwk6UTCymO5V55xpLKtrjhNhe9MofGEDqMtuTvOgK4fvpsHA6GH3cqo7BaLxfjdQbg/hAtz6UdiKwvx2vY64as8vnp7zxCp/8RIpbwIDAQAB"
  name     = "mom1._domainkey.mailomat.robines.space"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["robines.space"]
  settings = {}
}

resource "cloudflare_dns_record" "robines_space_mx_mailomat" {
  provider = cloudflare.personal

  content  = "mx.mailomat.cloud"
  name     = "mailomat.robines.space"
  priority = 10
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "MX"
  zone_id  = local.zone_ids["robines.space"]
  settings = {}
}
