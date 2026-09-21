# Six of Cloudflare's ~60 zone settings, the security and TLS relevant ones. The rest
# stay in the dashboard. Values are what each zone has today, captured so that a change
# shows up as a diff rather than going unnoticed.
locals {
  zone_settings = {
    "d3st.dev"               = { ssl = "flexible", always_use_https = "on", min_tls_version = "1.2", automatic_https_rewrites = "on", tls_1_3 = "on", security_level = "medium" }
    "d3st.org"               = { ssl = "flexible", always_use_https = "on", min_tls_version = "1.2", automatic_https_rewrites = "on", tls_1_3 = "on", security_level = "medium" }
    "d3strukt0r.dev"         = { ssl = "full", always_use_https = "on", min_tls_version = "1.2", automatic_https_rewrites = "on", tls_1_3 = "zrt", security_level = "medium" }
    "d3strukt0r.me"          = { ssl = "flexible", always_use_https = "off", min_tls_version = "1.2", automatic_https_rewrites = "on", tls_1_3 = "on", security_level = "medium" }
    "manuele-robine.wedding" = { ssl = "strict", always_use_https = "on", min_tls_version = "1.2", automatic_https_rewrites = "on", tls_1_3 = "zrt", security_level = "medium" }
    "manuele-vaccari.ch"     = { ssl = "full", always_use_https = "off", min_tls_version = "1.2", automatic_https_rewrites = "off", tls_1_3 = "zrt", security_level = "medium" }
    "robines.space"          = { ssl = "full", always_use_https = "on", min_tls_version = "1.0", automatic_https_rewrites = "on", tls_1_3 = "zrt", security_level = "medium" }
    "rubyn.li"               = { ssl = "full", always_use_https = "off", min_tls_version = "1.0", automatic_https_rewrites = "on", tls_1_3 = "on", security_level = "medium" }
    "sponte.me"              = { ssl = "full", always_use_https = "off", min_tls_version = "1.0", automatic_https_rewrites = "on", tls_1_3 = "on", security_level = "medium" }
    "wundexpertinplus.com"   = { ssl = "strict", always_use_https = "on", min_tls_version = "1.0", automatic_https_rewrites = "on", tls_1_3 = "zrt", security_level = "medium" }
    "arepazo.ch"             = { ssl = "full", always_use_https = "on", min_tls_version = "1.0", automatic_https_rewrites = "on", tls_1_3 = "zrt", security_level = "medium" }
  }

  # One entry per zone/setting pair, split by account because provider cannot be
  # chosen per for_each instance.
  setting_pairs = {
    for pair in flatten([
      for zone, settings in local.zone_settings : [
        for setting, value in settings : {
          key     = "${zone}/${setting}"
          zone    = zone
          setting = setting
          value   = value
        }
      ]
    ]) : pair.key => pair
  }

  personal_settings = { for k, p in local.setting_pairs : k => p if local.zones[p.zone].account == "personal" }
  arepazo_settings  = { for k, p in local.setting_pairs : k => p if local.zones[p.zone].account == "arepazo" }
}

resource "cloudflare_zone_setting" "personal" {
  for_each = local.personal_settings
  provider = cloudflare.personal

  zone_id    = local.zone_ids[each.value.zone]
  setting_id = each.value.setting
  value      = each.value.value
}

resource "cloudflare_zone_setting" "arepazo" {
  for_each = local.arepazo_settings
  provider = cloudflare.arepazo

  zone_id    = local.zone_ids[each.value.zone]
  setting_id = each.value.setting
  value      = each.value.value
}
