# DNS records for wundexpertinplus.com (personal account).

resource "cloudflare_dns_record" "wundexpertinplus_com_aaaa_apex" {
  provider = cloudflare.personal

  content  = "100::"
  name     = "wundexpertinplus.com"
  proxied  = true
  tags     = []
  ttl      = 1
  type     = "AAAA"
  zone_id  = local.zone_ids["wundexpertinplus.com"]
  settings = {}
}

resource "cloudflare_dns_record" "wundexpertinplus_com_aaaa_www" {
  provider = cloudflare.personal

  content  = "100::"
  name     = "www.wundexpertinplus.com"
  proxied  = true
  tags     = []
  ttl      = 1
  type     = "AAAA"
  zone_id  = local.zone_ids["wundexpertinplus.com"]
  settings = {}
}

# mailomat.swiss sends mail for this domain (smarthost, see "Sending through mailomat" in the
# README). The domain receives no mail, but its records still sit on a subdomain, so an MX on
# the apex stays free for a mail service of its own; the subdomain is the return path for
# bounces (SPF, MX) and the DKIM domain.

resource "cloudflare_dns_record" "wundexpertinplus_com_txt_mailomat" {
  provider = cloudflare.personal

  content  = "v=spf1 include:mailomat.cloud -all"
  name     = "mailomat.wundexpertinplus.com"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["wundexpertinplus.com"]
  settings = {}
}

resource "cloudflare_dns_record" "wundexpertinplus_com_txt_mom1_domainkey_mailomat" {
  provider = cloudflare.personal

  content  = "v=DKIM1; k=rsa; p=MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEArDuGM3tO1/0YkYwzZ8lGiX9a62hv2GPwLEJfHce9J+PA0UnagPsU4pCcPAz+I+AGgWW2QQtYcSKwek0MjbydR9Dn/sXUwiuQUa3hsNKSmzC8VsUOqmKaFJMG5DvZ1w59NuJaLwnLcEgZ/99GMP2Y7+nyk5y5jzI3LfoD1/4JT412/dZhZcJpQ0qc69ybctZE8f2vNXb7MjXJ+KUYJ1C3bXlnlomZ7su+UEnCOne+D0gIoKVvT9vydHDfb3sTXkX3l5AnzJCvHJzLQ52ujm8f4Rs2IhmdyLRJ7BsqSiL8wQyxTSmRsShEKn3jUDXAtZ24e2H4oKf/wVqbtVxJ/fF7NwIDAQAB"
  name     = "mom1._domainkey.mailomat.wundexpertinplus.com"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["wundexpertinplus.com"]
  settings = {}
}

resource "cloudflare_dns_record" "wundexpertinplus_com_mx_mailomat" {
  provider = cloudflare.personal

  content  = "mx.mailomat.cloud"
  name     = "mailomat.wundexpertinplus.com"
  priority = 10
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "MX"
  zone_id  = local.zone_ids["wundexpertinplus.com"]
  settings = {}
}
