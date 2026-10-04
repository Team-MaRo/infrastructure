# DNS records for d3strukt0r.dev (personal account).

# The old DigitalOcean server. Everything still served there points at this name - the
# wildcard among them, so any *.d3strukt0r.dev without a record of its own lands there.
# A service moving to the cluster gets its own record pointing at prod instead, which
# beats the wildcard.
resource "cloudflare_dns_record" "d3strukt0r_dev_a_prod_old" {
  provider = cloudflare.personal

  content  = "161.35.16.9"
  name     = "prod-old.d3strukt0r.dev"
  proxied  = true
  tags     = []
  ttl      = 1
  type     = "A"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_dev_aaaa_prod_old" {
  provider = cloudflare.personal

  content  = "2a03:b0c0:3:d0::f65:2001"
  name     = "prod-old.d3strukt0r.dev"
  proxied  = true
  tags     = []
  ttl      = 1
  type     = "AAAA"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

# The prod cluster's entry point: one A and one AAAA record per node, each node's public
# addresses. Traefik listens on every node's host network, IPv4 and IPv6 alike, and
# Cloudflare spreads requests over the records. Services on the cluster CNAME to this name.
# The addresses are not in any other module's state this one can read, so adding or
# replacing a node means updating this map - until a Hetzner Load Balancer gives the cluster
# one address.
locals {
  prod_nodes = {
    "prod-01" = { ipv4 = "178.104.135.61", ipv6 = "2a01:4f8:1c1e:9487::1" }
    "prod-02" = { ipv4 = "178.104.133.199", ipv6 = "2a01:4f8:1c16:4577::1" }
    "prod-03" = { ipv4 = "78.47.68.27", ipv6 = "2a01:4f8:1c16:335::1" }
  }
}

resource "cloudflare_dns_record" "d3strukt0r_dev_a_prod" {
  for_each = local.prod_nodes
  provider = cloudflare.personal

  content  = each.value.ipv4
  name     = "prod.d3strukt0r.dev"
  proxied  = true
  tags     = []
  ttl      = 1
  type     = "A"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

resource "cloudflare_dns_record" "d3strukt0r_dev_aaaa_prod" {
  for_each = local.prod_nodes
  provider = cloudflare.personal

  content  = each.value.ipv6
  name     = "prod.d3strukt0r.dev"
  proxied  = true
  tags     = []
  ttl      = 1
  type     = "AAAA"
  zone_id  = local.zone_ids["d3strukt0r.dev"]
  settings = {}
}

# The single records that pointed at the old server become prod-01's.
moved {
  from = cloudflare_dns_record.d3strukt0r_dev_a_prod
  to   = cloudflare_dns_record.d3strukt0r_dev_a_prod["prod-01"]
}

moved {
  from = cloudflare_dns_record.d3strukt0r_dev_aaaa_prod
  to   = cloudflare_dns_record.d3strukt0r_dev_aaaa_prod["prod-01"]
}

resource "cloudflare_dns_record" "d3strukt0r_dev_cname_wildcard" {
  provider = cloudflare.personal

  content = "prod-old.d3strukt0r.dev"
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

# Zitadel, the identity provider, on the cluster. An explicit record beats the wildcard above.
resource "cloudflare_dns_record" "d3strukt0r_dev_cname_auth" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "auth.d3strukt0r.dev"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.dev"]
  settings = {
    flatten_cname = false
  }
}

# Argo CD's UI and API, on the cluster behind Zitadel's login.
resource "cloudflare_dns_record" "d3strukt0r_dev_cname_argocd" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "argocd.d3strukt0r.dev"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.dev"]
  settings = {
    flatten_cname = false
  }
}

# Grafana, on the cluster behind Zitadel's login.
resource "cloudflare_dns_record" "d3strukt0r_dev_cname_grafana" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "grafana.d3strukt0r.dev"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.dev"]
  settings = {
    flatten_cname = false
  }
}

# Prometheus' UI, on the cluster behind the Zitadel gate (oauth2-proxy).
resource "cloudflare_dns_record" "d3strukt0r_dev_cname_prometheus" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "prometheus.d3strukt0r.dev"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.dev"]
  settings = {
    flatten_cname = false
  }
}

# Alertmanager's UI, on the cluster behind the Zitadel gate (oauth2-proxy).
resource "cloudflare_dns_record" "d3strukt0r_dev_cname_alertmanager" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "alertmanager.d3strukt0r.dev"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.dev"]
  settings = {
    flatten_cname = false
  }
}

# oauth2-proxy's own address, where Zitadel returns after a login for the UIs it guards.
resource "cloudflare_dns_record" "d3strukt0r_dev_cname_oauth2_proxy" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "oauth2-proxy.d3strukt0r.dev"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.dev"]
  settings = {
    flatten_cname = false
  }
}

# The Traefik dashboard, behind oauth2-proxy.
resource "cloudflare_dns_record" "d3strukt0r_dev_cname_traefik" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "traefik.d3strukt0r.dev"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.dev"]
  settings = {
    flatten_cname = false
  }
}

# phpMyAdmin, the shared MariaDB's web UI, behind oauth2-proxy.
resource "cloudflare_dns_record" "d3strukt0r_dev_cname_phpmyadmin" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "phpmyadmin.d3strukt0r.dev"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.dev"]
  settings = {
    flatten_cname = false
  }
}

# pgAdmin, the shared PostgreSQL's web UI, with its own Zitadel login.
resource "cloudflare_dns_record" "d3strukt0r_dev_cname_pgadmin" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "pgadmin.d3strukt0r.dev"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.dev"]
  settings = {
    flatten_cname = false
  }
}

# The wedding website's second name and its API (kubernetes/components/wedding-manuele-robine).
resource "cloudflare_dns_record" "d3strukt0r_dev_cname_wedding_manuele_robine" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "wedding-manuele-robine.d3strukt0r.dev"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.dev"]
  settings = {
    flatten_cname = false
  }
}

resource "cloudflare_dns_record" "d3strukt0r_dev_cname_api_wedding_manuele_robine" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "api-wedding-manuele-robine.d3strukt0r.dev"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.dev"]
  settings = {
    flatten_cname = false
  }
}

# The public status page (Gatus), on the cluster.
resource "cloudflare_dns_record" "d3strukt0r_dev_cname_status" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "status.d3strukt0r.dev"
  proxied = true
  tags    = []
  ttl     = 1
  type    = "CNAME"
  zone_id = local.zone_ids["d3strukt0r.dev"]
  settings = {
    flatten_cname = false
  }
}

# OpenBao's UI and API, on the cluster behind Zitadel's login.
resource "cloudflare_dns_record" "d3strukt0r_dev_cname_openbao" {
  provider = cloudflare.personal

  content = "prod.d3strukt0r.dev"
  name    = "openbao.d3strukt0r.dev"
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
  content = "prod-old.d3strukt0r.dev"
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

# Zitadel re-checks the organisation domain periodically, so this stays.
resource "cloudflare_dns_record" "d3strukt0r_dev_txt_zitadel_challenge" {
  provider = cloudflare.personal

  comment  = "Zitadel Verify (organisation D3strukt0r)"
  content  = "\"7VUEdq0n1PFU9toPwnmMKiYRIrwNu3g3\""
  name     = "_zitadel-challenge.d3strukt0r.dev"
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
