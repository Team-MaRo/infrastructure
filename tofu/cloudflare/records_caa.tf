# CAA: only Let's Encrypt may issue certificates for these zones - the old server's Traefik,
# GitHub Pages and cert-manager all use it. Cloudflare's own edge certificates come from
# further CAs, but Cloudflare adds CAA records for those by itself as soon as a zone has
# any, and hides them from the dashboard and API, so they never show up as drift here.
# `dig CAA <zone>` shows the full set.
locals {
  caa_tags = ["issue", "issuewild"]

  caa_personal = {
    for pair in setproduct(keys(local.personal_zones), local.caa_tags) :
    "${pair[0]} ${pair[1]}" => { zone = pair[0], tag = pair[1] }
  }
  caa_arepazo = {
    for pair in setproduct(keys(local.arepazo_zones), local.caa_tags) :
    "${pair[0]} ${pair[1]}" => { zone = pair[0], tag = pair[1] }
  }
}

resource "cloudflare_dns_record" "caa_personal" {
  for_each = local.caa_personal
  provider = cloudflare.personal

  name    = each.value.zone
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CAA"
  zone_id = local.zone_ids[each.value.zone]
  data = {
    flags = 0
    tag   = each.value.tag
    value = "letsencrypt.org"
  }
  settings = {}
}

resource "cloudflare_dns_record" "caa_arepazo" {
  for_each = local.caa_arepazo
  provider = cloudflare.arepazo

  name    = each.value.zone
  proxied = false
  tags    = []
  ttl     = 1
  type    = "CAA"
  zone_id = local.zone_ids[each.value.zone]
  data = {
    flags = 0
    tag   = each.value.tag
    value = "letsencrypt.org"
  }
  settings = {}
}
