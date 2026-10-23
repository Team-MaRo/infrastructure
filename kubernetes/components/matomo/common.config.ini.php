; <?php exit; ?> DO NOT REMOVE THIS LINE
; Matomo's settings beside its defaults (config/global.ini.php), only where they differ. Matomo
; reads this file before config.ini.php and never writes it; prepare.sh copies it into the code at
; every start and writes config.ini.php with the three secrets.

[database]
host = "mariadb-primary.mariadb.svc"
username = "matomo"
dbname = "matomo"
tables_prefix = "matomo_"
; What the installer chose and the tables have (database.yaml). Matomo's default is the old
; three-byte utf8: a table it creates then cannot reference its utf8mb4 tables - RebelOIDC's
; foreign key to the users failed with errno 150 (2026-10-23).
charset = "utf8mb4"
collation = "utf8mb4_uca1400_ai_ci"
; TLS, checked against the shared MariaDB's CA (trust-manager's ConfigMap mariadb-ca).
enable_ssl = 1
ssl_ca = "/etc/mariadb-ca/ca.crt"

[General]
trusted_hosts[] = "matomo.d3strukt0r.dev"
; Behind Cloudflare and Traefik, which end TLS: the visitor's address is in CF-Connecting-IP
; (the same trade-off as the WordPress chart's nginx), the scheme in Traefik's
; X-Forwarded-Proto - the pod itself only sees http. force_ssl redirects anything that is not
; https by that header.
proxy_client_headers[] = HTTP_CF_CONNECTING_IP
proxy_scheme_headers[] = HTTP_X_FORWARDED_PROTO
force_ssl = 1
; The installer cannot reach our MariaDB (it never uses TLS), and a public one would let anyone
; finish it; Matomo is installed elsewhere and imported (README "Installing").
enable_installer = 0
; Reports are built by the archive CronJob every hour, never by a visitor's browser.
enable_browser_archiving_triggering = 0
browser_archiving_disabled_enforce = 1
; Nothing installed or updated from the UI: the code comes from the image at every start.
enable_plugins_admin = 0
enable_auto_update = 0
; The config file is the pod's own; a change made in the UI is lost at the next start.
multi_server_environment = 1
; Where Zitadel sends the user after logging out (RebelOIDC passes it on, tofu/zitadel).
login_logout_url = "https://matomo.d3strukt0r.dev/"
noreply_email_address = "matomo@d3strukt0r.dev"
noreply_email_name = "Matomo"

; Scheduled email reports and invitations through mailomat, like the cluster's other mail
; (tofu/cloudflare README, "Sending through mailomat"); the password is in config.ini.php. Set
; here rather than in the UI, which would write config.ini.php - rebuilt at every start.
[mail]
transport = "smtp"
host = "smtp.mailomat.cloud"
port = 587
type = "LOGIN"
username = "matomo@mailomat.d3strukt0r.dev"
encryption = "tls"
