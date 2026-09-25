# Six of Cloudflare's ~60 zone settings, the security and TLS relevant ones. The rest
# stay in the dashboard. Values are what each zone has, so that a change shows up as a diff
# rather than going unnoticed.
#
# `ssl` (off/flexible/full/strict) is deliberately not here. Every zone uses Automatic
# SSL/TLS, where Cloudflare scans the origin and picks the mode itself; `ssl` is only that
# scan's current result, and writing it would switch the zone to manual mode. What is
# managed instead is the switch, ssl_automatic_mode = "auto", so a zone flipped to manual
# shows up as drift. TLS 1.0 and 1.1 are deprecated, hence min_tls_version 1.2 everywhere.
locals {
  zone_settings = {
    "d3st.dev"               = { ssl_automatic_mode = "auto", always_use_https = "on", min_tls_version = "1.2", automatic_https_rewrites = "on", tls_1_3 = "on", security_level = "medium" }
    "d3st.org"               = { ssl_automatic_mode = "auto", always_use_https = "on", min_tls_version = "1.2", automatic_https_rewrites = "on", tls_1_3 = "on", security_level = "medium" }
    "d3strukt0r.dev"         = { ssl_automatic_mode = "auto", always_use_https = "on", min_tls_version = "1.2", automatic_https_rewrites = "on", tls_1_3 = "zrt", security_level = "medium" }
    "d3strukt0r.me"          = { ssl_automatic_mode = "auto", always_use_https = "off", min_tls_version = "1.2", automatic_https_rewrites = "on", tls_1_3 = "on", security_level = "medium" }
    "manuele-robine.wedding" = { ssl_automatic_mode = "auto", always_use_https = "on", min_tls_version = "1.2", automatic_https_rewrites = "on", tls_1_3 = "zrt", security_level = "medium" }
    "manuele-vaccari.ch"     = { ssl_automatic_mode = "auto", always_use_https = "off", min_tls_version = "1.2", automatic_https_rewrites = "off", tls_1_3 = "zrt", security_level = "medium" }
    "robines.space"          = { ssl_automatic_mode = "auto", always_use_https = "on", min_tls_version = "1.2", automatic_https_rewrites = "on", tls_1_3 = "zrt", security_level = "medium" }
    "rubyn.li"               = { ssl_automatic_mode = "auto", always_use_https = "off", min_tls_version = "1.2", automatic_https_rewrites = "on", tls_1_3 = "on", security_level = "medium" }
    "sponte.me"              = { ssl_automatic_mode = "auto", always_use_https = "off", min_tls_version = "1.2", automatic_https_rewrites = "on", tls_1_3 = "on", security_level = "medium" }
    "wundexpertinplus.com"   = { ssl_automatic_mode = "auto", always_use_https = "on", min_tls_version = "1.2", automatic_https_rewrites = "on", tls_1_3 = "zrt", security_level = "medium" }
    "arepazo.ch"             = { ssl_automatic_mode = "auto", always_use_https = "on", min_tls_version = "1.2", automatic_https_rewrites = "on", tls_1_3 = "zrt", security_level = "medium" }
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

# The SSL mode Automatic SSL/TLS has currently chosen per zone - read, never written, so it
# cannot drift or switch a zone to manual mode. `tofu plan` shows it under "Changes to
# Outputs" when a scan changed it; `tofu output ssl_modes` shows what the last apply stored.
data "cloudflare_zone_setting" "ssl_personal" {
  for_each = { for zone, z in local.zones : zone => z if z.account == "personal" }
  provider = cloudflare.personal

  zone_id    = each.value.id
  setting_id = "ssl"
}

data "cloudflare_zone_setting" "ssl_arepazo" {
  for_each = { for zone, z in local.zones : zone => z if z.account == "arepazo" }
  provider = cloudflare.arepazo

  zone_id    = each.value.id
  setting_id = "ssl"
}

output "ssl_modes" {
  description = "The SSL mode Cloudflare's automatic mode currently uses per zone (flexible, full, strict)."
  value = merge(
    { for zone, s in data.cloudflare_zone_setting.ssl_personal : zone => s.value },
    { for zone, s in data.cloudflare_zone_setting.ssl_arepazo : zone => s.value },
  )
}
