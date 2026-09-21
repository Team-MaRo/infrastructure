# DNS records for wundexpertinplus.com (personal account).

resource "cloudflare_dns_record" "wundexpertinplus_com_aaaa_apex" {
  provider = cloudflare.personal

  content  = "100::"
  name     = "wundexpertinplus.com"
  proxied  = true
  tags     = []
  ttl      = 1
  type     = "AAAA"
  zone_id = local.zone_ids["wundexpertinplus.com"]
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
  zone_id = local.zone_ids["wundexpertinplus.com"]
  settings = {}
}
