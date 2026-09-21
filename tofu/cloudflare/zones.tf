# Zone ids are literals rather than references so that every zone_id in this module
# is known at plan time regardless of ordering. They never change for a given zone.
locals {
  account_ids = {
    personal = "969508b122e95ca7f0abf4d5d32ee2b6"
    arepazo  = "f26583002df54d85be54859c47e772e2"
  }

  zones = {
    "d3st.dev"               = { id = "314dcef02e6e2ee57f31fe56cd0dcdb2", account = "personal" }
    "d3st.org"               = { id = "fbd415128cf6970d1f88ec27bf0898c7", account = "personal" }
    "d3strukt0r.dev"         = { id = "1a6f0bb01dc074c1a03af0f173aef29f", account = "personal" }
    "d3strukt0r.me"          = { id = "572bdbbd687053ca652d80a0beb8f611", account = "personal" }
    "manuele-robine.wedding" = { id = "edb52a357e98b43f456d8be0a2824979", account = "personal" }
    "manuele-vaccari.ch"     = { id = "cbeaf02654d2fa5b4978572f4d4595d0", account = "personal" }
    "robines.space"          = { id = "925a4ca8a2379617a1aa10b275d71142", account = "personal" }
    "rubyn.li"               = { id = "8b9387dc511856989c9fa6bf261eb29a", account = "personal" }
    "sponte.me"              = { id = "d9e566857db35c2c68b4b4afd406c2dc", account = "personal" }
    "wundexpertinplus.com"   = { id = "2889e840f9c25c8c2f0283940d549746", account = "personal" }
    "arepazo.ch"             = { id = "643779bc80b781ad8246192ed73cc559", account = "arepazo" }
  }

  zone_ids = { for name, z in local.zones : name => z.id }

  personal_zones = { for name, z in local.zones : name => z if z.account == "personal" }
  arepazo_zones  = { for name, z in local.zones : name => z if z.account == "arepazo" }
}

# cloudflare_zone has almost no writable surface - account.id, name, paused, type and
# vanity_name_servers (Business/Enterprise only). It is kept because it records which
# account owns which zone, which is otherwise undiscoverable from the config.
resource "cloudflare_zone" "personal" {
  for_each = local.personal_zones
  provider = cloudflare.personal

  account = { id = local.account_ids.personal }
  name    = each.key
  paused  = false
  type    = "full"
}

resource "cloudflare_zone" "arepazo" {
  for_each = local.arepazo_zones
  provider = cloudflare.arepazo

  account = { id = local.account_ids.arepazo }
  name    = each.key
  paused  = false
  type    = "full"
}
