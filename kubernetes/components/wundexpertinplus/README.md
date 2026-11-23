# wundexpertinplus

The old WordPress site of Wund Expertin Plus, public at `https://old.wundexpertinplus.com`
(`kubernetes/clusters/prod/wundexpertinplus.yaml`, namespace `wundexpertinplus`).
`wundexpertinplus.com` and `www.wundexpertinplus.com` are a Cloudflare Worker - the live site, a
rebuild in its own repository - and have nothing to do with this. It ran on prod-old under Docker
Compose until 2026-05-19 and came back from that day's database backup and the uploads kept in
`server-config`.

It is the WordPress chart ([`../../charts/wordpress/README.md`](../../charts/wordpress/README.md):
what runs, the design, the runbooks) with the values in `values.yaml`.

The database is `wundexpertinplus` in the shared MariaDB, the uploads are in the bucket
`d3strukt0r-prod-wundexpertinplus` (`tofu/objectstorage`).

## As it was

- **Its own address, `old.wundexpertinplus.com`.** The content's URLs were rewritten from
  `www.wundexpertinplus.com` at the import (below). On prod-old the site also answered on
  `wundexpertinplus.ch` and `wundexpertin-bl.ch`, and on `wundexpertinplus.d3strukt0r.dev`
  (now GitHub Pages); the two `.ch` domains still exist but resolve nothing - what they are and
  what is open about them is in `tofu/cloudflare/README.md`. The admin address
  `cms@wundexpertinplus.ch` from those days receives no mail.
- **The theme prod-old had active** (Customify), at today's version from wordpress.org, language
  `de_CH`; Polylang's languages (only German) are in the database.
- **Plugins**: what the site uses - Polylang, Yoast SEO, WPForms Lite, WP Mail SMTP, Jetpack -
  at today's versions (prod-old's were from 2024, Jetpack 13.7). prod-old's others were removed
  on 2026-11-23, each deactivated and its data deleted first (chart README "Plugins"): Bulk
  Delete, Duplicator and WordPress Importer (tools; backups are MariaDB's and the bucket's),
  MonsterInsights (Google Analytics, which is gone; Matomo to come) and Loginizer (Jetpack's
  Protect guards the login). What their uninstall routines left - MonsterInsights' options, cache
  table and cron hooks - went by hand, together with the 211 options and two tables LiteSpeed
  Cache had left years before (among them an old Cloudflare token, revoked long since).
- **Keys and salts are prod-old's**, so its stored sessions and password hashes stay valid. Its
  three administrators: `admin` is the local break-glass account and gets a new password from
  1Password at the import (step 5 below); `D3strukt0r` is linked to Zitadel with the site's
  Zitadel login (the step after the import); `stephan.buergi`, the owner's, keeps its own
  password.
- **The content is the owner's** (2026-11-23): every page, upload and menu item prod-old had under
  `admin` belongs to `stephan.buergi`, so the break-glass account appears nowhere public. Its
  public name is not its login: slug `inhaber`, display name "Stephan" - the user list
  (`/wp-json/wp/v2/users`) and author pages would otherwise hand out the login of an account that
  logs in with a password.
- **Jetpack's modules as on the other sites** (2026-11-23): Protect (brute force, firewall,
  account protection), Monitor, Activity Log, Backup, Blocks, Notifications, JSON API, Site
  Verification stay; Secure Sign On (a way in past Zitadel), Stats, Blaze, AI, Newsletter, the
  image and asset CDNs (visitors would load from WordPress.com), comments and the blog extras
  were switched off. The CDNs' switches did not save in wp-admin; `Jetpack::deactivate_module()`
  through `php -r` did. Jetpack is connected to WordPress.com afresh, as
  `old.wundexpertinplus.com` (not as prod-old's site of `www.`).
- **Counted in Matomo** (site 4, since 2026-11-23; the chart's "Web analytics"): cookieless for
  everyone, with cookies for whoever allows statistics in the WPConsent banner
  (`wpconsent-cookies-banner-privacy-suite` with `wp-consent-api`), set up as robines-portfolio's
  (2026-11-23: its German texts, the categories Notwendig/Statistik/Marketing, the service Matomo
  with its three cookies, the WordPress cookies, the page "Cookie-Richtlinie" - all in the
  database; robines' X/Twitter service left out). Site 4 is the one Google
  Analytics' history went into (Matomo's README, "Google Analytics history"); it keeps
  `https://wundexpertinplus.com` as its address, with `https://old.wundexpertinplus.com` added as
  a second one, and e-commerce switched off (no shop). **Reports
  in wp-admin** come from Connect Matomo (`wp-piwik`), set up in its settings (kept in the
  database): mode HTTP API, `https://matomo.d3strukt0r.dev`, site 4, the token of Matomo's user
  `wundexpertinplus-reports` (**view** on site 4 only, 1Password
  [`Matomo | Prod | wundexpertinplus-reports`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=435kagemljc7i3fazb227vpou4&h=my.1password.com)), and its tracking code
  **off** - the mu-plugin counts, a second tracker would count every visit twice.
- **Jetpack Protect** (the vulnerability scan) and **Jetpack Boost** (critical CSS, generated by
  hand in wp-admin - regenerate it after theme or plugin changes, which every pod start can
  bring), as on the other sites.
- **Every table in `utf8mb4_unicode_520_ci`** since 2026-11-23, as on every site (chart README,
  the database's collation): 14 tables were still three-byte `utf8mb3`, from the original install
  at the old hosting (the core tables, WordPress converts them only on upgrades across 4.2) and
  from Yoast.
- **Mail leaves through mailomat** (`web@mailomat.wundexpertinplus.com`): the chart's constants
  override WP Mail SMTP's stored settings, which still name prod-old's provider.
- **Login through Zitadel** (the project "Wund Expertin Plus" in `tofu/zitadel`,
  `project_wundexpertinplus.tf`), with the button first on `wp-login.php`; `D3strukt0r` is linked
  to its Zitadel user and has only a random password nobody knows (chart README "Linking an
  existing account"). The break-glass `admin` and `stephan.buergi` keep the form; the owner's
  account stays a local one with its own password (decided 2026-11-23).
- **No object cache** (decided 2026-11-23): seven pages gain little from one, and the site shows
  how a WordPress site runs without it. It can get one later like the other sites (the chart's
  `objectCache`).
- **Content through Claude** (since 2026-11-23; the chart's "Content through Claude"): the app
  `wundexpertinplus-claude` in `tofu/zitadel`. claude.ai / Claude Desktop: Add custom connector,
  URL `https://old.wundexpertinplus.com/wp-json/mcp/mcp-adapter-default-server`, "Use your own
  OAuth client" with client ID `400934183486161647`, no secret. Only a Zitadel user linked to a
  WordPress account gets in, so `stephan.buergi` cannot. Claude Code:

  ```shell
  claude mcp add --transport http --scope user --client-id 400934183486161647 --callback-port 33418 wundexpertinplus https://old.wundexpertinplus.com/wp-json/mcp/mcp-adapter-default-server
  ```

## Bringing it back (done once, 2026-11-23)

From the gitignored data in `~/Projects/server-config`. The site was deployed first; WordPress
answers with its installer until the database is in.

1. **The database**, checked and imported into the primary, with the address rewritten on the
   way. `www.` and `old.` are the same length, so a plain replacement keeps WordPress's
   serialized options valid (their stored lengths do not change) - with a different length it
   would need WP-CLI's `search-replace`:

   ```sh
   cd ~/Projects/server-config/spaces/db-backup-d3strukt0r/prod/wundexpertinplus
   shasum -a 1 -c mariadb_db_db_20260519-181254.sql.gz.sha1
   primary=$(kubectl --context d3strukt0r-prod-admin -n mariadb get mariadb mariadb -o jsonpath='{.status.currentPrimary}')
   gunzip -c mariadb_db_db_20260519-181254.sql.gz | sed 's/www\.wundexpertinplus\.com/old.wundexpertinplus.com/g' \
     | kubectl --context d3strukt0r-prod-admin -n mariadb exec -i "$primary" -c mariadb -- sh -c 'mariadb -uroot -p"$MARIADB_ROOT_PASSWORD" wundexpertinplus'
   ```

2. **The uploads** into the bucket, without WPForms' cache (not earlier than 15 minutes after the
   bucket's apply, `tofu/objectstorage/README.md`):

   ```sh
   aws --profile d3strukt0r-hetzner --endpoint-url https://nbg1.your-objectstorage.com s3 sync \
     ~/Projects/server-config/digitalocean/prod/wundexpertinplus/uploads/ s3://d3strukt0r-prod-wundexpertinplus/uploads/ \
     --exclude 'wpforms/cache/*'
   ```

3. **A restart**, so the init containers update the database to today's WordPress
   (`wp core update-db`) and load the language packs:
   `kubectl --context d3strukt0r-prod-admin -n wundexpertinplus rollout restart deployment web`.

4. **In wp-admin**: Einstellungen → Lesen → "Suchmaschinen davon abhalten, diese Website zu
   indexieren" on - an archive under a second name must not compete with the live site in search
   engines.

5. **A new password for the break-glass `admin`**, generated in its existing 1Password item
   (which also gets the new address) and piped into WordPress (never on a command line); it also
   ends every open session of `admin`:

   ```sh
   op item edit 'Wund Expertin Plus CMS | Prod | Admin' --account my.1password.com --generate-password='letters,digits,symbols,20' --url https://old.wundexpertinplus.com/wp-login.php >/dev/null
   op item get 'Wund Expertin Plus CMS | Prod | Admin' --account my.1password.com --fields password --reveal \
     | kubectl --context d3strukt0r-prod-admin -n wundexpertinplus exec -i deploy/web -c php-fpm -- php -r 'require "/var/www/html/wp-load.php"; $u = get_user_by("login", "admin"); wp_set_password(trim(stream_get_contents(STDIN)), $u->ID); echo "ok\n";'
   ```

## Secrets

| OpenBao | 1Password | Holds |
|---|---|---|
| `secret/mariadb-apps/wundexpertinplus` | [`MariaDB | Prod | wundexpertinplus`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=iuehqp5c4rhsnawqp5zhvjwgty&h=my.1password.com) | the database password |
| `secret/wundexpertinplus` | [`Wund Expertin Plus CMS | Prod | App`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=6uuspv364f2hs7apwlmevbcujy&h=my.1password.com) | the eight keys and salts, prod-old's |
| `secret/wundexpertinplus-s3` | [`Hetzner | S3 | prod wundexpertinplus`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=zyx2nlhgljreoo2fsjin3asxde&h=my.1password.com) | the key that writes the uploads |
| `secret/wundexpertinplus-uploads-proxy-s3` | [`Hetzner | S3 | prod wundexpertinplus uploads-proxy`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=efxryvkeefiviz5rcpdtufguye&h=my.1password.com) | the uploads proxy's read-only key |
| `secret/wundexpertinplus-mailomat` | [`Mailomat | Prod | wundexpertinplus`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=2nr3n556yfo6aejcrmfqdzgaom&h=my.1password.com) | the SMTP login and the API token mailomat-watch reads |
| `secret/wundexpertinplus-oidc` | [`Zitadel | Prod | wundexpertinplus`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=2kjoqiwcmo6dtdyuseajil6ymm&h=my.1password.com) | the client secret of the login through Zitadel |
