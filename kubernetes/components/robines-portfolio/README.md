# robines-portfolio

The old WordPress portfolio, public at `https://old.robines.space`
(`kubernetes/clusters/prod/robines-portfolio.yaml`, plain manifests here, namespace
`robines-portfolio`). `robines.space` and `www.robines.space` are a Cloudflare Worker - the live
site - and have nothing to do with this. Moved from prod-old, where it ran under Docker Compose
with the whole web root on a bind mount and plugins installed through wp-admin.

## What runs

One pod (`deployment.yaml`) and a CronJob (`cronjob.yaml`):

| Container | Image | Does |
|---|---|---|
| init `core` | `wordpress:7-php8.4-fpm` | writes the CA bundle, copies WordPress core and writes `wp-config.php` into `html`, a directory it creates itself in the emptyDir - the image's `tar` exits 2 when it cannot set the target's permissions, as in the root-owned volume; the other containers mount `html` as a `subPath` |
| init `plugins` | `wordpress:cli-2-php8.4` | installs `config/plugins.txt` and S3-Uploads; once the database holds the site, `wp core update-db` and the `de_CH` language packs |
| `php-fpm` | `wordpress:7-php8.4-fpm` | runs WordPress |
| `nginx` | `nginxinc/nginx-unprivileged:stable` | serves it on 8081 (`config/default.conf`) |
| `uploads-proxy` | `ghcr.io/nginxinc/nginx-s3-gateway/nginx-oss-s3-gateway:unprivileged-oss` | serves `/wp-content/uploads/` from the private bucket |
| CronJob `wp-cron` | `curlimages/curl` | calls `http://web/wp-cron.php` every five minutes |

The database is `robines_portfolio` in the shared MariaDB (`database.yaml`), the uploads are in
the bucket `d3strukt0r-prod-robines-portfolio` (`tofu/objectstorage`).

## Design

- **No image of its own, no build, no volume.** The stock images, and the site assembled at every
  start: core from the WordPress image, plugins and themes from wordpress.org (the latest release
  of each, `config/plugins.txt` - everything prod-old had, active or not, except the premium
  theme `mintraro-pro`, which is inactive and not on wordpress.org), S3-Uploads from its GitHub
  release (pinned, 3.0.10). `DISALLOW_FILE_MODS` keeps WordPress from installing or updating
  anything itself, so the list in git is what runs; which plugins are active stays in the
  database. A start needs wordpress.org and GitHub; the running site does not. Keel rolls out new
  images of the floating tags (`components/keel`).
- **Its own address, `old.robines.space`.** `WP_HOME` and `WP_SITEURL` pin it, whatever the
  database says; the URLs in the content were rewritten from `robines.space` at the move.
- **Uploads in a private bucket.** S3-Uploads, loaded as a must-use plugin
  (`config/s3-uploads-hetzner.php`, which also loads its autoloader first - without that its
  wp-cli command breaks every `wp` command), writes them with the app's key; their objects are
  private and their URLs point at the site itself, so `/wp-content/uploads/...` reaches nginx,
  which hands it to the uploads proxy. The proxy signs with a read-only key (the bucket policy
  denies it every write) and streams the file; it caches up to 256 MB, a deleted file for up to an
  hour. It addresses the bucket virtual-host style (`S3_STYLE=virtual`): in path style the gateway
  signs `Host: nbg1.your-objectstorage.com:443`, which Hetzner rejects as
  `SignatureDoesNotMatch` - shown to the browser as 404. Its log's "connect() to [2a01:…]:443
  failed (101: Network is unreachable)" is harmless: Hetzner's endpoint has an IPv6 address too,
  the pods have none, and nginx falls back to IPv4.
- **TLS to MariaDB, verified.** The user `robines_portfolio` is refused without TLS. WordPress has
  no CA setting (wpdb only passes `MYSQL_CLIENT_FLAGS` to `mysqli_real_connect`), so PHP checks
  the certificate against `openssl.cafile` (`config/php.ini`) - but only with
  `MYSQLI_CLIENT_SSL_VERIFY_SERVER_CERT` in the flags; `MYSQLI_CLIENT_SSL` alone encrypts without
  checking anything. The CA file is a bundle of the image's public CAs and the shared MariaDB's CA
  (ConfigMap `mariadb-ca` from trust-manager, through the namespace label), written by the init
  container `core`. A rotated MariaDB CA reaches the pod with its next start.
- **WP-Cron from a CronJob.** WordPress would run its scheduled tasks on visitors' page loads by
  calling its own public address; `DISABLE_WP_CRON` turns that off and the CronJob calls
  `wp-cron.php` through the Service every five minutes - on time, independent of visitors, and a
  failing run shows as a failed Job.
- **Language packs at start.** The site runs in `de_CH`; WordPress cannot fetch language packs
  itself, so the init container installs them for core, plugins and themes. A failure there only
  logs a warning (English text until the next start).
- **nginx** carries prod-old's rules, in an order where they take effect (nginx uses the first
  matching regex location, and on prod-old the PHP rule came first, so the denies below it never
  applied): no PHP under `wp-content`/`wp-includes` except TinyMCE's loader, no xmlrpc,
  `wp-config*`, readme or licence, no dotfiles, nothing executable under uploads; prod-old's
  security headers and permissive CSP (embeds from YouTube and Twitter). `absolute_redirect off`
  keeps nginx's own redirects on the site's address instead of `http://…:8081`.
- **A drain before stopping.** Every container of the pod waits 10 s (`preStop` `sleep`) before
  it stops: all of them get the stop signal at once, while Traefik still routes to the pod for a
  moment (measured: 8 s after the signal). php-fpm then lets running requests finish for up to
  20 s (`process_control_timeout`, `config/php-fpm.conf`; the default 0 drops them), within a
  40 s grace period. Without both, a rollout answered a few requests with 502.
- **Restricted Pod Security**: the pod runs as 33 (`www-data`), the two nginx images as their own
  user 101, the curl job as 100.
- **Watched by Gatus** ("Robines Portfolio (old)" on the status page, `components/gatus`): the home
  page every five minutes.
- **Not for search engines.** The live site is the Worker at robines.space; this archive tells
  search engines `noindex, nofollow` (wp-admin → Settings → Reading → "Discourage search engines",
  set 2026-10-04 - in the database, not in git). Before the move its pages named robines.space as
  their address, so the rewrite to old.robines.space made them a site of their own.
- **No e-mail.** WP Mail SMTP is on PHP's `mail()`, which needs a local `sendmail` the image does
  not have ("Could not instantiate mail function"); on prod-old sending failed every month too.
  Password resets and form notifications (WPForms, Gutena Forms) do not go out - form entries are
  still stored. Sending needs an SMTP account in WP Mail SMTP, its password through OpenBao.
- **Not migrated**: prod-old's Duplicator package (`wp-content/backups-dup-lite`, 222 MB, from
  2025-10-14) and the `robines-portfolio.d3strukt0r.dev` name.

## Secrets

| OpenBao | Fields | From |
|---|---|---|
| `secret/robines-portfolio` | `auth-key`, `secure-auth-key`, `logged-in-key`, `nonce-key`, `auth-salt`, `secure-auth-salt`, `logged-in-salt`, `nonce-salt` (prod-old's) | 1Password [`Robines Portfolio CMS | Prod | App`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=fkyq7sgplhkvasmf3aiwh2acge&h=my.1password.com) |
| `secret/mariadb-apps/robines-portfolio` | `password` (20 characters) | 1Password [`MariaDB | Prod | robines-portfolio`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=swneckmu7nc3tkxv5c2njudv2q&h=my.1password.com) |
| `secret/robines-portfolio-s3` | `access-key`, `secret-key` (read and write) | 1Password [`Hetzner | S3 | prod robines-portfolio`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=j3phtimnb5zbwonzf655meqrb4&h=my.1password.com) |
| `secret/robines-portfolio-uploads-proxy-s3` | `access-key`, `secret-key` (read only) | 1Password [`Hetzner | S3 | prod robines-portfolio uploads-proxy`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=3icb7ypn2ub7bk7x2yzbwhkrfm&h=my.1password.com) |

A changed value in OpenBao reaches the pod through External Secrets (within an hour, or at once
with `kubectl --context d3strukt0r-prod-admin -n robines-portfolio annotate externalsecret --all force-sync=$(date +%s) --overwrite`),
and Reloader then restarts it (`components/reloader`).

## Runbooks

- **Adding or removing a plugin or theme**: a line in `config/plugins.txt`, push; the pod
  restarts with it. Activate a new one in wp-admin afterwards. A removed one's settings stay in
  the database.
- **A plugin release breaks the site** (a 500 with its path in the php-fpm log): pin the last good
  version as a third column in `config/plugins.txt` (`plugin <slug> <version>`). Every start
  installs the latest release otherwise.
- **A new WordPress major version** comes with the image tag (`7-php8.4-fpm`); moving to the next
  major is changing the tag in `deployment.yaml` (both places). The init container `plugins` runs
  the database update.
- **The pod hangs in an init container**: its log says which download failed
  (`kubectl --context d3strukt0r-prod-admin -n robines-portfolio logs deploy/web -c plugins`).

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin -n robines-portfolio get pods,cronjobs,jobs,externalsecrets,certificates,ingress
kubectl --context d3strukt0r-prod-admin -n mariadb get databases.k8s.mariadb.com,users.k8s.mariadb.com,grants.k8s.mariadb.com robines-portfolio
kubectl --context d3strukt0r-prod-admin -n robines-portfolio logs deploy/web -c plugins
```
