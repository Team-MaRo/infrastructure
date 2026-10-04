# DNS records for arepazo.ch (arepazo account).

# The shop on the prod cluster (kubernetes/components/arepazo): one proxied A and AAAA record per
# node, as prod.d3strukt0r.dev has. Not a CNAME to that name: it lives in the personal account,
# and Cloudflare refuses a proxied CNAME to a proxied name in another account (error 1014).
# www. follows through its CNAME to the apex.
resource "cloudflare_dns_record" "arepazo_ch_a_apex" {
  for_each = local.prod_nodes
  provider = cloudflare.arepazo

  content  = each.value.ipv4
  name     = "arepazo.ch"
  proxied  = true
  tags     = []
  ttl      = 1
  type     = "A"
  zone_id  = local.zone_ids["arepazo.ch"]
  settings = {}
}

resource "cloudflare_dns_record" "arepazo_ch_aaaa_apex" {
  for_each = local.prod_nodes
  provider = cloudflare.arepazo

  content  = each.value.ipv6
  name     = "arepazo.ch"
  proxied  = true
  tags     = []
  ttl      = 1
  type     = "AAAA"
  zone_id  = local.zone_ids["arepazo.ch"]
  settings = {}
}

# The single records that pointed at the old server become prod-01's.
moved {
  from = cloudflare_dns_record.arepazo_ch_a_apex
  to   = cloudflare_dns_record.arepazo_ch_a_apex["prod-01"]
}

moved {
  from = cloudflare_dns_record.arepazo_ch_aaaa_apex
  to   = cloudflare_dns_record.arepazo_ch_aaaa_apex["prod-01"]
}

resource "cloudflare_dns_record" "arepazo_ch_cname_autodiscover" {
  provider = cloudflare.arepazo

  content = "autodiscover.outlook.com"
  name    = "autodiscover.arepazo.ch"
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["arepazo.ch"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "arepazo_ch_cname_domainconnect" {
  provider = cloudflare.arepazo

  content = "_domainconnect.gd.domaincontrol.com"
  name    = "_domainconnect.arepazo.ch"
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["arepazo.ch"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "arepazo_ch_cname_www" {
  provider = cloudflare.arepazo

  content = "arepazo.ch"
  name    = "www.arepazo.ch"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["arepazo.ch"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "arepazo_ch_mx_apex" {
  provider = cloudflare.arepazo

  content  = "arepazo-ch.mail.protection.outlook.com"
  name     = "arepazo.ch"
  priority = 0
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "MX"
  zone_id  = local.zone_ids["arepazo.ch"]
  settings = {}
}

resource "cloudflare_dns_record" "arepazo_ch_txt_apex" {
  provider = cloudflare.arepazo

  content  = "google-site-verification=AxYYpTxn4z0tGGYFn2YQPqaLi-x5j4GrxzjUf6kI1o0"
  name     = "arepazo.ch"
  proxied  = false
  tags     = []
  ttl      = 3600
  type     = "TXT"
  zone_id  = local.zone_ids["arepazo.ch"]
  settings = {}
}

resource "cloudflare_dns_record" "arepazo_ch_txt_apex_2" {
  provider = cloudflare.arepazo

  content  = "v=spf1 include:spf.protection.outlook.com -all"
  name     = "arepazo.ch"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["arepazo.ch"]
  settings = {}
}

resource "cloudflare_dns_record" "arepazo_ch_txt_apex_3" {
  provider = cloudflare.arepazo

  content  = "v=verifydomain MS=5017473"
  name     = "arepazo.ch"
  proxied  = false
  tags     = []
  ttl      = 1
  type     = "TXT"
  zone_id  = local.zone_ids["arepazo.ch"]
  settings = {}
}
