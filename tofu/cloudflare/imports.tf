# Adoption of the hand-created Cloudflare configuration, kept in version control so
# the adoption itself is reviewable.
#
# Zones and settings import through for_each over the same maps that declare them.
# DNS records are explicit because each one is a hand-editable resource.

import {
  for_each = local.personal_zones

  to = cloudflare_zone.personal[each.key]
  id = each.value.id
}

import {
  for_each = local.arepazo_zones

  to = cloudflare_zone.arepazo[each.key]
  id = each.value.id
}

import {
  for_each = local.personal_settings

  to = cloudflare_zone_setting.personal[each.key]
  id = "${local.zone_ids[each.value.zone]}/${each.value.setting}"
}

import {
  for_each = local.arepazo_settings

  to = cloudflare_zone_setting.arepazo[each.key]
  id = "${local.zone_ids[each.value.zone]}/${each.value.setting}"
}

import {
  to = cloudflare_dns_record.d3st_dev_cname_wildcard
  id = "314dcef02e6e2ee57f31fe56cd0dcdb2/f1072fc500596a01166b10d7e30a7bd3"
}

import {
  to = cloudflare_dns_record.d3st_dev_cname_apex
  id = "314dcef02e6e2ee57f31fe56cd0dcdb2/2870a524a43a365a380ae52e3c9246f5"
}

import {
  to = cloudflare_dns_record.d3st_dev_cname_dk1_domainkey
  id = "314dcef02e6e2ee57f31fe56cd0dcdb2/becdd07bd6410d812fbabb710eace7f3"
}

import {
  to = cloudflare_dns_record.d3st_dev_cname_dk2_domainkey
  id = "314dcef02e6e2ee57f31fe56cd0dcdb2/9474c644bfe79407b7c68642ed0d109e"
}

import {
  to = cloudflare_dns_record.d3st_dev_mx_apex
  id = "314dcef02e6e2ee57f31fe56cd0dcdb2/8a2a3d6ef3e478af5af458fcbc54a8c4"
}

import {
  to = cloudflare_dns_record.d3st_dev_mx_apex_2
  id = "314dcef02e6e2ee57f31fe56cd0dcdb2/06a2ba8b1b0b68a32b14999e670d7d21"
}

import {
  to = cloudflare_dns_record.d3st_dev_txt_apex
  id = "314dcef02e6e2ee57f31fe56cd0dcdb2/1fa184be95615536ab8f9c408666b733"
}

import {
  to = cloudflare_dns_record.d3st_dev_txt_apex_2
  id = "314dcef02e6e2ee57f31fe56cd0dcdb2/9ba77f4c92b60be3b2339c2b3bead106"
}

import {
  to = cloudflare_dns_record.d3st_dev_txt_apex_3
  id = "314dcef02e6e2ee57f31fe56cd0dcdb2/56b81e8d1c9cd90dcd49b5e4a85bf198"
}

import {
  to = cloudflare_dns_record.d3st_dev_txt_apex_4
  id = "314dcef02e6e2ee57f31fe56cd0dcdb2/e361a2a1d4f244fc129d582eb66d0480"
}

import {
  to = cloudflare_dns_record.d3st_dev_txt_dmarc
  id = "314dcef02e6e2ee57f31fe56cd0dcdb2/20987934db6bece1be6ab30e683a0a0d"
}

import {
  to = cloudflare_dns_record.d3st_org_cname_wildcard
  id = "fbd415128cf6970d1f88ec27bf0898c7/04e6736349a6d98aaf7d9a9d030a6645"
}

import {
  to = cloudflare_dns_record.d3st_org_cname_apex
  id = "fbd415128cf6970d1f88ec27bf0898c7/29bf91d15d98ff31644ee44cca629115"
}

import {
  to = cloudflare_dns_record.d3st_org_cname_dk1_domainkey
  id = "fbd415128cf6970d1f88ec27bf0898c7/2883b61d8f06c15e3c027c39f740d246"
}

import {
  to = cloudflare_dns_record.d3st_org_cname_dk2_domainkey
  id = "fbd415128cf6970d1f88ec27bf0898c7/c43413c6741f0a60f3bc1e7f3f77592f"
}

import {
  to = cloudflare_dns_record.d3st_org_mx_apex
  id = "fbd415128cf6970d1f88ec27bf0898c7/9912c2a69a19a25985438441b7a6e910"
}

import {
  to = cloudflare_dns_record.d3st_org_mx_apex_2
  id = "fbd415128cf6970d1f88ec27bf0898c7/94c937a7e1542b2c2b864a2e48dfcd9c"
}

import {
  to = cloudflare_dns_record.d3st_org_txt_apex
  id = "fbd415128cf6970d1f88ec27bf0898c7/cb86576d45a4144d253642384404baec"
}

import {
  to = cloudflare_dns_record.d3st_org_txt_apex_2
  id = "fbd415128cf6970d1f88ec27bf0898c7/2acb350ea42d60f1978b22705e7dbc73"
}

import {
  to = cloudflare_dns_record.d3st_org_txt_apex_3
  id = "fbd415128cf6970d1f88ec27bf0898c7/d994b1474f87a3606d41e067042526b8"
}

import {
  to = cloudflare_dns_record.d3st_org_txt_dmarc
  id = "fbd415128cf6970d1f88ec27bf0898c7/4de66d105404e28ad5046be29a2a17dc"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_a_prod
  id = "1a6f0bb01dc074c1a03af0f173aef29f/e84c23510803e917500c8b9225a1966f"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_aaaa_prod
  id = "1a6f0bb01dc074c1a03af0f173aef29f/4f913513aa7f91ae977bf6dd17efa65a"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_cname_wildcard
  id = "1a6f0bb01dc074c1a03af0f173aef29f/f459f20c6afc3cba8145ec3e6ab98f1a"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_cname_dk1_domainkey
  id = "1a6f0bb01dc074c1a03af0f173aef29f/46ae6558ec376cd9e97092a9191bac9a"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_cname_dk2_domainkey
  id = "1a6f0bb01dc074c1a03af0f173aef29f/47c306e23d3efba78a27ad065697dbdb"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_cname_ssh
  id = "1a6f0bb01dc074c1a03af0f173aef29f/d9bf576127a07f63cfa757c0bc370fa9"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_cname_wundexpertinplus
  id = "1a6f0bb01dc074c1a03af0f173aef29f/88ae3e671f8d8b5b27e7423b336ea9da"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_mx_apex
  id = "1a6f0bb01dc074c1a03af0f173aef29f/c23802131f7c5fe2b639dd911dd3e1f8"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_mx_apex_2
  id = "1a6f0bb01dc074c1a03af0f173aef29f/f10763fef79a09e12404d1e5fc971cbc"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_txt_acme_challenge_portainer
  id = "1a6f0bb01dc074c1a03af0f173aef29f/f6b035be5e090488b8fd9014a899e529"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_txt_acme_challenge_portainer_2
  id = "1a6f0bb01dc074c1a03af0f173aef29f/1a87e754e562e545d7f3014a642ddcf0"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_txt_atproto
  id = "1a6f0bb01dc074c1a03af0f173aef29f/8f1c00ae8e480174b16b4133b090dfb0"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_txt_apex
  id = "1a6f0bb01dc074c1a03af0f173aef29f/6e0fa010d95e1ae882d92fd663acf69f"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_txt_apex_2
  id = "1a6f0bb01dc074c1a03af0f173aef29f/66e9f56e86806db9aa85c3beda0b681a"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_txt_apex_3
  id = "1a6f0bb01dc074c1a03af0f173aef29f/f528a203106138c8cd27a0ceef44fe94"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_txt_apex_4
  id = "1a6f0bb01dc074c1a03af0f173aef29f/35864c41dce3e91cba9b4a1c896cb0f8"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_txt_dmarc
  id = "1a6f0bb01dc074c1a03af0f173aef29f/7ee4d00311330afc14ed576a04a70f5d"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_aaaa_apex
  id = "1a6f0bb01dc074c1a03af0f173aef29f/49fa6b6f6c6eeddd747229fa265ad33b"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_aaaa_weleda_webcenter_text_export
  id = "1a6f0bb01dc074c1a03af0f173aef29f/12f72c87d8b9a0b15efd6b1f03651923"
}

import {
  to = cloudflare_dns_record.d3strukt0r_dev_aaaa_www
  id = "1a6f0bb01dc074c1a03af0f173aef29f/56c3ba8757dc0dd686c3869a9b3520cd"
}

import {
  to = cloudflare_dns_record.d3strukt0r_me_cname_wildcard
  id = "572bdbbd687053ca652d80a0beb8f611/77b1c9dffeabfeb7d9e2d856b280a388"
}

import {
  to = cloudflare_dns_record.d3strukt0r_me_cname_apex
  id = "572bdbbd687053ca652d80a0beb8f611/4afb066a0e06370d47b46f46de296493"
}

import {
  to = cloudflare_dns_record.d3strukt0r_me_cname_protonmail2_domainkey
  id = "572bdbbd687053ca652d80a0beb8f611/b5e49eafe081770fe6833c220714fab1"
}

import {
  to = cloudflare_dns_record.d3strukt0r_me_cname_protonmail3_domainkey
  id = "572bdbbd687053ca652d80a0beb8f611/ce74aa88619f2a97380b2cbf6c0f6f30"
}

import {
  to = cloudflare_dns_record.d3strukt0r_me_cname_protonmail_domainkey
  id = "572bdbbd687053ca652d80a0beb8f611/f25c01f13ece2c0f1d9091eb4086250d"
}

import {
  to = cloudflare_dns_record.d3strukt0r_me_mx_apex
  id = "572bdbbd687053ca652d80a0beb8f611/a43020dfe2ab49a8906d8d17d90eeb42"
}

import {
  to = cloudflare_dns_record.d3strukt0r_me_mx_apex_2
  id = "572bdbbd687053ca652d80a0beb8f611/c543fe35a64451bd09f3eafc9f6ddc8d"
}

import {
  to = cloudflare_dns_record.d3strukt0r_me_txt_apex
  id = "572bdbbd687053ca652d80a0beb8f611/52ade00089dba7237cdf6efb13aa6d88"
}

import {
  to = cloudflare_dns_record.d3strukt0r_me_txt_apex_2
  id = "572bdbbd687053ca652d80a0beb8f611/2123db14983e8c6c7db098816493cc80"
}

import {
  to = cloudflare_dns_record.d3strukt0r_me_txt_dmarc
  id = "572bdbbd687053ca652d80a0beb8f611/b2babf2f4dd4c73e91ac5f4f3511c7b7"
}

import {
  to = cloudflare_dns_record.manuele_robine_wedding_cname_wildcard
  id = "edb52a357e98b43f456d8be0a2824979/59c4a690f6660416ad3dae94328d7a8b"
}

import {
  to = cloudflare_dns_record.manuele_robine_wedding_cname_apex
  id = "edb52a357e98b43f456d8be0a2824979/85341da6bbf85f2470f41e45a54f9f37"
}

import {
  to = cloudflare_dns_record.manuele_robine_wedding_txt_apex
  id = "edb52a357e98b43f456d8be0a2824979/bdaa266a66d133bbf35016ffccabc1d8"
}

import {
  to = cloudflare_dns_record.manuele_vaccari_ch_cname_wildcard
  id = "cbeaf02654d2fa5b4978572f4d4595d0/88d146360e96917f6c4d6cfcb0855253"
}

import {
  to = cloudflare_dns_record.manuele_vaccari_ch_cname_apex
  id = "cbeaf02654d2fa5b4978572f4d4595d0/13c3b0833eb3bb0edad9a25341c102ff"
}

import {
  to = cloudflare_dns_record.manuele_vaccari_ch_cname_webcenter_text_export
  id = "cbeaf02654d2fa5b4978572f4d4595d0/b6a1f9facf0632f0acc4e6f3dbb6205b"
}

import {
  to = cloudflare_dns_record.robines_space_cname_dk1_domainkey
  id = "925a4ca8a2379617a1aa10b275d71142/99a8a11b800506e5968b9e5e61611ed0"
}

import {
  to = cloudflare_dns_record.robines_space_cname_dk2_domainkey
  id = "925a4ca8a2379617a1aa10b275d71142/c1fb919bc2b873b0d168c05eb9e21972"
}

import {
  to = cloudflare_dns_record.robines_space_cname_old
  id = "925a4ca8a2379617a1aa10b275d71142/5e25d835c0d19cbdac23c51544aafd8c"
}

import {
  to = cloudflare_dns_record.robines_space_cname_wildcard
  id = "925a4ca8a2379617a1aa10b275d71142/02cb43663691cfd807a3bfe05b1b2018"
}

import {
  to = cloudflare_dns_record.robines_space_mx_apex
  id = "925a4ca8a2379617a1aa10b275d71142/1c9101bb4503267d7707bdcf8caf75a0"
}

import {
  to = cloudflare_dns_record.robines_space_mx_apex_2
  id = "925a4ca8a2379617a1aa10b275d71142/a26ab9a1a9cca80675bbcc1e84633523"
}

import {
  to = cloudflare_dns_record.robines_space_txt_dmarc
  id = "925a4ca8a2379617a1aa10b275d71142/5e27c1916da957a3e41d74a7fc0e8f8e"
}

import {
  to = cloudflare_dns_record.robines_space_txt_apex
  id = "925a4ca8a2379617a1aa10b275d71142/eb9109e6acf1a7083b3acfbf0c34fd78"
}

import {
  to = cloudflare_dns_record.robines_space_txt_apex_2
  id = "925a4ca8a2379617a1aa10b275d71142/e909d34665f2163b1c24ee10efb94dc9"
}

import {
  to = cloudflare_dns_record.robines_space_txt_apex_3
  id = "925a4ca8a2379617a1aa10b275d71142/297ca2e2dd0c6780324f0cf7863b0769"
}

import {
  to = cloudflare_dns_record.robines_space_aaaa_apex
  id = "925a4ca8a2379617a1aa10b275d71142/fb5f0c09a91ca1002ad10490f949efda"
}

import {
  to = cloudflare_dns_record.robines_space_aaaa_www
  id = "925a4ca8a2379617a1aa10b275d71142/7a60d1036839ad3e5082f11ceef6c3ec"
}

import {
  to = cloudflare_dns_record.rubyn_li_cname_dk1_domainkey
  id = "8b9387dc511856989c9fa6bf261eb29a/54c1ec0017eb7df99f907be89bbca851"
}

import {
  to = cloudflare_dns_record.rubyn_li_cname_dk2_domainkey
  id = "8b9387dc511856989c9fa6bf261eb29a/db902a64c7309b3cff523fc513d96641"
}

import {
  to = cloudflare_dns_record.rubyn_li_mx_apex
  id = "8b9387dc511856989c9fa6bf261eb29a/ffb7fdb7f5df352c6c350c1f2f4f8608"
}

import {
  to = cloudflare_dns_record.rubyn_li_mx_apex_2
  id = "8b9387dc511856989c9fa6bf261eb29a/d20cb00a7c122e5f5001ca7e1c2f6fb4"
}

import {
  to = cloudflare_dns_record.rubyn_li_txt_dmarc
  id = "8b9387dc511856989c9fa6bf261eb29a/47e501e449eb6d4c977801e82f7f1a15"
}

import {
  to = cloudflare_dns_record.rubyn_li_txt_apex
  id = "8b9387dc511856989c9fa6bf261eb29a/7ef862fe456932f5233adafbcdbfb219"
}

import {
  to = cloudflare_dns_record.rubyn_li_txt_apex_2
  id = "8b9387dc511856989c9fa6bf261eb29a/428c0655f06a7e01dae4cb38931e3e74"
}

import {
  to = cloudflare_dns_record.sponte_me_txt_dmarc
  id = "d9e566857db35c2c68b4b4afd406c2dc/41f0432ffa16006ef3f29b47ff4b4377"
}

import {
  to = cloudflare_dns_record.sponte_me_txt_apex
  id = "d9e566857db35c2c68b4b4afd406c2dc/b0109be3907d6f2ba712fc1fdb179a61"
}

import {
  to = cloudflare_dns_record.wundexpertinplus_com_aaaa_apex
  id = "2889e840f9c25c8c2f0283940d549746/89979b634cfe63eba94055ba96f47df3"
}

import {
  to = cloudflare_dns_record.wundexpertinplus_com_aaaa_www
  id = "2889e840f9c25c8c2f0283940d549746/250b725b59db7380c6766aeb3bc141dc"
}

import {
  to = cloudflare_dns_record.arepazo_ch_a_apex
  id = "643779bc80b781ad8246192ed73cc559/f3ea5a3e7bf6e59a91b148c6249417c2"
}

import {
  to = cloudflare_dns_record.arepazo_ch_aaaa_apex
  id = "643779bc80b781ad8246192ed73cc559/3ef6721f6528a3989a333edfb5c046ad"
}

import {
  to = cloudflare_dns_record.arepazo_ch_cname_autodiscover
  id = "643779bc80b781ad8246192ed73cc559/40ce380b0bf35974ef22232ab537ea1a"
}

import {
  to = cloudflare_dns_record.arepazo_ch_cname_domainconnect
  id = "643779bc80b781ad8246192ed73cc559/f028c33513af451bd4c93da41e449399"
}

import {
  to = cloudflare_dns_record.arepazo_ch_cname_www
  id = "643779bc80b781ad8246192ed73cc559/0ff34c24c86b7afbcd0554804b9bf8fb"
}

import {
  to = cloudflare_dns_record.arepazo_ch_mx_apex
  id = "643779bc80b781ad8246192ed73cc559/f120c50817a19802921ae9b494537b2d"
}

import {
  to = cloudflare_dns_record.arepazo_ch_txt_apex
  id = "643779bc80b781ad8246192ed73cc559/4971a2440c61647800b03da149b118f7"
}

import {
  to = cloudflare_dns_record.arepazo_ch_txt_apex_2
  id = "643779bc80b781ad8246192ed73cc559/7c2b467d33024c42470896051696438b"
}

import {
  to = cloudflare_dns_record.arepazo_ch_txt_apex_3
  id = "643779bc80b781ad8246192ed73cc559/c14919d83981eaa496c3de5827f9d96a"
}
