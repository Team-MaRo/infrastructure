# matomo

Matomo 5, the web analytics of every site, at `https://matomo.d3strukt0r.dev`: one instance for
all sites and their owners, each of whom sees only their own sites. Its UI logs in through Zitadel
(RebelOIDC plugin); its tracking script and endpoint (`/matomo.js`, `/matomo.php`) are public,
since every site's visitors call them. It replaces Google Analytics on the WordPress sites.

## Design

- **The stock image, no build** (`matomo:5-fpm`), assembled at every start like the WordPress
  chart's pods: the init container `prepare` runs `prepare.sh` - the code from the image into an
  emptyDir, the RebelOIDC plugin from GitHub, the configuration, the GeoIP database from the
  bucket, `core:update` and the plugin's activation. php-fpm runs it, nginx serves it
  (`default.conf`, after Matomo's own nginx configuration: PHP only through `index.php`,
  `matomo.php`, `piwik.php` and `js/index.php`, the code's directories denied).
- **Nothing on the pod survives, by design.** Matomo's state - visits, reports, users, sites,
  plugin settings, sessions - is in its database `matomo` in the shared MariaDB (TLS, checked
  against the MariaDB CA; covered by MariaDB's backups and point-in-time recovery). The GeoIP
  database is in the bucket `d3strukt0r-prod-matomo` (`tofu/objectstorage`).
- **Configuration in two files.** `common.config.ini.php` (in git) holds every setting that
  differs from Matomo's `global.ini.php`; `prepare.sh` writes `config.ini.php` at every start with
  only the database password, the salt and the SMTP password. See "What the UI may change".
- **One replica**, `matomo:5-fpm` and nginx on floating tags rolled out by Keel; the secrets
  restart the pod through Reloader. php-fpm 256Mi request / 1Gi limit at a 512M PHP limit.
- **The job** `matomo/php-fpm` (`pod-monitor.yaml`) brings the php-fpm alerts and dashboard of
  kube-prometheus-stack; Gatus checks `/matomo.php`.

### Settings beside Matomo's defaults (`common.config.ini.php`)

| Setting | Why |
|---|---|
| `[database]` host, user, name, `tables_prefix`, `enable_ssl`, `ssl_ca` | The shared MariaDB, over TLS checked against its CA |
| `[database] schema = "Mariadb"` | Matomo treats the server as MySQL otherwise: it skips the extra sort MariaDB needs for truncated reports ("Others" rows) and writes query time limits in MySQL's syntax, which MariaDB ignores |
| `trusted_hosts[]` | Matomo refuses requests for any other host name |
| `proxy_client_headers[] = HTTP_CF_CONNECTING_IP` | The visitor's address behind Cloudflare and Traefik; a client that skips Cloudflare can set it (the same trade-off as the WordPress chart's nginx) |
| `proxy_scheme_headers[] = HTTP_X_FORWARDED_PROTO`, `force_ssl` | TLS ends at Traefik, so the pod sees http; the scheme comes from Traefik's header. Without it Matomo would loop on `force_ssl`, and the Zitadel callback would be an `http://` address Zitadel refuses |
| `enable_installer = 0` | See "Installing" |
| `enable_browser_archiving_triggering = 0`, `browser_archiving_disabled_enforce = 1` | Reports come from the archive CronJob, never from a visitor's request |
| `enable_plugins_admin = 0`, `enable_auto_update = 0`, `multi_server_environment = 1` | The code comes from the image at every start; nothing is installed or changed from the UI |
| `login_logout_url` | Where Zitadel returns to after a logout (`post_logout_redirect_uris` in `tofu/zitadel`) |
| `noreply_email_address`, `noreply_email_name`, `[mail]` | Mail through mailomat (see "Email") |

### What the UI may change

Most of what the admin UI saves goes into the database and stays: websites and their settings
(URLs, currency, e-commerce, excluded IPs), users and rights, every plugin's settings (RebelOIDC's
too), the archiving setting, geolocation, privacy settings (IP anonymisation, deleting old raw
data), goals, segments, dashboards and scheduled email reports. A few things Matomo writes into
`config.ini.php` instead (the code paths that save it in 5.14.2), and those are **lost at the next
start**:

| In the UI | In `config.ini.php` | Here instead |
|---|---|---|
| Allgemeine Einstellungen → E-Mail-Server-Einstellungen | `[mail]` | `common.config.ini.php` |
| Allgemeine Einstellungen → Vertrauenswürdige Hostnamen | `trusted_hosts[]` | `common.config.ini.php` |
| Activating or deactivating a plugin | `[Plugins]` | off in the UI (`enable_plugins_admin = 0`); `prepare.sh` activates RebelOIDC |
| Some Tag Manager settings | `[TagManager]` | `common.config.ini.php`, if Tag Manager is ever used |

A custom logo is a file on the pod (`misc/user/`) and is lost the same way.

## Secrets

| OpenBao | Keys | 1Password | Used for |
|---|---|---|---|
| `secret/mariadb-apps/matomo` | `password` | [`MariaDB \| Prod \| matomo`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=y25mk6eqqqqgbxub7ikgodomdi&h=my.1password.com) | The database user (`database.yaml`) and Matomo |
| `secret/matomo` | `salt` | [`Matomo \| Prod \| salt`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=754ixrqhxttvt4ryqgucloapda&h=my.1password.com) | Matomo's `[General] salt` |
| `secret/matomo-s3` | `access-key`, `secret-key` | [`Hetzner \| S3 \| prod matomo`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=4vhpkhsjy4meb3pqhgvhu2zsge&h=my.1password.com) | The bucket `d3strukt0r-prod-matomo` |
| `secret/matomo-mailomat` | `password` | [`Mailomat \| Prod \| matomo`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=fq2kn2amieb2g7bmvjxurwbf6q&h=my.1password.com) | The SMTP login `matomo@mailomat.d3strukt0r.dev` |

The database and SMTP passwords are letters and digits only: they land in an ini file. **The salt never
changes**: it signs Matomo's sessions and hashes its API tokens, so a new salt logs everyone out
and breaks every token. The OIDC client secret is not here - it is entered once in the plugin's
settings, which Matomo keeps in its database (1Password [`Zitadel | Prod | matomo`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=aeul5lglmj3r7vj7arzk5v53cy&h=my.1password.com), like the other Zitadel clients).

The local `admin` superuser from the installation is the **break-glass login** if Zitadel or the
plugin fails (Matomo's own login form), its password in 1Password
[`Matomo | Prod | admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=sipf4klsp6n4xddugmcrgjkc5m&h=my.1password.com).

## Installing

Matomo's installer cannot reach our MariaDB - it never uses TLS (`enable_ssl => false` in its
`FormDatabaseSetup`, 5.14.2), and the user requires TLS - and a public installer could be
finished by anyone who brings a database of their own. So it is off (`enable_installer = 0`),
Matomo is installed once on the admin's machine and its database imported. Until then `prepare`
stops with "Matomo is not installed in its database yet". Once, in this order:

1. **The S3 key**: Hetzner console → Object Storage → Credentials, label `prod matomo`, into
   1Password `Hetzner | S3 | prod matomo`, its access key ID into `tofu/objectstorage`'s tfvars,
   then `tofu apply` there; nothing goes into the new bucket within 15 minutes
   (`tofu/objectstorage/README.md`). The DNS record (`tofu/cloudflare`) and the Zitadel app
   (`tofu/zitadel`, then regenerate its client secret) are applied the same way.
2. **The SMTP login** at mailomat: the user `matomo@mailomat.d3strukt0r.dev` ("Eigenes Passwort
   setzen" with a 1Password value of letters and digits), 1Password E-Mail item
   `Mailomat | Prod | matomo`. Then the database password into 1Password, and it, the S3 key and the
   SMTP password into OpenBao:

   ```sh
   op item create --account my.1password.com --vault Private --category login \
     --title 'MariaDB | Prod | matomo' username=matomo --generate-password='letters,digits,20' >/dev/null
   jq -n --arg p "$(op item get 'MariaDB | Prod | matomo' --account my.1password.com --vault Private --fields password --reveal)" \
     'if ($p|length)==0 then error("empty value - 1Password lookup failed") else {"password":$p} end' \
   | bao kv put secret/mariadb-apps/matomo -
   jq -n --arg a "$(op item get 'Hetzner | S3 | prod matomo' --account my.1password.com --vault Private --fields username)" \
         --arg s "$(op item get 'Hetzner | S3 | prod matomo' --account my.1password.com --vault Private --fields credential --reveal)" \
     'if ($a|length)==0 or ($s|length)==0 then error("empty value - 1Password lookup failed") else {"access-key":$a,"secret-key":$s} end' \
   | bao kv put secret/matomo-s3 -
   jq -n --arg p "$(op item get 'Mailomat | Prod | matomo' --account my.1password.com --vault Private --fields smtp_password --reveal)" \
     'if ($p|length)==0 then error("empty value - 1Password lookup failed") else {"password":$p} end' \
   | bao kv put secret/matomo-mailomat -
   ```

3. **Install locally** with the same Matomo version as the image (`docker run --rm --entrypoint grep
   matomo:5-fpm "const VERSION" /usr/src/matomo/core/Version.php`): a throwaway Compose project with
   `matomo:<that version>-apache` and `mariadb:11.8`, the installer at `http://localhost:8080`,
   the superuser `admin` with a password from 1Password `Matomo | Prod | admin`, the first
   website the first site that moves to Matomo.
4. **The salt** from the local `config/config.ini.php` (`[General] salt`) into 1Password
   `Matomo | Prod | salt` and OpenBao `secret/matomo` (`{"salt": ...}`).
5. **Push** this component; Argo CD creates the database and user, the pod waits in `prepare`.
6. **Import** the local database into the cluster's (on the primary, as root):

   ```sh
   docker compose exec -T db mariadb-dump -uroot -p<local root password> --single-transaction matomo > matomo.sql
   primary=$(kubectl --context d3strukt0r-prod-admin -n mariadb get mariadb mariadb -o jsonpath='{.status.currentPrimary}')
   kubectl --context d3strukt0r-prod-admin -n mariadb exec -i "$primary" -c mariadb -- \
     sh -c 'mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" matomo' < matomo.sql
   kubectl --context d3strukt0r-prod-admin -n matomo rollout restart deployment matomo
   ```

   Then delete `matomo.sql` and the local Compose project (`docker compose down -v`).
7. **The GeoIP database** once by hand, so the first start has it:
   `kubectl --context d3strukt0r-prod-admin -n matomo create job --from=cronjob/geoip geoip-initial`,
   then restart the deployment again.
8. **In Matomo**, logged in as `admin`:
   - Administration → System → Allgemeine Einstellungen → RebelOIDC: Authorize URL
     `https://auth.d3strukt0r.dev/oauth/v2/authorize`, Token URL
     `https://auth.d3strukt0r.dev/oauth/v2/token`, Userinfo URL
     `https://auth.d3strukt0r.dev/oidc/v1/userinfo`, End session URL
     `https://auth.d3strukt0r.dev/oidc/v1/end_session`, client ID (`tofu output -raw
     matomo_client_id` in `tofu/zitadel`) and secret, scope `openid email profile`, "Erstelle
     automatisch einen neuen Account …" on, "Deaktiviere Passwortbestätigung" on (OIDC users know
     no Matomo password), "Feingranulare Zugriffsrechte nutzen" on, "Name" `OpenID Connect` (the
     login button's label, AGENTS.md; set 2026-10-29) - then **Speichern**, confirmed
     with the admin password.
   - Administration → System → Geolokalisierung: the DBIP / GeoIP 2 provider, automatic updates
     **never** (the CronJob updates it; Matomo's own download would land on the pod and vanish).
   - Log in once through "OpenID Connect", then as `admin` give that user superuser access
     (Administration → Benutzer).

## Email

Scheduled email reports (and invitations) go out through mailomat like the cluster's other mail
(`tofu/cloudflare/README.md`, "Sending through mailomat"): from `Matomo <matomo@d3strukt0r.dev>`,
logged in as `matomo@mailomat.d3strukt0r.dev` on `smtp.mailomat.cloud:587` with STARTTLS. The
reports are sent by the archive CronJob, so both it and the web pod get the password. They count
against mailomat's 200 mails a month shared by every app. The settings are in
`common.config.ini.php`, not in the UI's "E-Mail-Server-Einstellungen", which would write
`config.ini.php` - rebuilt at every start.

## Login through Zitadel

RebelOIDC 5.1.6 with the project "Matomo" in `tofu/zitadel` (README there, "Matomo"): a user is
created at their first login, and their site rights are set at every login from their Zitadel
roles `matomo:<siteID>:<view|write|admin>` - rights given inside Matomo vanish at the next login,
so they are always given in Zitadel. `matomo:superuser` only gets its holder past Zitadel's role
check; the superuser flag is set in Matomo.

**The plugin is patched** at every start (`prepare.sh`): it decodes tokens as plain base64
instead of base64url, so a login fails at random whenever a token holds `-` or `_` (found
2026-10-23 in the local trial; upstream issue #47, open since 2026-03-20, the same fix in it). If
a new plugin version no longer has the line, `prepare` stops with "the base64url patch did not
apply": check whether #47 is fixed in that version, then drop the patch.

What each right allows (Matomo's FAQ): `view` sees the reports and makes its own segments;
`write` also creates goals, funnels and the like; `admin` also changes the site's settings and
uses the GDPR tools. Only a superuser adds sites, manages plugins and global settings.

## Adding a site

1. As superuser: Administration → Websites → Verwalten → add the site; note its ID.
2. Its roles in `tofu/zitadel/project_matomo.tf`, e.g. `"matomo:3:admin" = "arepazo.ch admin"`,
   and the owner's grant - or a project grant with only that site's roles to the owner's
   organisation, where the owner hands them out. Apply.
3. For a WordPress site: its ID as `matomo.siteId` in its values, which brings the tracking
   (the chart's README, "Web analytics"). For a shop also the token its orders are sent with:
   Administration → System → Benutzer → a **local** user `<site>-tracking` (RebelOIDC never
   touches users that do not log in through it), **Schreiben** on that site only; logged in as it,
   Persönlich → Sicherheit → Auth-Token erstellen. The token into 1Password
   `Matomo | Prod | <site>-tracking` and OpenBao `secret/<site>-matomo` (`token`), read by the site
   as `MATOMO_TOKEN`. Elsewhere: the tracking code from Administration → Websites → Tracking-Code.

## Importing past visits from a site's logs

A WordPress site's visits from before its tracking started are only in its nginx log, which
Loki keeps 30 days (`{namespace="<site>", container="nginx"}`, every page request with the
visitor's address and user agent; Traefik's access log has only failed and slow requests). Done
on 2026-10-23 for Arepazo and robines-portfolio, back to the move to the cluster (2026-10-03).

1. **Export** through `kubectl --context d3strukt0r-prod-admin -n loki port-forward svc/loki
   13100:3100`: `/loki/api/v1/query_range` per UTC day, direction forward, 5,000 lines per page
   (Loki's limit), the next page from the last timestamp; keep only access-log lines; compare each
   day's count with `count_over_time`. The files hold visitors' addresses: keep them out of git and
   out of folders a cloud syncs (`~/Documents`, `~/Desktop`).
2. **Cut** each site's lines at its first tracked visit, or visits count twice. **Look at the
   addresses before dropping any**: nginx takes the visitor from Cloudflare's `CF-Connecting-IP`
   only since 2026-10-20 22:51 (c555dd8). Before, every request carried Traefik's cluster address
   (10.x) - visitors included -, after it a 10.x address means a request past Cloudflare to port
   443, a scanner. Lines without the visitor's address still count as visits, without location.
   (On 2026-10-23 all 10.x visits were deleted again through the GDPR API as scanners, which took
   the real visits of 2026-10-03..20 with them; Google Analytics' history filled those days -
   "Google Analytics history".)
3. **Import** with Matomo's own `misc/log-analytics/import_logs.py` (copy it out of the pod, which
   has no Python), on the admin's machine, with a temporary token of the `admin` user from the
   clipboard (`--token-auth="$(pbpaste)"`), deleted afterwards. A dry run first (`--dry-run`
   still needs the token). Options: `--url=https://matomo.d3strukt0r.dev --idsite=<n>
   --log-format-name=ncsa_extended --recorders=2`, `--useragent-exclude=` for `kube-probe`,
   `WordPress/`, `WordPress.com`, `Jetpack`, `Gatus`, `curl/`, `Go-http-client`, `python`,
   `Scrapy` and whatever else the log shows, `--exclude-path=` for `/wp-admin*`, `/wp-json*`,
   `/wp-login.php*`, `/wp-cron.php*`, `/xmlrpc.php*`, `/m/*`, `/wp-content/*`, `/wp-includes/*`,
   `*/feed*`, `/robots.txt`, `/healthz`; its defaults leave out bots, static files, errors and
   redirects. Its summary counts `/robots.txt` under "downloads" before the path filter drops it -
   "imported successfully" is what goes in. A shop's site excludes the URL parameter `key`
   first (the site's settings, "Ausgeschlossene Parameter"): WooCommerce's order key is in the
   thank-you URLs of the raw log. Matomo notes the imported days and the `archive` CronJob
   rebuilds their reports.
4. **Clean up**: delete the token in Matomo (Persönlich → Sicherheit) and the exported files.

Calls to Matomo's API from a script need a user agent of their own: Cloudflare refuses Python's
default (`Python-urllib`, 403). Since 2026-10-22 Varnish answers cached pages before nginx, so a
later import undercounts those.

## Google Analytics history

Before Matomo the sites were measured by Google Analytics 4 (the user's account "D3strukt0r").
Their history came in on 2026-10-24 with the free plugin **GoogleAnalyticsImporter** (pinned in
`prepare.sh`), each property into its **existing** Matomo site, from the property's creation up
to 2026-10-20 - the last day without raw visits in Matomo (its own data starts 2026-10-21), so no
day is both imported and tracked: Arepazo (property 287677108, from 2021-09-25), Robines Space
(508744479, from 2025-10-14) and site 3 (from 2021-09-26). Imported were visits, pages,
referrers, devices, locations and the key events as goals (`purchase`, for Robines Space also
`qualify_lead` and `close_convert_lead`); **no custom dimensions** - MonsterInsights had created
some with personal data in every property (`email_address`, `tel_number`, `user_id`,
`wp_user_id`).

Two properties of sites that no longer run came in on 2026-10-27 as **new sites**, over their
whole life: Wund Expertin Plus (property 287841226, data from 2021-09-28 to 2026-05-19, site 4)
and Weleda WebCenter Text Export (227190878, site 5 - the property never received data, so the
site holds empty days). Only superusers see them. The account's last property, D3strukt0r
(279220328), had data on three days in July 2021 and was left out.

- **It imports reports, not visits**: one finished report per day (visits, pages, referrers,
  ...), no raw visits. Matomo would rebuild an invalidated day from its raw visits - zero for these
  days - and only the loaded plugin stops that (`Archiving.isRequestAuthorizedToArchive`). So
  `prepare.sh` loads it in the **archive pod too**, and it stays installed **as long as the
  imported history matters**, even with Google long gone (Upgrades: Matomo 6 needs a release of
  it for 6 first).
- **No Google credentials are left** (2026-10-27: client configuration removed in Matomo, Google
  Cloud project shut down, every property in the trash). The plugin encrypts its OAuth client and
  token in Matomo's database with `[GoogleAnalyticsImporter] encryption_key`; without one it
  generates a key into `config.ini.php`, which `prepare.sh` rewrites at every start, so the
  credentials would become unreadable at the next restart. Another import therefore first needs
  a key that stays: `openssl rand -base64 32` into a 1Password item and into `secret/matomo` as
  `ga-importer-key`, and that key's entry in `external-secrets.yaml` (`secretKey` and `property`
  `ga-importer-key`). Both pods read it as an optional variable, and `prepare.sh` writes the
  `[GoogleAnalyticsImporter]` section only when it is set. The entry cannot stay without the
  value: External Secrets fails the whole Secret over one missing field.
- **Existing sites are not what the plugin offers**: its UI ("Einen Import aus Google Analytics 4
  (GA4) planen") and its command always create a new site; only its WordPress variant imports
  into the current one. `ga-prepare-site.php` (here, run by hand) does what that variant does:
  moves the site's creation date back to the first imported day (reports before it stay hidden)
  and writes the importer's status for the site, so the command resumes into it.

### Importing a property

1. **Google** (Google Cloud Console, the GA account's owner): a new project of its own (deleted
   afterwards, so nothing else is touched), "APIs & Dienste" → enable the **Google Analytics Data
   API** and **Google Analytics Admin API**. "Google Auth Platform" → "Erste Schritte":
   "App-Information" (Anwendungsname, Nutzersupport-E-Mail), "Zielgruppe" **Extern**,
   "Kontaktdaten", "Fertig" (accept the user data policy) → "Erstellen". "Zielgruppe" →
   "Testnutzer" → "Add users": the owner's Google address (Enter, then "Speichern"); the status
   stays "Testen" (no verification; a token lasts 7 days, enough for an import). "Clients" →
   "Client erstellen": "Webanwendung", "Autorisierte JavaScript-Quellen"
   `https://matomo.d3strukt0r.dev`, "Autorisierte Weiterleitungs-URIs"
   `https://matomo.d3strukt0r.dev/index.php?module=GoogleAnalyticsImporter&action=processAuthCode`
   → "Erstellen" → download the JSON into the 1Password item of the encryption key (above). It can
   take minutes to hours until a new client works.
2. **Matomo** (superuser): Administration → System → **Google-Analytics-Import** → "UPLOAD" the
   JSON → "AUTORISIEREN" (Google warns "Google hat diese App nicht überprüft": "Weiter"). A
   "Sicherheitsprüfung" error means the page's one-time token expired - reload and click at once.
   Do **not** use the form "Einen Import … planen" for an existing site.
3. **Check the property** (GA → Verwaltung): Property-Details (time zone and currency like the
   site's), Ereignisse → "Schlüsselereignisse" (they become goals), "Benutzerdefinierte
   Definitionen" (custom dimensions - see step 5). The creation date is not shown there;
   `GoogleAnalyticsImporter\Google\AuthorizationGA4::getAdminClient()->getProperty()` returns it.
4. **Backup**: dump the `matomo` database from the MariaDB replica (as in the chart README's
   upgrade runbook, for `matomo` only).
5. **Prepare and import**, in the web pod (`kubectl --context d3strukt0r-prod-admin -n matomo cp
   ga-prepare-site.php <pod>:/tmp/ -c php-fpm`): `php /tmp/ga-prepare-site.php <idSite>
   <propertyId> <created> <last day without raw visits>` (dry run), then with `apply`;
   `php console plugin:deactivate CustomDimensions` (the importer then skips custom dimensions
   altogether - a restart activates it again); then in the background `nohup php console
   googleanalyticsimporter:import-ga4-reports --idsite=<n> --dates=<created>,<end> >
   /tmp/gaimport-<n>.log 2>&1 &`. A site that no longer runs gets a new Matomo site instead: no
   prepare script, and the first run takes `--property=properties/<propertyId>` without
   `--idsite` (the bare number fails with "Could not map bindings") and logs "Created new site
   with ID = <n>"; that site has the property's name as its address - set the real one in
   "Websites verwalten" - and every later run adds `--idsite=<n>`. It works backwards from the
   end date, 1-8 seconds per day with data (under one without), and ends with "Done in …" and its
   own archiving of weeks, months and years ("Error: error or warning logs detected" there only
   repeats the CustomDimensions warning). Google's daily quota pauses it ("rate limit" / "quota"
   in the log), and so does a restart of the pod: **run the same command again** (after a quota,
   the next day) - it resumes where it stopped. Google also breaks it off now and then for a
   moment ("The service is currently unavailable", code 14, or a cURL timeout after 60 s, then
   "Error encountered, aborting."), and the next run is refused for about 5 minutes ("An import
   is currently in progress"). A loop in the pod resumes it unattended - ten tries stay under the
   API's ten server errors per project and hour: `nohup sh -c 'for i in 1 2 3 4 5 6 7 8 9 10; do
   php console googleanalyticsimporter:import-ga4-reports --idsite=<n> --dates=<created>,<end> >
   /tmp/gaimport-<n>-$i.log 2>&1; grep -qE "Error encountered|currently in progress"
   /tmp/gaimport-<n>-$i.log || break; sleep 120; done' > /dev/null 2>&1 &`. The plugin's own
   hourly resume does not help here: it runs among `core:archive`'s scheduled tasks in the
   short-lived `archive` pod and starts the import in that pod's background, which ends with the
   pod. Afterwards `php console plugin:activate CustomDimensions`.
6. **Check**: the importer's status `finished` (option `GoogleAnalyticsImporter.importStatus_<n>`),
   visits per month against GA's, the first tracked days unchanged. An aborted run archives
   weeks, months and years with only the days it had, and later runs do not rebuild them (site 4's
   2022 showed 185 of 388 visits): mark them outdated and rebuild, leaving the days alone - they
   are the imported reports themselves - with `php console core:invalidate-report-data
   --sites=<n> --dates=<created>,<end> --periods=week,month,year`, then `php console core:archive
   --force-idsites=<n>`. The deactivated plugin did not stop the import of site 4 from configuring
   five of MonsterInsights' dimensions ("Email Address" among them), without values: list them
   with `CustomDimensions.getConfiguredCustomDimensions` and deactivate them in "Benutzerdefinierte
   Dimensionen" - Matomo cannot delete one.
7. **Wind down** when all properties are in: "Client Konfiguration entfernen" → "ENTFERNEN" in
   Matomo, delete the Google Cloud project, then the encryption key: its ExternalSecret entry
   first, then the OpenBao field and the 1Password item. The plugin stays.

## GeoIP

Visitor locations come from DB-IP's free city database (IP to City Lite, CC BY 4.0 - DB-IP asks
for "IP Geolocation by DB-IP" wherever the data is shown publicly; Matomo's reports are not).
The CronJob `geoip` fetches it on the 3rd of each month into the bucket
(`geoip/DBIP-City-Lite.mmdb`); `prepare` copies it into `misc/` at every start, where Matomo
looks for that file name. A failed run raises `KubeJobFailed`; Matomo keeps the previous
database until then. If the bucket fails at a start, `prepare` fetches the current month's file
straight from DB-IP instead (a warning in its log); only if that fails too does Matomo run
without it, guessing the country from the browser's language. To fill the bucket now:
`kubectl --context d3strukt0r-prod-admin -n matomo create job --from=cronjob/geoip geoip-manual`.

## Archiving

The CronJob `archive` runs `core:archive` every hour at :05 in a pod of its own (`prepare.sh
archive`: code and configuration only), with a 1G PHP limit (Matomo asks for 768M when
archiving). Reports of today appear within the hour. A failed run raises `KubeJobFailed`; the
next run catches up.

The official image has no `ps`, so Matomo cannot run archiving as parallel processes of its own
(system check "Cron einrichten - Prozesse via CLI steuern: nicht unterstützt", optional) and
sends the work as HTTP requests to `--url` instead ("Async process archiving not supported, using
curl requests" in the job's log) - out through Cloudflare and back. Accepted (2026-10-26): a run
takes about half a minute for three sites, and if the public address is down the next run catches
up. Copying `ps` into the pod from another image would be fragile; if archiving ever needs the
parallel processes (many sites, long runs), the way is an image of our own with `procps` added.

## Upgrades

- **Matomo 5.x** arrives through Keel (floating `5-fpm`), and `prepare` runs `core:update` before
  the new version serves. **Matomo 6** waits for RebelOIDC (it declares `matomo <6.0.0-b1`) and
  for a GoogleAnalyticsImporter release for 6 - without the importer the Google history is
  rebuilt as zeros ("Google Analytics history"); 5.x gets fixes for at least 12 months after 6.0
  (Matomo's LTS policy).
- **RebelOIDC** is pinned in `prepare.sh`; a new version is a one-line change there, after
  checking issue #47 (above). So is **GoogleAnalyticsImporter** (`gaimporter=`); its versions
  are listed on plugins.matomo.org.
