# d3strukt0r-portfolio

The WordPress portfolio at `https://old.d3strukt0r.dev`
(`kubernetes/clusters/prod/d3strukt0r-portfolio.yaml`, namespace `d3strukt0r-portfolio`).
`d3strukt0r.dev` and `www.d3strukt0r.dev` are a Cloudflare Worker - the portfolio rebuild, in its
own repository - and have nothing to do with this. Unlike the other WordPress sites it came from
nowhere: a fresh install on 2026-11-24, not a copy from prod-old.

It is the WordPress chart ([`../../charts/wordpress/README.md`](../../charts/wordpress/README.md):
what runs, the design, the runbooks) with the values in `values.yaml`.

The database is `d3strukt0r_portfolio` in the shared MariaDB, the uploads are in the bucket
`d3strukt0r-prod-d3strukt0r-portfolio` (`tofu/objectstorage`), the login is the Zitadel project
"D3strukt0r Portfolio" (`tofu/zitadel`, `project_d3strukt0r_portfolio.tf`), the name's record is
in `tofu/cloudflare`.

## What it has

- **German and English** with Polylang: English is WordPress' own language, German comes as
  `de_CH` packs. Which one is the default is set in Polylang's settings (in the database).
- **Login through Zitadel**, the button first on `wp-login.php` (chart README "Login through
  Zitadel"). The owner's account was never local: its first login through Zitadel created it, as
  administrator from the project role, so it has no password at all. `admin` is the local
  break-glass account, its password in 1Password
  [`D3strukt0r Portfolio CMS | Prod | Admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=razxvuhrdiur32yu7xsi7xtacm&h=my.1password.com),
  and WordPress' admin address `d3strukt0r-portfolio-admin@d3strukt0r.dev`.
- **Content through Claude** (the chart's "Content through Claude"): the app
  `d3strukt0r-portfolio-claude` in `tofu/zitadel`. Claude Code:

  ```shell
  claude mcp add --transport http --scope user --client-id 401010431017222895 --callback-port 33418 d3strukt0r-portfolio https://old.d3strukt0r.dev/wp-json/mcp/mcp-adapter-default-server
  ```
- **Mail leaves through mailomat** as `d3strukt0r-portfolio@mailomat.d3strukt0r.dev`, a login of
  the domain Zitadel and Matomo send through too. The chart wants every mailomat site to bring
  its domain's API token to mailomat-watch, so this one brings the shared domain's token a second
  time; mailomat-watch keys tokens by domain, so the domain is still watched once.
- **Counted in Matomo** as site 6 (since 2026-11-24, `https://old.d3strukt0r.dev`, site search
  on; the chart's "Web analytics"), with WPConsent's banner and Connect Matomo's reports as on
  the other sites.
- **No object cache, no page cache**, like wundexpertinplus: a small site gains little from them.
- **Hidden from search engines** (Einstellungen → Lesen) while it is being built.

## Installing it (done once, 2026-11-24)

The chart serves WordPress' installer until the database holds a site, and the installer is
public: whoever reaches it first could claim the site. So the site is installed by command
right after its first pod is Ready, never in the browser.

1. **The break-glass account's password**, generated in 1Password:

   ```sh
   op item create --category Login --account my.1password.com --vault Personal --title 'D3strukt0r Portfolio CMS | Prod | Admin' --generate-password='letters,digits,symbols,20' --url https://old.d3strukt0r.dev/wp-login.php username=admin 'Profil.E-Mail[email]=d3strukt0r-portfolio-admin@d3strukt0r.dev' >/dev/null
   ```

2. **The install**, the password piped in (never on a command line), search engines kept out:

   ```sh
   op item get 'D3strukt0r Portfolio CMS | Prod | Admin' --account my.1password.com --fields password --reveal \
     | kubectl --context d3strukt0r-prod-admin -n d3strukt0r-portfolio exec -i deploy/web -c php-fpm -- php -r 'define("WP_INSTALLING", true); require "/var/www/html/wp-load.php"; require ABSPATH . "wp-admin/includes/upgrade.php"; if (is_blog_installed()) { exit("already installed\n"); } $r = wp_install("D3strukt0r", "admin", "d3strukt0r-portfolio-admin@d3strukt0r.dev", false, "", trim(stream_get_contents(STDIN))); echo "installed, user ", $r["user_id"], "\n";'
   ```

3. **A restart**, so the init containers activate the plugins and load the language packs
   (chart README "Plugins"; once the rollout race deactivates one, a second restart):
   `kubectl --context d3strukt0r-prod-admin -n d3strukt0r-portfolio rollout restart deployment web`.

4. **The site's plugins activated**: the chart activates only its own (the login, MCP, S3
   uploads); which of the site's are active is the database's, and a fresh one has none
   (`activate_plugin()` for each of `values.yaml`'s, through `php -r` as above; Query Monitor
   stays off).

5. **The owner's account**: log in through "OpenID Connect" once.

## Secrets

| OpenBao | 1Password | Holds |
|---|---|---|
| `secret/mariadb-apps/d3strukt0r-portfolio` | [`MariaDB \| Prod \| d3strukt0r-portfolio`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=ynbq6adr5vij4uow4oq25olnr4&h=my.1password.com) | the database password |
| `secret/d3strukt0r-portfolio` | [`D3strukt0r Portfolio CMS \| Prod \| App`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=32xvupsw7o63vz5esnvnouf7tq&h=my.1password.com) | the eight keys and salts |
| `secret/d3strukt0r-portfolio-s3` | [`Hetzner \| S3 \| prod d3strukt0r-portfolio`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=wyr7zalw5eg53ne2a7onksizr4&h=my.1password.com) | the key that writes the uploads |
| `secret/d3strukt0r-portfolio-uploads-proxy-s3` | [`Hetzner \| S3 \| prod d3strukt0r-portfolio uploads-proxy`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=46pr4rdzkhd5cwevum44y74lui&h=my.1password.com) | the uploads proxy's read-only key |
| `secret/d3strukt0r-portfolio-mailomat` | [`Mailomat \| Prod \| d3strukt0r-portfolio`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=mokr7pmg3u45vjqjrnbeyu5dou&h=my.1password.com), the token from [`Mailomat \| API \| mailomat.d3strukt0r.dev`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=3h3sz5lq3fi2n647in6k73iham&h=my.1password.com) | the SMTP login and the shared domain's API token mailomat-watch reads |
| `secret/d3strukt0r-portfolio-oidc` | [`Zitadel \| Prod \| d3strukt0r-portfolio`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=qu73xjkska72yjinngqeion22e&h=my.1password.com) | the client secret of the login through Zitadel |
