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
  `www.wundexpertinplus.com` at the import (below).
- **The plugins and the theme prod-old had active** (Customify; Polylang, Yoast SEO, WPForms
  Lite, WP Mail SMTP, Jetpack, Loginizer, Bulk Delete, Duplicator, MonsterInsights, WordPress
  Importer), at today's versions from wordpress.org - prod-old's were from 2024 (Jetpack 13.7)
  and are not kept. Language `de_CH`; Polylang's languages are in the database.
- **Keys and salts are prod-old's**, so its stored sessions and password hashes stay valid. Its
  three administrators: `admin` is the local break-glass account and gets a new password from
  1Password at the import (step 5 below); `D3strukt0r` is linked to Zitadel with the site's
  Zitadel login (the step after the import); `stephan.buergi`, the owner's, keeps its own
  password until the clean-up decides with the owner.
- **Mail leaves through mailomat** (`web@mailomat.wundexpertinplus.com`): the chart's constants
  override WP Mail SMTP's stored settings, which still name prod-old's provider.
- **Login through Zitadel** (the project "Wund Expertin Plus" in `tofu/zitadel`,
  `project_wundexpertinplus.tf`), with the button first on `wp-login.php`; `D3strukt0r` is linked
  to its Zitadel user and has only a random password nobody knows (chart README "Linking an
  existing account"). The break-glass `admin` and `stephan.buergi` keep the form.
- Not yet: Matomo, the object cache, the consent banner, Content through Claude - "Next".

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
   engines. Jetpack stays disconnected until "Next" decides about it.

5. **A new password for the break-glass `admin`**, generated in its existing 1Password item
   (which also gets the new address) and piped into WordPress (never on a command line); it also
   ends every open session of `admin`:

   ```sh
   op item edit 'Wund Expertin Plus CMS | Prod | Admin' --account my.1password.com --generate-password='letters,digits,symbols,20' --url https://old.wundexpertinplus.com/wp-login.php >/dev/null
   op item get 'Wund Expertin Plus CMS | Prod | Admin' --account my.1password.com --fields password --reveal \
     | kubectl --context d3strukt0r-prod-admin -n wundexpertinplus exec -i deploy/web -c php-fpm -- php -r 'require "/var/www/html/wp-load.php"; $u = get_user_by("login", "admin"); wp_set_password(trim(stream_get_contents(STDIN)), $u->ID); echo "ok\n";'
   ```

## Next

The clean-up, each step decided on its own: `stephan.buergi` (with the owner: a Zitadel user and
grant, then linked), the plugins that are tools rather than features
(Bulk Delete, Duplicator, WordPress Importer) and those the cluster replaces (MonsterInsights by
Matomo, Loginizer by the login rate limit, maybe Jetpack); the cluster's standards (posts by a
linked author instead of `admin`, Matomo, the consent banner, the object cache, Content through
Claude); the leftover tables of plugins gone long ago (LiteSpeed
Cache and others) and the 15 tables still in three-byte `utf8mb3`.

## Secrets

| OpenBao | 1Password | Holds |
|---|---|---|
| `secret/mariadb-apps/wundexpertinplus` | [`MariaDB | Prod | wundexpertinplus`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=iuehqp5c4rhsnawqp5zhvjwgty&h=my.1password.com) | the database password |
| `secret/wundexpertinplus` | [`Wund Expertin Plus CMS | Prod | App`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=6uuspv364f2hs7apwlmevbcujy&h=my.1password.com) | the eight keys and salts, prod-old's |
| `secret/wundexpertinplus-s3` | [`Hetzner | S3 | prod wundexpertinplus`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=zyx2nlhgljreoo2fsjin3asxde&h=my.1password.com) | the key that writes the uploads |
| `secret/wundexpertinplus-uploads-proxy-s3` | [`Hetzner | S3 | prod wundexpertinplus uploads-proxy`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=efxryvkeefiviz5rcpdtufguye&h=my.1password.com) | the uploads proxy's read-only key |
| `secret/wundexpertinplus-mailomat` | [`Mailomat | Prod | wundexpertinplus`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=2nr3n556yfo6aejcrmfqdzgaom&h=my.1password.com) | the SMTP login and the API token mailomat-watch reads |
| `secret/wundexpertinplus-oidc` | [`Zitadel | Prod | wundexpertinplus`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=2kjoqiwcmo6dtdyuseajil6ymm&h=my.1password.com) | the client secret of the login through Zitadel |
