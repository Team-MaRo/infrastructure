# arepazo

The WooCommerce shop at `https://www.arepazo.ch` (`arepazo.ch` redirects there;
`kubernetes/clusters/prod/arepazo.yaml`, namespace `arepazo`). Moved from prod-old, where it ran
under Docker Compose from images of its own (`d3strukt0r/arepazo`, 2024), with every later
WordPress and plugin update living only in the container. Its shop data - orders, customers,
invoices - is personal data.

It is the WordPress chart ([`../../charts/wordpress/README.md`](../../charts/wordpress/README.md):
what runs, the design, the runbooks) with the values in `values.yaml`. The database is `arepazo`
in the shared MariaDB, the uploads are in the bucket `d3strukt0r-prod-arepazo`
(`tofu/objectstorage`); WP-Cron runs WooCommerce's Action Scheduler.

## What is particular to it

- **Private files never reach the proxy.** WooCommerce and its plugins keep files under uploads
  that must not be public: the invoice PDFs (`wpo_wcpdf_*`), protected downloads
  (`woocommerce_uploads`), logs (`wc-logs`), Facebook's product catalog exports
  (`facebook_for_woocommerce`), Contact Form 7's temporary uploads (`wpcf7_uploads`), WPForms'
  cache (`wpforms/cache`) and a removed file manager's leftovers. Their `.htaccess` files work
  only under Apache - prod-old's nginx ignored them; here nginx refuses them (403) before the
  proxy sees the request. WordPress reads them itself through S3-Uploads. A plugin that adds
  such a directory needs it added to `nginx.deniedUploadPaths` in `values.yaml`.
- **More memory.** prod-old's php-fpm was OOM-killed at 280M: PHP gets `memory_limit 256M`, the
  php-fpm container a 1Gi limit (384Mi requested; it idles at 250-300Mi). Under real traffic all
  workers (`pm.max_children`) run at once - 512Mi was OOM-killed twice with five right after the
  switch (2026-10-04). wp-cli in the init container loads WooCommerce's admin code and
  crashes at 128M, so it runs with `-d memory_limit=1024M` (`WP_CLI_PHP_ARGS`) in a 1280Mi
  container that exits once the site is assembled. It requests only 256Mi: without a request
  the limit would count as one, and a pod keeps its largest init container's request booked on
  the node for its whole life.
- **Six workers, sized from the memory limit** (`phpFpm` in `values.yaml`): `(limit - opcache -
  master - headroom) / memory per worker` = (1024 - 128 - 40 - ~200) / 90 MB = 7, one fewer so a
  heavy request (an export, up to `memory_limit` 256M) still fits beside five normal ones: ~160
  fixed + 5 x 90 + 256 = ~870 MiB; two at once would not. The memory per
  worker is its own, not the RSS `ps` shows: every worker's RSS includes the opcache (128M, shared
  once), which made them look 225 MB each. The image's five were taken by a single `/wp-admin/`
  load (2026-10-18): Jetpack, Yoast and admin-ajax fire ~17 requests at once, each 1.5-9 s and
  mostly waiting on wordpress.com and the database - five busy workers used 1.65 of the node's 4
  cores. `pm` stays `dynamic` (2-4 idle), so an idle count of 0 still means a full pool; a worker
  restarts after 500 requests (`pm.max_requests`) against plugins' memory leaks. To recompute,
  read the workers' own memory and the pool's peak:
  `kubectl --context d3strukt0r-prod-admin -n arepazo exec deploy/web -c php-fpm -- sh -c 'grep RssAnon /proc/[0-9]*/status'`
  and `phpfpm_memory_peak{namespace="arepazo"}` in Prometheus; raise the limit before the count.
  robines-portfolio keeps the image's five - it has never filled them.
- **Slow requests, patient probes.** Every page takes about 3 s, on prod-old as here (WooCommerce,
  Jetpack, Yoast and some thirty more plugins, no page cache). Startup and readiness still go
  through WordPress (`/robots.txt`), but wait 10 s and run every 30 s (readiness); the 1 s
  default left nginx not ready. A page cache would be the real fix (`litespeed-cache` is
  installed but works only on a LiteSpeed server).
- **Liveness asks nginx alone** (`/healthz`, answered by nginx with 204, at Kubernetes' default
  timing). On 2026-10-11 from 03:30 to 06:00 every request took 6-9 s while the pod, the node
  and MariaDB were idle; the liveness probe, then through WordPress, restarted nginx 19 times
  and turned a slow shop into a dead one (503), until PHP recovered by itself. The cause is
  unknown, so php-fpm now keeps a **slow log**: a request running longer than 2 s writes its PHP
  backtrace into php-fpm's log (`request_slowlog_timeout`, the chart's `files/php-fpm.conf`) -
  `{namespace="arepazo", container="php-fpm"} |= "slow"` in Grafana's Explore.
- **Object cache in valkey-cache, database 2** (since 2026-10-20; the chart's "An object cache in
  valkey-cache"): before it, loading WordPress alone took 834 database queries, and every page
  1.5-2 s.
- **Login through Zitadel** (since 2026-10-21; the chart's "Login through Zitadel" and
  "Registration in Zitadel"): the project `arepazo` in `tofu/zitadel`, granted to the
  organisation `Arepazo`, where the shop's people are - Eduardo, who owns the shop and that
  organisation and gives his staff their roles there (`arepazo:administrator`,
  `arepazo:shop_manager`), and the customers. **Customers register in Zitadel** ("Create an
  account" on the login page and on My account; WooCommerce's own registration is off) and get
  `arepazo:customer` from the registration webhook (`components/zitadel` README
  "Registration"); their first login creates their WordPress account. The user's own account
  `D3strukt0r` is linked to his Zitadel user by hand. **Break-glass: the local `admin`**
  (`info@arepazo.ch`, an address no Zitadel user has), its password in 1Password
  [`Arepazo CMS | Prod | Admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=5aankgyeyucb7h6wrqk4qegxoa&h=my.1password.com).
- **Three languages.** wp-admin runs in `de_CH`, the shop in German, English and Spanish
  (Polylang); the init container installs `de_CH`, `de_DE` and `es_ES` for core, plugins and
  themes.
- **No Content-Security-Policy**: checkout embeds the payment providers' frames (Braintree,
  PayPal).
- **`timber-library` pinned to 1.23.1**: 1.23.4 lacks a file and takes the site down (a fatal
  error on every page).
- **Mail through mailomat** (since 2026-10-18; from 2024-06 until then nothing went out - the old
  SMTP account's login was refused). The website's mail - order confirmations, password resets,
  contact forms, whatever it sends - leaves through mailomat's SMTP user
  `web@mailomat.arepazo.ch` (`smtp.mailomat.cloud:587`, STARTTLS), set by WP Mail SMTP's
  `WPMS_*` constants in `WORDPRESS_CONFIG_EXTRA`, so wp-admin shows the connection greyed out;
  the sender stays wp-admin's (`Arepazo <info@arepazo.ch>`). The domain's own mail stays at
  Microsoft 365; mailomat signs as `mailomat.arepazo.ch`, which DMARC's relaxed alignment counts
  for `arepazo.ch` ([`../../../tofu/cloudflare/README.md`](../../../tofu/cloudflare/README.md),
  "Sending through mailomat"). No IP restriction on the SMTP user (user decision): the password
  is its only guard - rotate it in mailomat, then in 1Password and OpenBao, if it may have
  leaked.
  - **The FREE plan: 200 mails per 30-day period**, shared by everything on the cluster that
    sends through mailomat. An order sends about three (to the customer, to the shop, on
    completion). The counter is in mailomat's dashboard; nothing alerts on it.
  - **The invoice PDF goes along as an attachment**: mailomat allows attachments on FREE (to
    test every feature, support ticket #1972, 2026-10) but not on LITE - only from PRO. If they
    are ever refused, switch the invoice plugin (WooCommerce → PDF Invoices) from attaching to a
    download link. Which emails carry it is the plugin's setting in wp-admin ("Attach to").
- **Not migrated**: the `arepazo.d3strukt0r.dev` name (a redirect) and prod-old's public
  phpMyAdmin - the shared one is behind Zitadel.

## Secrets

| OpenBao | Fields | From |
|---|---|---|
| `secret/arepazo` | `auth-key` … `nonce-salt` (prod-old's, so logins survived the move) | 1Password [`Arepazo CMS | Prod | App`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=m5jimwowtt7z3zzkbldyzwpxsi&h=my.1password.com) |
| `secret/mariadb-apps/arepazo` | `password` (20 characters) | 1Password [`MariaDB | Prod | arepazo`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=373ezgwhdppj4dsxghr3aqcxpa&h=my.1password.com) |
| `secret/arepazo-s3` | `access-key`, `secret-key` (read and write) | 1Password [`Hetzner | S3 | prod arepazo`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=t6onf7inzynbh6oqvdggcrdzoy&h=my.1password.com) |
| `secret/arepazo-uploads-proxy-s3` | `access-key`, `secret-key` (read only) | 1Password [`Hetzner | S3 | prod arepazo uploads-proxy`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=gs5mn5w3sfbqsljokq3ukhjtjy&h=my.1password.com) |
| `secret/valkey-apps/arepazo` | `password` (20 characters; its user in valkey-cache) | 1Password [`Valkey | Prod | arepazo`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=liu2kovgpq7s6kuyhkpfrytnhm&h=my.1password.com) |
| `secret/arepazo-mailomat` | `username`, `password` (the SMTP user at mailomat) | 1Password [`Mailomat | Prod | arepazo`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=6ietzpx5jygsojtd5fiflvdlee&h=my.1password.com) (an E-Mail item, SMTP section) |
| `secret/arepazo-oidc` | `client-secret` (the Zitadel app's) | 1Password [`Zitadel | Prod | arepazo`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=f2tzxffbdlwbr2ya5v7t45oid4&h=my.1password.com) |

A changed value reaches the pod through External Secrets (within five minutes, or at once with
`kubectl --context d3strukt0r-prod-admin -n arepazo annotate externalsecret --all force-sync=$(date +%s) --overwrite`),
and Reloader then restarts it (`components/reloader`).

## Checking it

```shell
helm template arepazo kubernetes/charts/wordpress -n arepazo -f kubernetes/components/arepazo/values.yaml
kubectl --context d3strukt0r-prod-admin -n arepazo get pods,cronjobs,jobs,externalsecrets,certificates,ingress
kubectl --context d3strukt0r-prod-admin -n mariadb get databases.k8s.mariadb.com,users.k8s.mariadb.com,grants.k8s.mariadb.com arepazo
kubectl --context d3strukt0r-prod-admin -n arepazo logs deploy/web -c plugins
```
