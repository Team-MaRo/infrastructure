# arepazo

The WooCommerce shop at `https://www.arepazo.ch` (`arepazo.ch` redirects there;
`kubernetes/clusters/prod/arepazo.yaml`, plain manifests here, namespace `arepazo`). Moved from
prod-old, where it ran under Docker Compose from images of its own (`d3strukt0r/arepazo`, 2024),
with every later WordPress and plugin update living only in the container. Its shop data -
orders, customers, invoices - is personal data.

## What runs

One pod (`deployment.yaml`) and a CronJob (`cronjob.yaml`):

| Container | Image | Does |
|---|---|---|
| init `core` | `wordpress:7-php8.4-fpm` | writes the CA bundle, copies WordPress core and writes `wp-config.php` into `html`, a directory it creates itself in the emptyDir - the image's `tar` exits 2 when it cannot set the target's permissions, as in the root-owned volume; the other containers mount `html` as a `subPath` |
| init `plugins` | `wordpress:cli-2-php8.4` | installs `config/plugins.txt` and S3-Uploads; once the database holds the site, `wp core update-db` and the language packs |
| `php-fpm` | `wordpress:7-php8.4-fpm` | runs WordPress |
| `nginx` | `nginxinc/nginx-unprivileged:stable` | serves it on 8081 (`config/default.conf`) |
| `uploads-proxy` | `ghcr.io/nginxinc/nginx-s3-gateway/nginx-oss-s3-gateway:unprivileged-oss` | serves `/wp-content/uploads/` from the private bucket |
| CronJob `wp-cron` | `curlimages/curl` | calls `http://web/wp-cron.php` every five minutes (WooCommerce's Action Scheduler runs on it) |

The database is `arepazo` in the shared MariaDB (`database.yaml`), the uploads are in the bucket
`d3strukt0r-prod-arepazo` (`tofu/objectstorage`).

## Design

The same design as [`../robines-portfolio/README.md`](../robines-portfolio/README.md) - stock
images assembled at every start, plugins from `config/plugins.txt` (latest release each start,
`DISALLOW_FILE_MODS`), uploads in a private bucket behind a signing proxy (`S3_STYLE=virtual`),
TLS to MariaDB verified (`MYSQLI_CLIENT_SSL_VERIFY_SERVER_CERT` plus the CA bundle), WP-Cron from
a CronJob, `absolute_redirect off`, a 10 s drain before stopping and php-fpm finishing running
requests (`config/php-fpm.conf`), restricted Pod Security. What differs:

- **Private files never reach the proxy.** WooCommerce and its plugins keep files under uploads
  that must not be public: the invoice PDFs (`wpo_wcpdf_*`), protected downloads
  (`woocommerce_uploads`), logs (`wc-logs`), Facebook's product catalog exports
  (`facebook_for_woocommerce`), Contact Form 7's temporary uploads (`wpcf7_uploads`), WPForms'
  cache (`wpforms/cache`) and a removed file manager's leftovers. Their `.htaccess` files work
  only under Apache - prod-old's nginx ignored them; here nginx refuses them (403) before the
  proxy sees the request. WordPress reads them itself through S3-Uploads. A plugin that adds
  such a directory needs it added to the list in `config/default.conf`.
- **More memory.** prod-old's php-fpm was OOM-killed at 280M: PHP gets `memory_limit 256M`, the
  php-fpm container a 1Gi limit (384Mi requested; it idles at 250-300Mi). Under real traffic all
  five workers (`pm.max_children`) run at once - 512Mi was OOM-killed twice right after the
  switch (2026-10-04). wp-cli in the init container loads WooCommerce's admin code and
  crashes at 128M, so it runs with `-d memory_limit=1024M` (`WP_CLI_PHP_ARGS`) in a 1280Mi
  container that exits once the site is assembled.
- **Slow requests, patient probes.** Every page takes about 3 s, on prod-old as here (WooCommerce,
  Jetpack, Yoast and some thirty more plugins, no page cache). The probes still go through
  WordPress (`/robots.txt`), but wait 10 s and run every 30 s (readiness) and 60 s (liveness);
  robines-portfolio's 1 s timeout left nginx not ready. A page cache would be the real fix
  (`litespeed-cache` is installed but works only on a LiteSpeed server).
- **Three languages.** wp-admin runs in `de_CH`, the shop in German, English and Spanish
  (Polylang); the init container installs `de_CH`, `de_DE` and `es_ES` for core, plugins and
  themes.
- **No Content-Security-Policy**: checkout embeds the payment providers' frames (Braintree,
  PayPal).
- **`timber-library` pinned to 1.23.1**: 1.23.4 lacks a file and takes the site down (a fatal
  error on every page).
- **No e-mail.** WP Mail SMTP's SMTP login has been refused since 2024-06 (every logged attempt
  fails), so order confirmations and customer mails do not go out - left as is at the move
  (2026-10-04). Sending needs a working SMTP account for `info@arepazo.ch` (the domain's mail is
  Microsoft 365), its password through OpenBao.
- **Not migrated**: the `arepazo.d3strukt0r.dev` name (a redirect) and prod-old's public
  phpMyAdmin - the shared one is behind Zitadel.

## Secrets

| OpenBao | Fields | From |
|---|---|---|
| `secret/arepazo` | `auth-key` … `nonce-salt` (prod-old's, so logins survived the move) | 1Password [`Arepazo CMS | Prod | App`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=m5jimwowtt7z3zzkbldyzwpxsi&h=my.1password.com) |
| `secret/mariadb-apps/arepazo` | `password` (20 characters) | 1Password [`MariaDB | Prod | arepazo`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=373ezgwhdppj4dsxghr3aqcxpa&h=my.1password.com) |
| `secret/arepazo-s3` | `access-key`, `secret-key` (read and write) | 1Password [`Hetzner | S3 | prod arepazo`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=t6onf7inzynbh6oqvdggcrdzoy&h=my.1password.com) |
| `secret/arepazo-uploads-proxy-s3` | `access-key`, `secret-key` (read only) | 1Password [`Hetzner | S3 | prod arepazo uploads-proxy`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=gs5mn5w3sfbqsljokq3ukhjtjy&h=my.1password.com) |

A changed value reaches the pod through External Secrets (within an hour, or at once with
`kubectl --context d3strukt0r-prod-admin -n arepazo annotate externalsecret --all force-sync=$(date +%s) --overwrite`),
and Reloader then restarts it (`components/reloader`).

## Runbooks

- **Adding or removing a plugin or theme**: a line in `config/plugins.txt`, push; the pod
  restarts with it. Activate a new one in wp-admin afterwards.
- **A plugin release breaks the site** (a 500 with its path in the php-fpm log): pin the last good
  version as a third column in `config/plugins.txt` (`plugin <slug> <version>`).
- **A new WordPress major version** comes with the image tag (`7-php8.4-fpm`, both places in
  `deployment.yaml`); the init container runs the database update.
- **The pod hangs in an init container**: its log says which download failed
  (`kubectl --context d3strukt0r-prod-admin -n arepazo logs deploy/web -c plugins`).

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin -n arepazo get pods,cronjobs,jobs,externalsecrets,certificates,ingress
kubectl --context d3strukt0r-prod-admin -n mariadb get databases.k8s.mariadb.com,users.k8s.mariadb.com,grants.k8s.mariadb.com arepazo
kubectl --context d3strukt0r-prod-admin -n arepazo logs deploy/web -c plugins
```
