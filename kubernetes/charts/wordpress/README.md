# wordpress

A WordPress site from the stock images: its database in the shared MariaDB, its uploads in a
private bucket, nothing on a volume. Every WordPress site on the cluster is this chart with its
own values; what is particular to a site (its history, its plugins, its measurements) is in
`kubernetes/components/<site>/README.md` and `values.yaml`.

The chart is never packaged or published: Argo CD renders it straight from git on `master`, so
**a chart change reaches every site at once** with the push - render and diff all of them first
(see "Rolling out a chart change"). Not pinned per site by commit SHA: Argo CD refuses a `$values`
source that reads the same repository at another revision than the chart ("cannot reference a
different revision of the same repository", tried 2026-10-19), so a pinned chart would need the
values written into each Application. A staged rollout needs the chart in a registry instead.

## What runs

One pod (`templates/deployment.yaml`) and a CronJob (`templates/cronjob.yaml`), in a namespace
named after the site:

| Container | Image | Does |
|---|---|---|
| init `core` | `wordpress:7-php8.5-fpm` | writes the CA bundle, copies WordPress core and writes `wp-config.php` into `html`, a directory it creates itself in the emptyDir - the image's `tar` exits 2 when it cannot set the target's permissions, as in the root-owned volume; the other containers mount `html` as a `subPath` |
| init `plugins` | `wordpress:cli-2-php8.5` | installs the plugins and themes of the values and S3-Uploads; once the database holds the site, `wp core update-db` and the language packs |
| `php-fpm` | `wordpress:7-php8.5-fpm` | runs WordPress |
| `varnish` (with `pageCache`) | `varnish:9.1` | the page cache on 6081, in front of nginx (`files/default.vcl`, "Page cache") |
| `nginx` | `nginxinc/nginx-unprivileged:stable` | serves it on 8081 (`files/default.conf`), and php-fpm's pool figures on 9253 for Prometheus |
| `uploads-proxy` | `ghcr.io/nginxinc/nginx-s3-gateway/nginx-oss-s3-gateway:unprivileged-oss` | serves `/wp-content/uploads/` from the private bucket |
| CronJob `wp-cron` | `curlimages/curl` | calls `http://web/wp-cron.php` every five minutes |

Beside them: the Service and Ingress `web` (cert-manager creates the Certificate `web-tls` from
its annotation), the ExternalSecrets `web`
and `uploads-proxy`, the PodMonitor `php-fpm`, and in the namespace `mariadb` the site's
`Database`, `User`, `Grant` and the ExternalSecret `<site>-db` (`templates/database.yaml`).

## Names from the release

The Application's name is the release name and the namespace; these follow from it, so the
values never repeat them (the secrets do not - see "Secrets"):

| What | Name |
|---|---|
| Database and its user | the name with `_` for `-` (`robines_portfolio`) |
| Bucket | `d3strukt0r-prod-<site>` (`tofu/objectstorage`) |
| Valkey user | the site's name, in its database `objectCache.database` |
| PHP's configuration | `zz-<site>.ini`, `zz-<site>.conf` |

## Values

Only what differs between sites; `values.yaml` holds the defaults and says what each one does.

| Value | Default | What |
|---|---|---|
| `hosts` | (required) | the site's names; the first is WordPress's address (`WP_HOME`, the probes' and WP-Cron's `Host`), the others redirect to it |
| `plugins`, `themes` | `[]` | wordpress.org slugs, `<slug>` or `<slug> <version>` |
| `languages` | `[]` | language packs for core, plugins and themes |
| `secrets` | `[]` | every secret of the WordPress containers: blocks of `key` and `env` ("Secrets") |
| `uploadsProxy.secrets` | `[]` | the uploads proxy's key, the same way |
| `oidc.clientId` | empty | the site's app in Zitadel, which turns on the login through Zitadel; empty: off |
| `oidc.registerOrganisation` | empty | the Zitadel organisation people register into ("Registration in Zitadel"); empty: no registration |
| `mcp.clientId` | empty | the site's Claude app in Zitadel, which makes the site an MCP server for Claude ("Content through Claude"); needs `oidc`; empty: off |
| `pageCache.enabled`, `.size`, `.resources` | `false`, `64M`, 256Mi | Varnish in front of nginx ("Page cache"), its memory for pages and its container's |
| `pageCache.staticSizeMiB` | `512` | Varnish's storage for static files, a file on the pod's disk ("Page cache") |
| `pageCache.dropBasicAuth` | `false` | drop a basic-auth header before WordPress; only for a site behind Traefik's basic auth |
| `matomo.siteId` | `0` | the site's ID in the cluster's Matomo, which turns on its tracking ("Web analytics"); 0: off |
| `objectCache.database` | `0` | the site's database in valkey-cache, which turns on the object cache; 0: off |
| `reloader` | `true` | the Reloader annotation; needs the namespace in Reloader's list |
| `phpFpm.pool` | `{}` | `pm.*` settings, e.g. `max_children: 6`; empty: the image's five workers |
| `phpIni` | `{}` | PHP settings of the site, after `files/php.ini` so they win (e.g. `opcache.interned_strings_buffer: 64`) |
| `phpFpm.resources` | 256Mi / 768Mi | the php-fpm container's memory |
| `wpCli.memoryLimit` | empty | PHP's `memory_limit` for wp-cli in the init container (`WP_CLI_PHP_ARGS`) |
| `wpCli.resources` | limit 256Mi | the init container `plugins`' memory |
| `nginx.contentSecurityPolicy` | empty | the CSP header; empty: none |
| `nginx.xmlrpcAllow` | WordPress.com's ranges | who may reach `xmlrpc.php` (Jetpack); empty: nobody |
| `nginx.deniedUploadPaths` | `[]` | directories under uploads nginx refuses (regular expressions) |
| `ingress.middlewares` | `[]` | Traefik middlewares on the site's router |

## Secrets

A site's values list every secret it reads, so the values show where each environment variable
comes from - the chart derives none of them. One block per entry in the secret store, with the
1Password item it comes from in a comment above it:

```yaml
secrets:
  # 1Password "MariaDB | Prod | <site>"
  #   https://start.1password.com/open/i?...
  - key: mariadb-apps/<site>          # the entry (secret/mariadb-apps/<site> in OpenBao)
    env:
      WORDPRESS_DB_PASSWORD: password # environment variable: field
```

`key` and `property` are External Secrets' own words (`remoteRef`), the same for any store. A
variable from an entry of its own is a block of one; several fields of one entry share a block.
The blocks become the Secret `web`, which the containers `core`, `plugins` and `php-fpm` read as
environment variables; `uploadsProxy.secrets` becomes the Secret `uploads-proxy` the same way.
A new variable is one line in the site's values, no chart change.

The chart itself relies on these names:

| Variable | Read by |
|---|---|
| `WORDPRESS_DB_PASSWORD`, `WORDPRESS_AUTH_KEY` … `WORDPRESS_NONCE_SALT` | the WordPress image's `wp-config.php` |
| `WORDPRESS_DB_PASSWORD` | also the MariaDB user's password (`templates/database.yaml`) - required |
| `S3_KEY`, `S3_SECRET` | S3-Uploads (`getenv` in `WORDPRESS_CONFIG_EXTRA`) |
| `SMTP_USERNAME`, `SMTP_PASSWORD` | WP Mail SMTP (`getenv` in `WORDPRESS_CONFIG_EXTRA`) |
| `OIDC_CLIENT_SECRET` | the login through Zitadel - required with `oidc.clientId` |
| `VALKEY_PASSWORD` | the object cache, and the Valkey user's password (`templates/valkey-user.yaml`) - required with `objectCache.database` |
| `AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY` (`uploadsProxy.secrets`) | the uploads proxy |

The MariaDB and Valkey users take their password from whichever block names that variable, so
both sides always read the same value. A block without `key` or `env`, or a missing required
variable, stops the rendering with a message naming it.

## Design

- **No image of its own, no build, no volume.** The stock images, and the site assembled at every
  start: core from the WordPress image, plugins and themes from wordpress.org (the latest release
  of each unless the values pin a version), S3-Uploads from its GitHub release (pinned, 3.0.10).
  The release is the version wordpress.org's API names: the unversioned download is the
  development version (trunk), which ran Jetpack and Yoast pre-releases until 2026-10-22. A
  plugin wordpress.org has closed has no release (its API answers 404): the site still starts,
  with that plugin's development version and a `WARNING: … has no release` line in the init
  container `plugins`'s log - replace or remove it. Pinning does not help, wordpress.org removes
  a closed plugin's versioned downloads too. Any other failure of the API stops the start, so an
  outage there never puts every plugin on trunk. The two plugins the image brings, Akismet and Hello Dolly, are deleted first, so only
  the values decide; a site that wants Akismet lists it there.
  `DISALLOW_FILE_MODS` keeps WordPress from installing or updating anything itself, so the list
  in git is what runs; which plugins are active stays in the database. A plugin's uninstall
  routine stays reachable as "Daten löschen" (runbook "Plugins"). A start needs
  wordpress.org and GitHub; the running site does not. Keel rolls out new images of the floating
  tags (`components/keel`).
- **Its own address.** `WP_HOME` and `WP_SITEURL` are the first host, whatever the database says.
- **Uploads in a private bucket.** S3-Uploads, loaded as a must-use plugin
  (`files/s3-uploads-hetzner.php`, which also loads its autoloader first - without that its
  wp-cli command breaks every `wp` command), writes them with the site's key; their objects are
  private and their URLs point at the site itself, so `/wp-content/uploads/...` reaches nginx,
  which hands it to the uploads proxy. The proxy signs with a read-only key (the bucket policy
  denies it every write) and streams the file. Its cache key leaves out the query string, so a
  file rewritten under the same name stays old for as long as it caches, whatever `?ver=` the
  site sends, and Varnish's purge does not reach it (a consent banner's generated CSS showed an
  hour late at its default of an hour, 2026-10-26). So with the page cache it keeps nothing
  (`PROXY_CACHE_VALID_OK=0`): Varnish keeps the files by their full URL ("Page cache"), and a
  caching proxy would hand it the old file under the new URL. Without the page cache it keeps a
  file a minute, up to 256 MB. It addresses the bucket virtual-host style (`S3_STYLE=virtual`): in path style the gateway
  signs `Host: nbg1.your-objectstorage.com:443`, which Hetzner rejects as
  `SignatureDoesNotMatch` - shown to the browser as 404. Its log's "connect() to [2a01:…]:443
  failed (101: Network is unreachable)" is harmless: Hetzner's endpoint has an IPv6 address too,
  the pods have none, and nginx falls back to IPv4. Private files plugins keep under uploads
  (invoices, logs) go into `nginx.deniedUploadPaths`: their `.htaccess` works only under Apache.
  The init container also activates the plugin, so the plugin list shows it as in use: WordPress
  then loads it with `include_once` after the must-use plugin did, not a second time, and
  deactivating it in wp-admin changes nothing.
- **A request to the bucket gives up instead of hanging.** Object Storage now and then leaves a
  request unanswered; PHP then waited until nginx gave up after a minute (504 on a site's
  uncached pages, 2026-10-26). `files/s3-uploads-hetzner.php` gives S3-Uploads 5 s to connect,
  cuts a transfer only when nothing has moved for 10 s (a large upload takes as long as it needs)
  and retries once instead of three times: about 20 s at worst, logged as a PHP warning. A page
  then misses that one file check; an upload in wp-admin shows an error and can be repeated.
- **Plugins' generated files stay out of the bucket** where a plugin lets them: Complianz writes
  its banner's CSS under the uploads and checks it on every page it builds - a bucket request per
  page. `files/complianz.php` sends it to `wp-content/complianz/` in the pod (the init container
  creates it - Complianz would only for an admin); nginx serves it, Varnish keeps it by `?ver=`, and
  after a restart Complianz rebuilds it on the first page. Without Complianz it does nothing.
- **Complianz under a privacy signal**: with "respect DNT/GPC" on, a browser that sends Global
  Privacy Control (Brave always does) gets no banner and nothing optional - but Complianz also
  leaves it no way to decide otherwise: its "Manage consent" button stays hidden and opens nothing,
  and its consent switches show "consider excluding this site" instead. GPC is meant as a default
  the visitor can override on a site, so `files/complianz.php` shows the button to such browsers
  and sets the banner up on its first click; an explicit choice then counts (under a signal
  Complianz reads only the stored consent). It calls Complianz's global `cmplz_do_not_track()` and
  `show_cookie_banner()`, no public interface: after a Complianz update, check with Brave that the
  button still opens the banner. If not, nothing breaks - the button is just dead again.
- **TLS to MariaDB, verified.** The site's user is refused without TLS. WordPress has no CA
  setting (wpdb only passes `MYSQL_CLIENT_FLAGS` to `mysqli_real_connect`), so PHP checks the
  certificate against `openssl.cafile` (`files/php.ini`) - but only with
  `MYSQLI_CLIENT_SSL_VERIFY_SERVER_CERT` in the flags; `MYSQLI_CLIENT_SSL` alone encrypts without
  checking anything. The CA file is a bundle of the image's public CAs and the shared MariaDB's CA
  (ConfigMap `mariadb-ca` from trust-manager, through the namespace label), written by the init
  container `core`. A rotated MariaDB CA reaches the pod with its next start.
- **WP-Cron from a CronJob.** WordPress would run its scheduled tasks on visitors' page loads by
  calling its own public address; `DISABLE_WP_CRON` turns that off and the CronJob calls
  `wp-cron.php` through the Service every five minutes - on time, independent of visitors, past
  any basic auth on the Ingress, and a failing run shows as a failed Job. While the pod is missing
  (a node drained for a reboot, the new pod still assembling WordPress) curl retries every 30 s
  for up to 5 minutes instead of failing; a kured reboot failed a run that way (2026-10-18). A
  finished Job deletes itself a day later (`ttlSecondsAfterFinished`), so KubeJobFailed fires once
  and clears itself; the run's output stays in Loki: `{namespace="<site>", container="wp-cron"}`.
- **nginx** (`files/default.conf`) takes its rules in an order where they take effect (nginx uses
  the first matching regex location, so the denies come before the PHP rule): no PHP under
  `wp-content`/`wp-includes` except TinyMCE's loader, no `wp-config*`, readme or licence,
  no dotfiles, nothing executable under uploads; security headers on every answer, a CSP only
  where the values give one. `absolute_redirect off` keeps nginx's own redirects on the site's
  address instead of `http://…:8081`.
- **Static files a year when versioned**: CSS, JS, images and fonts whose URL carries `?ver=`
  (WordPress adds it and changes it with every update) are `expires 1y`, the rest and uploads
  7 days. `immutable` is left out: it needs an `add_header` in those locations, and one there
  drops the server's security headers (`nosniff` on scripts among them). Files from
  wordpress.org get a new `ver` with each update; a theme of our own must raise its `Version:`
  with every style change or, better, give `filemtime()` of the file as `ver` - one that gives no
  version gets WordPress' own, and returning visitors would keep its old CSS and JS until the next
  WordPress update. Purging Cloudflare or Varnish does not help: the copy is in the browser.
- **XML-RPC only for WordPress.com** (`nginx.xmlrpcAllow`, Jetpack's published addresses,
  https://jetpack.com/ips-v4.txt): Jetpack's connection reaches the site through `xmlrpc.php`,
  which for everyone else is a door for guessing passwords. Its connection test failed with 403
  while it was blocked for all (Site Health, 2026-10-22). A site behind basic auth needs the path
  exempted there too. The list may change: compare it with Jetpack's
  when its connection test (Site Health) fails again. The address nginx checks is Cloudflare's
  `CF-Connecting-IP`, with the caveat of the next bullet.
- **The opcache holds every site's code** (`files/php.ini`): 256 MB (its interned strings
  included, measured), 32 MB of strings, 32,531 files, and no timestamp checks - the files never
  change while the pod runs. The image's 128 MB, 8 MB and 4,000 files were full on every site
  (2026-10-22): arepazo's 20,000 PHP files kept filling the cache, which was thrown away and
  compiled again (hit rate 88 %). The pool sizing counts the 256 MB in.
- **WordPress sees the visitor's address**: nginx takes it from Cloudflare's `CF-Connecting-IP`
  (`real_ip_header`) and hands it to PHP as `REMOTE_ADDR` and as `X-Real-IP`, replacing
  Traefik's, which is Cloudflare's own - Jetpack reads that one and locked arepazo's login page
  for a Cloudflare node every visitor there shared (2026-10-21); WooCommerce records it with an
  order. nginx's access log shows it too. Port 443 is open to everyone, so a client that skips
  Cloudflare can set the header itself and choose the address plugins see; restricting 443 to
  Cloudflare's ranges would close that.
- **32k for the headers of a PHP response** (`fastcgi_buffer_size`; default 4k): logging out
  through Zitadel redirects with the whole ID token in the URL and deletes a cookie per plugin,
  which on arepazo ended in a 502 (`upstream sent too big header`).
- **Patient probes.** Startup and readiness go through WordPress (`/robots.txt`), but wait 10 s
  and run every 30 s (readiness): under load PHP sometimes needs more than the 1 s default, and
  three slow answers in a row take the pod out of its Service - then wp-cron and visitors get no
  answer (2026-10-05).
- **Liveness asks nginx alone** (`/healthz`, answered by nginx with 204, at Kubernetes' default
  timing). Restarting nginx cannot help a slow PHP: through WordPress, a liveness probe restarted
  nginx 19 times while PHP was slow (2026-10-11) and turned a slow shop into a dead one.
- **A slow log.** A request running longer than 2 s writes its PHP backtrace into php-fpm's log
  (`request_slowlog_timeout`, `files/php-fpm.conf`), so a slow spell shows what WordPress waits
  on: `{namespace="<site>", container="php-fpm"} |= "slow"` in Grafana's Explore.
- **php-fpm's pool figures in Prometheus** (job `<site>/php-fpm`, `templates/pod-monitor.yaml`):
  busy and idle workers, queued requests, slow requests, how often all workers were taken.
  php-fpm writes them itself in Prometheus' format (`?openmetrics`, PHP 8.1+), so no exporter
  runs; nginx serves them on port 9253 (`/metrics`), which neither the Service nor the Ingress
  knows. They come from a status pool of their own (`pm.status_listen`), which answers while all
  workers are busy (tested: 5 busy, 2 queued, answer in 2 ms); a hung php-fpm gives a 504 within
  2 s. nginx fixes the query, so the per-request detail with request URLs (`?full`) never leaves
  the pod. Not hipages/php-fpm_exporter: last release 2022, and it hangs when php-fpm does not
  answer (its issue #253). Alerts and the dashboard "php-fpm" are kube-prometheus-stack's.
- **A drain before stopping.** Every container of the pod waits 10 s (`preStop` `sleep`) before
  it stops: all of them get the stop signal at once, while Traefik still routes to the pod for a
  moment (measured: 8 s after the signal). php-fpm then lets running requests finish for up to
  20 s (`process_control_timeout`; the default 0 drops them), within a 40 s grace period. Without
  both, a rollout answered a few requests with 502.
- **Restricted Pod Security**: the pod runs as 33 (`www-data`), the two nginx images as their own
  user 101, the curl job as 100.
- **Mail through mailomat**: WP Mail SMTP's `WPMS_*` constants in `WORDPRESS_CONFIG_EXTRA` send
  through `smtp.mailomat.cloud:587` (STARTTLS) with the site's SMTP user from OpenBao, so wp-admin
  shows the connection greyed out; the sender stays wp-admin's. Signing and DMARC are in
  [`../../../tofu/cloudflare/README.md`](../../../tofu/cloudflare/README.md) ("Sending through
  mailomat").
- **An object cache in valkey-cache** (`objectCache.database`, `components/valkey-cache`):
  WordPress keeps its query results in Valkey instead of asking MariaDB again on the next
  request - for admin and visitors alike. Redis Object Cache's drop-in, through Predis (the image
  has no phpredis), logged in as a user named after the site and restricted to its own database.
  The chart brings everything: the plugin (added to `plugins.txt`), the drop-in (copied by the
  init container `plugins` - the plugin's admin button is hidden under `DISALLOW_FILE_MODS`, and
  the drop-in works without the plugin activated), the `WP_REDIS_*` constants, the password in
  `web`, the namespace label valkey-cache's NetworkPolicy asks for, and the site's Valkey user as
  the ExternalSecret `db-<n>` in `valkey-cache` (`templates/valkey-user.yaml`). Graceful
  (`WP_REDIS_GRACEFUL`): while Valkey cannot be reached the site runs from its database, slower,
  instead of showing an error page; what changes in that time misses the cache
  ([`../../components/valkey-cache/README.md`](../../components/valkey-cache/README.md), "After a
  cache outage"). The init container activates the plugin at every start (its status page under
  Settings → Redis); deactivating it in wp-admin deletes the drop-in, and the next start puts
  both back.
- **The chart creates its namespace** (`templates/namespace.yaml`), against Helm's convention of
  `--create-namespace`: the sites' Applications carry the finalizer, and Argo CD never deletes a
  namespace it created through `CreateNamespace=true` - only one among the Application's own
  objects goes with it. The namespace also carries trust-manager's label for the MariaDB CA. The
  chart is only ever rendered by Argo CD, one namespace per site, so Helm's reason (a release
  failing on, or taking over, an existing namespace) does not apply.
- **The ConfigMap keeps its name** (`web-config`); the pod template carries its checksum, so a
  changed file still rolls the pod.

## Login through Zitadel

With `oidc.clientId` set, wp-login.php offers a button to log in through Zitadel beside the
normal form. The site's project and app are in `tofu/zitadel` (`project_<site>.tf`; README
"WordPress sites"): Zitadel lets in only people with a role for the site.

- **The plugin** OpenID Connect Generic Client (`daggerhart-openid-connect-generic`, added to
  `plugins.txt` and activated by the init container at every start), configured entirely by `OIDC_*` constants in `WORDPRESS_CONFIG_EXTRA` - its
  settings page shows them locked. The ID token's signature is checked against Zitadel's keys
  (`OIDC_ENDPOINT_JWKS_URL`; without it the plugin would not check). Logging out of WordPress
  ends the Zitadel session too, and Zitadel sends people back to
  `wp-login.php?loggedout=true` - the only post-logout address its app allows, so `files/oidc.php`
  drops the `wp_lang` WordPress would add.
- **The roles come from Zitadel alone**: `files/oidc.php`, a must-use plugin, gives every
  WordPress role named in the user's Zitadel roles `<site>:<role>`
  (`robines-portfolio:administrator` -> administrator, `arepazo:shop_manager` -> shop_manager),
  several at once if Zitadel grants several - at the first login, when it creates the user, and
  at every later one, so a role changed in wp-admin is undone by the next login. It keeps no list:
  any role WordPress or a plugin defines works, a key WordPress does not know is skipped. Without
  such a role, or with an address Zitadel has not verified, it refuses the login. WordPress's and
  WooCommerce's own registration are off for the same reason - they would hand out a role
  Zitadel never gave.
- **So does the profile**: the email (only a verified one), first and last name and the public
  name (Zitadel's free display name - WordPress's dropdown only suggests combinations and lists
  the current value too), at every login like the roles - the plugin would set them only when it
  creates the account, so a linked one would keep its old values. WordPress's "your email was
  changed" mail is not sent for it. An address that belongs to another account is skipped and
  logged in php-fpm's log (`zitadel: profile of ... not updated`); the login goes on.
  WooCommerce's billing and shipping data stays what the customer entered. **For a linked account
  those fields are read only** (the public name's dropdown disabled) - in wp-admin's profile and
  in WooCommerce's account details, with a link to the account page at Zitadel
  (`/ui/console/users/me`) - and a change sent anyway is dropped on the server
  (`wp_pre_insert_user_data`, `insert_user_meta`; no confirmation mail to a new address): it would
  last only until the next login. Only the sync writes them.
- **People are recognised by Zitadel's `sub`**, the user ID, never by their email
  (`OIDC_LINK_EXISTING_USERS` off): an organisation's admin can give a user any address and mark
  it verified, and email linking would hand that user someone else's account. A new person gets
  a new WordPress user; if their address is already taken by an account, WordPress refuses to
  create it - that account is linked once by hand (below).
- **PKCE**, which the plugin lacks, also in `files/oidc.php`: a verifier stored with the login's
  state, its hash as the challenge, the verifier with the code. The app is confidential too
  (client secret, `OIDC_CLIENT_SECRET`).
- **The login's state lives in a WordPress transient** - in Valkey when the object cache is on.
  While Valkey cannot be reached, a login through Zitadel fails with "Invalid state"; the
  normal form keeps working.
- **A break-glass administrator** per site: a local account whose address belongs to nobody in
  Zitadel, its password only in 1Password, for when Zitadel is down. It logs in with the form.
- **A linked account logs in through Zitadel only** (`files/oidc.php`): the password form refuses
  it with a hint to the button, it gets no password reset (not even through a link sent
  before; WordPress's and WooCommerce's reset forms answer that it lives behind the login
  button, under its login page's "Reset Password"), and its profile offers no new password - "Lost
  your password?" would otherwise let whoever reads its mailbox set one and get past Zitadel and
  its second factor. Its password is a random one nobody knows (the plugin sets
  one for new accounts; an account linked by hand gets one too, "Linking an existing account").
  Unlinked accounts - the break-glass administrator - keep the form and the reset. Jetpack's
  "Log in with WordPress.com" (its SSO module) is a way in of its own that none of this covers;
  whether a site offers it is its admin's choice.
- **Who is linked shows in wp-admin**, read only: the profile page has a row "OpenID Connect user
  ID", the user list a column "OpenID Connect" (also `files/oidc.php`) - an account without one
  logs in with the form only. The plugin itself keeps the ID out of sight.
- **wp-login.php puts the login through Zitadel first** (`files/oidc.php`): the buttons move into a
  box of their own, and the password form - the break-glass administrator's - waits behind "Log in
  with username and password" below them. It shows at once after a failed password login, without
  JavaScript, in wp-admin's re-login dialog and on every other wp-login.php page (lost password,
  reset). WooCommerce's login form keeps its layout.
- **WooCommerce's login form** (My account, checkout) shows the button too and leads back to the
  page it was on.
- **No text people see names Zitadel** (user decision, 2026-10-21) - buttons, messages, the
  wp-admin row and column and the Must-Use plugin's name say "OpenID Connect". **They are
  translated** into German and Spanish: `files/zitadel-login-de.l10n.php` and `-es.l10n.php`,
  WordPress's PHP translation format (since 6.5, read like a `.mo`, plain text in git), one per
  language whatever the country (`de` for `de_CH`/`de_DE`, `es` for arepazo's Polylang `es_VE`),
  loaded from `mu-plugins/zitadel-login-languages/` - not `mu-plugins/` itself, where every
  `.php` runs as a plugin. They are loaded at `init` and again at every `change_locale`: Polylang
  holds translations back until it knows a page's language, then empties them and announces the
  locale - loading only at `init` left shop pages in English. The plugin's own button text has
  no translation upstream, so the mu-plugin translates it. A new text gets its entry in every
  file; a new language is one file and its ConfigMap key (`templates/configmap.yaml`).

### Registration in Zitadel

A shop whose customers create their own accounts sets `oidc.registerOrganisation` to the ID of
the Zitadel organisation they register into (which allows registration; `tofu/zitadel` README
"WordPress sites"). The login page and WooCommerce's login form then get a second button,
"Create an account", that opens Zitadel's registration page for that organisation
(`prompt=create` and the scope `urn:zitadel:iam:org:id:<id>`). Only this button sends the scope:
Zitadel lets only members of the named organisation log in under it, so the login button names
none and staff of another organisation still get in. The new person's role is given by Zitadel
at registration, not by WordPress; their first login creates the account.

### Switching a site to the login through Zitadel

1. **Zitadel**: the site's project, roles and app, and its secret in 1Password and OpenBao
   (`tofu/zitadel`, README "Adding a WordPress site").
2. **The existing WordPress users**, with their roles and addresses:

   ```shell
   kubectl --context d3strukt0r-prod-admin -n <site> exec deploy/web -c php-fpm -- php -r 'require "/var/www/html/wp-load.php"; foreach (get_users() as $u) printf("%s <%s> %s, %d posts\n", $u->user_login, $u->user_email, implode(",", $u->roles), count_user_posts($u->ID));'
   ```
3. **A decision for each**:
   - **The break-glass administrator** stays local, logging in with the form. Its address must
     belong to no Zitadel user, and its password is in 1Password.
   - **Someone who keeps working on the site** gets a Zitadel user (in the right organisation)
     and a grant with their role, and their account is linked (below) - otherwise their first
     login through Zitadel finds no account with their ID, and WordPress refuses to create a
     second one with their address.
   - **An account nobody uses any more** is deleted in wp-admin, its content handed to someone.
4. **The site's values**: `oidc.clientId` and the `secrets` block for `OIDC_CLIENT_SECRET`; push.
5. **The links** (below), once the new pod runs.
6. **Each linked person logs in through the button once**, and the break-glass administrator
   with the form.

### Linking an existing account

The plugin recognises people only by their Zitadel user ID (`sub`), which it keeps on the
WordPress account. An account from before Zitadel has none; linking writes it there once, so
that person's logins through Zitadel land in their existing account - with its content,
settings and history, and the role from Zitadel. The ID is in the console (Users → the user →
User ID).

```shell
SITE=<site> LOGIN=<wordpress login> ZITADEL_ID=<zitadel user id>
kubectl --context d3strukt0r-prod-admin -n "$SITE" exec deploy/web -c php-fpm -- php -r 'require "/var/www/html/wp-load.php"; $u = get_user_by("login", $argv[1]); if (!$u) exit(1); var_dump(update_user_option($u->ID, "openid-connect-generic-subject-identity", $argv[2], true));' "$LOGIN" "$ZITADEL_ID"
```

A number (the new row, first link) or `bool(true)` (changed) means linked; `bool(false)` means
the value was already there. Then replace its old password with a random one nobody knows (the
plugin does the same for new accounts) - it can no longer be used anyway, and so it is not left
in a password manager either:

```shell
kubectl --context d3strukt0r-prod-admin -n "$SITE" exec deploy/web -c php-fpm -- php -r 'require "/var/www/html/wp-load.php"; $u = get_user_by("login", $argv[1]); wp_set_password(wp_generate_password(32, true, true), $u->ID); echo "ok\n";' "$LOGIN"
```

Someone without an account from before needs nothing: their first login creates one.

## Content through Claude

With `mcp.clientId`, people edit the site by chatting with Claude - claude.ai, Claude Desktop or
Claude Code - each as their own WordPress user, with that user's rights: a customer can write
nothing, an editor only what an editor may. Who may is decided in Zitadel alone.

- **Two plugins, pinned**: MCP Adapter (`mcp-adapter`, WordPress's own, experimental) serves the
  abilities as an MCP server; Enable Abilities for MCP (`enable-abilities-for-mcp`) brings the
  abilities for posts, pages, media, portfolio items, Polylang translations, Yoast fields and
  WooCommerce products. Both are activated at every start. Plugins of this kind have had
  access-control flaws (AI Engine's MCP let subscribers become admins), so a new version is read
  in its changelog before the pin moves.
- **Claude logs in with Zitadel** (`files/mcp.php`): the site's project has a second app,
  `<site>-claude` (tofu/zitadel; public, PKCE), so the role check applies to Claude as to the
  login. The MCP route answers a request without a token with 401 and the address of its
  metadata (`/wp-json/oidc-mcp/v1/resource`, RFC 9728), which names Zitadel; Claude logs the
  person in there and sends its token with every request. The site asks Zitadel about each token
  (introspection, with the site's own app's credentials; cached a minute), accepts only the Claude
  app's, and acts as the WordPress account linked to that Zitadel user - someone who never logged
  in to the site has none and gets nothing. Deactivating someone or ending their session in
  Zitadel ends their Claude access within a minute; a removed role at the latest when the access
  token expires (an hour), since Zitadel then refuses to renew it. Tested 2026-10-22: a user
  deactivated in Zitadel was refused on the first call after it - the site answered 401 and
  Zitadel refused to renew the token (`Errors.User.NotActive`); reactivated, a new login through
  `/mcp` worked again. Enable Abilities' own OAuth
  server and its site-wide Bearer token are off - each would be a key Zitadel never sees.
- **Application Passwords stay on** (user decision, 2026-10-21): customers' and staff's apps -
  the WooCommerce app among them - log in to self-hosted sites with them. They are WordPress's
  own keys, which Zitadel never sees, and they open the REST API and the MCP route with that
  person's rights too. So **when someone leaves, delete their Application Passwords** as well
  (wp-admin → Users → the user → Application Passwords): deactivating them in Zitadel does not
  end those. On 2026-10-21 no site had any.
- **Connecting** - the URL is `https://<host>/wp-json/mcp/mcp-adapter-default-server` and the
  OAuth client ID the site's Claude app's (`tofu output -raw <site>_claude_client_id`, no
  secret): claude.ai/Claude Desktop: Settings → Connectors → Add custom connector, the client ID
  under "Use your own OAuth client" (a Team plan's members cannot add one - its owner does, or a
  Pro/Max account). Claude Code:
  `claude mcp add --transport http --scope user --client-id <id> --callback-port 33418 <site>
  <url>` (`--scope user`: in every project; the default `local` sets it up for the current
  directory only), then `/mcp` to log in (the app allows exactly that callback). Each site's
  README has its finished command and connector values. Claude Code is tested on all three sites
  (2026-10-22); claude.ai and Claude Desktop are not yet - no account at hand that may add a
  custom connector - so a first connection there may still meet a surprise.
- **What Claude may do** is fixed in `files/mcp.php`, not in the plugin's settings page
  (`pre_option_*`, so a click in wp-admin changes nothing): read and write content - posts,
  pages, media, categories and tags, portfolio items, translations, Yoast fields, products -
  never delete it, and nothing about people: no users, comments, orders or customers (personal
  data would flow to Claude), no search and replace across the database, no code. The plugin
  itself would allow nearly all of that. **The list decides for every plugin**: WooCommerce,
  Yoast, WPForms, WP Mail SMTP and core mark abilities of their own public for MCP (orders, mail
  logs, site-wide SEO settings), so whatever is not listed is made private there, last after
  every plugin's filter - a plugin added later included (2026-10-22: 21 such abilities on
  arepazo before this). WooCommerce's own product abilities are listed (list, create, edit -
  creating exists only there); deleting products and its order abilities are not.
- **Prompt injection**: Claude reads what others wrote (posts, imported text) and acts with the
  person's rights. In Claude, leave writing tools on "ask before use", never "always allow".

## Page cache

With `pageCache.enabled`, Varnish (`varnish:9.1`, Varnish Software's build of Vinyl Cache) sits in
the pod in front of nginx: the Service sends every request to it on 6081, and it hands what is
not for the cache to nginx on `127.0.0.1:8081`. It keeps what an anonymous visitor sees; PHP then
serves only what is personal. Rules in `files/default.vcl`:

- **Passed through, never kept**: anything but GET/HEAD; wp-admin, wp-login, wp-cron, xmlrpc,
  comments, the REST API (Claude's MCP included), `wc-api`, add-to-cart, `wc-ajax`, previews,
  searches; cart, checkout and my account (also German, `warenkorb`/`kasse`/`mein-konto`);
  anyone logged in or with a cart, a WooCommerce session, a comment-author or protected-post
  cookie; a Bearer token.
- **Static files kept too** (since 2026-10-26): CSS, JS, fonts, images and the uploads, for
  everyone alike - logged in or not, cookies dropped - by their full URL, so a new `?ver=` is a
  new file at once. A week with a query string, an hour without (an upload replaced under the same
  name), 404s a minute; never a 403 (`nginx.deniedUploadPaths`) or a file over 10 MB (videos and
  downloads flow through as before). They used to be left to Cloudflare; kept here, the origin
  does not depend on it - leaving Cloudflare needs no change in the chart.
- **One copy per page**: other cookies (analytics, Polylang's language - the language is in the
  URL) are dropped; tracking parameters (`utm_*`, `gclid`, `fbclid` ...) are removed and the
  query sorted; a browser's `Accept` becomes `text/html`, since WordPress answers every page with
  `Vary: accept, content-type` and would otherwise get a copy per browser. On a site behind
  Traefik's basic auth (`pageCache.dropBasicAuth`) the basic-auth header is removed too - Traefik
  checked it already - so its visitors share the pages. Everywhere else it reaches WordPress: there
  it is an Application Password, and a request carrying it is never cached. Removed on every site
  2026-10-22 for a few hours, which broke Application Passwords.
- **Never cached**: an answer with a cookie, `private`/`no-store`/`no-cache`, or a status other
  than 200, 301 and 404. On a page fetched for the cache, Polylang's `pll_language` and
  WooCommerce's empty guest session (`wp_woocommerce_session_*`, set on the shop page) are
  dropped first - without that no page of arepazo's could be cached; a real session starts with
  the first cart action, which is never cached.
- **Six hours, then up to twelve more** while a fresh copy is fetched in the background (grace),
  and a failed refresh keeps the old copy; sitemaps and feeds an hour, 404s five minutes. Twelve
  plus six stays well under WPForms' 48-hour form token.
- **Emptied by WordPress**: Proxy Cache Purge (`varnish-http-purge`, pinned) sends a PURGE to
  `127.0.0.1:6081` (`VHP_VARNISH_IP`) whenever a post, page or product is saved - in wp-admin or
  by Claude - for the page, the home page and its archives; `files/page-cache.php` adds
  WooCommerce's stock changes (an order taking the last one), which save nothing: the product's
  page (a variation's parent), the shop page and its categories with their parent categories.
  Saving a consent banner's settings (Complianz, WPConsent) empties the whole site: the banner is
  in every page but its settings are no post; Complianz's only once it has rewritten its CSS, so
  the pages pick up the new `?ver=`.
  PURGE comes only from inside the pod; from outside it answers 405. The admin bar's "Purge Cache
  (All Pages)" empties static files too - the way to show a file replaced in place before its hour.
  Its ads for the paid add-on Cacheability Pro in wp-admin notices count as dismissed for every
  user (also `files/page-cache.php`); its own settings page still shows them.
- **Memory**: `pageCache.size` for pages (`-s malloc`), 64M for passed answers while they flow
  through (`-s Transient`; with `transit_buffer=1M` Varnish reads only 1M ahead of the visitor, so a
  file of any size passes - without it a 200 MB download broke off at 33 MB, 2026-10-22), plus
  Varnish itself - the container's limit leaves room for all three
  (arepazo: 128M in 384Mi; a page is a few dozen kB, so that holds thousands). Kept small on
  purpose: with all three sites the nodes would carry a failed one at about 94% of their memory
  at twice these sizes (2026-10-22). Static files have a storage of their own, so images never
  push pages out: a file of `pageCache.staticSizeMiB` (512) on the working directory's emptyDir
  (`-s static=file`, sized to it plus 200Mi), held in the kernel's file cache, which gives way
  under memory pressure, not in the container's own memory. The working directory is that
  emptyDir; the root filesystem is read-only.
- **Gatus checks the login page**, not the home page: grace would serve the home page for hours
  while PHP is down.
- **Not yet**: Prometheus metrics - the usual exporter needs an image of its own (roadmap).
  `varnishstat` in the container shows the figures meanwhile.
- Tested 2026-10-22 against a stand-in backend: hits across browsers, cookies dropped on the shop
  page, passes for login/cart/REST/Bearer, basic auth shared, PURGE 405 from outside, 200 from
  inside, regex purge. nginx's `/healthz` answers 204, which the backend probe must expect - it
  counts only 200 otherwise and marks nginx sick.

Seeing it work: the answer carries `X-Cache: HIT` or `MISS` and `Age` -
`curl -s -D - -o /dev/null https://<host>/ | grep -i -E 'x-cache|age'` twice. Emptying it by hand,
the whole site:
`kubectl --context d3strukt0r-prod-admin -n <site> exec deploy/web -c varnish -- varnishadm "ban req.url ~ ."`;
its figures:
`kubectl --context d3strukt0r-prod-admin -n <site> exec deploy/web -c varnish -- varnishstat -1 -f MAIN.cache_hit -f MAIN.cache_miss -f MAIN.n_object`.

## Web analytics

A site with `matomo.siteId` is counted in the cluster's Matomo (components/matomo), by the
mu-plugin `files/matomo.php`, written for this chart rather than a plugin from wordpress.org:
Connect Matomo has no WooCommerce sales, Matomo Site Kit had 30 installs, Matomo's own WooCommerce
connector is paid, and none of them reads the WP Consent API (2026-10-23).

- **From the site's own address.** The tracker loads as `/m/a.js` and reports to `/m/e`; nginx
  (`files/default.conf`) passes both to Matomo inside the cluster (`matomo.matomo.svc`, as
  `matomo.d3strukt0r.dev`) with the visitor's address in `CF-Connecting-IP`, and without the
  site's cookies. Ad blockers block `/matomo.js` and `/matomo.php` on any host (EasyPrivacy), not
  these. Resolved per request (`resolver` = kube-dns, `10.43.0.10`), so a site starts and runs
  while Matomo is away. The page cache passes `/m/` (`files/default.vcl`).
- **Cookieless unless the visitor agrees.** Everyone is counted without cookies
  (`requireCookieConsent`); whoever allows "statistics" in the site's consent banner gets
  Matomo's cookies and is recognised on later visits. The banner speaks through the WP Consent API
  (plugin `wp-consent-api` beside e.g. WPConsent): its consent is read in the browser, since
  pages come from the page cache. Without a banner nobody gets cookies. WPConsent's script blocking
  (on for arepazo and robines-portfolio since 2026-10-24, holding back e.g. X's embeds until
  "Marketing") never holds back the tracker: its list knows Matomo only by `matomo.php`, which
  `/m/` avoids, and `files/matomo.php` exempts the service `matomo` in case the list grows.
- **Matomo's page overlay** (Verhalten → Seiten, the overlay icon; free, unlike the heatmap
  plugin) shows the site in a frame with the share of clicks on each link. For it the site allows
  Matomo as a frame ancestor (`frame-ancestors 'self' https://matomo.d3strukt0r.dev`, beside
  `X-Frame-Options: SAMEORIGIN` for older browsers), and the tracker knows Matomo's address
  (`setAPIUrl`) to load the overlay's script from there - visitors' hits still go to `/m/e`. The
  overlay also puts a hidden frame of Matomo into the page, so a site with its own CSP lists
  `https://matomo.d3strukt0r.dev` in `frame-src` (robines-portfolio). Matomo's nginx sets no
  Referrer-Policy of its own: the overlay starts from the full address of Matomo's
  `startOverlaySession` page, which a stricter policy cuts off ("Laden…" for ever). An ad blocker in
  the viewer's browser (Brave's Shields) blocks the overlay's script from Matomo - allow it for
  `matomo.d3strukt0r.dev`.
- **Shop pages.** Product and category views (`setEcommerceView`), and the whole cart after every
  change, sent with the next page (carts change in AJAX requests).
- **Orders from the server**, when an order becomes "processing" or "completed": straight to
  Matomo, so an ad blocker or a closed tab cannot lose a sale. The visitor's address and browser
  are noted when the order is placed, Matomo's visitor ID only if they agreed to cookies; an order
  entered in wp-admin has neither and is still counted (`bots=1` - Matomo would otherwise take
  WordPress's user agent for a bot and drop it), with the shop as its visitor. Sent
  once per order (`_matomo_tracked`); a failure is logged and retried at the next status. Needs a
  token in `MATOMO_TOKEN` (`secrets`): a local Matomo user with **write** on this site only - with
  it Matomo takes the visitor's address from the request instead of the shop's
  (components/matomo, README "Adding a site").

## Vulnerability check

Every site checks WordPress, its plugins and its themes once a day for known vulnerabilities
and for plugins wordpress.org has closed; Prometheus alerts like on anything else (rules in
[`../../components/kube-prometheus-stack/wordpress.yaml`](../../components/kube-prometheus-stack/wordpress.yaml)).

- **How**: the must-use plugin `files/vulnerability-check.php` schedules a daily event in
  WordPress' own cron (run by the CronJob `wp-cron`), asks for every installed item and stores the
  result in the option `vulnerability_check`. `files/vulnerability-metrics.php` - outside the
  directory WordPress loads plugins from, started with `SHORTINIT` so no plugin or theme runs -
  serves it on nginx's metrics port as `/wordpress` (cluster-internal, like `/metrics`), scraped
  every minute through the PodMonitor (a longer interval leaves gaps past Prometheus' five-minute
  lookback, and the alerts' `for` would keep restarting).
- **Sources**: known vulnerabilities from [WPVulnerability.net](https://www.wpvulnerability.net/)
  (free, no account, CC0 data; it gathers CVE/NVD, Patchstack, WPScan and Wordfence), one
  request per item; closures from api.wordpress.org (`error: closed` with date and reason). The
  requests identify as `vulnerability-check`, not with WordPress' default user agent, which names
  the site's address - beside the installed versions that would be a target list. An
  entry applies when the installed version lies within its bounds, compared with PHP's
  `version_compare()`; "fixed in" is known only where the entry says "below". Many entries have
  no CVSS score (no CVE behind them) - they report 0. Not used: Wordfence's feed (v2 is gone, v3
  needs an account and a key Wordfence may revoke), WPScan (25 requests a day, non-commercial
  only), Patchstack (paid), OSV (no WordPress).
- **Metrics**: `wordpress_vulnerability{kind, slug, version, id, title, fixed_in, kev, link}`
  (value: CVSS score), `wordpress_closed{kind, slug, version, closed_date, reason}`,
  `wordpress_vulnerability_check_timestamp_seconds` (0 before the first check) and
  `wordpress_vulnerability_check_errors`. A source that does not answer keeps that item's last
  findings - an outage never clears an alert - and counts as an error.
- **Alerts**: `WordPressVulnerability` (warning), `WordPressVulnerabilityCritical` (CVSS 9 and
  above, or exploited in the wild per CISA's KEV list; to the phone), `WordPressPluginClosed`, and
  `WordPressVulnerabilityCheckStale` (no result for two days, or a source unreachable for two).
  Tested 2026-10-22 against WPVulnerability's answers: the Timber 1.23.1 that ran until that day
  would have raised CVE-2024-45411 (CVSS 8.6, fixed in 1.23.3).

**On an alert**: update the item (a newer release arrives at the next start; a pinned one needs
its pin raised), replace it or remove it from the site's values; the alert clears after the next
check. A false positive or an accepted risk gets an Alertmanager silence on its `id`. To check at
once instead of waiting for the next day, and to read the result:

```shell
kubectl --context d3strukt0r-prod-admin -n <site> exec deploy/web -c php-fpm -- php -r '$_SERVER["HTTP_HOST"] = "localhost"; require "/var/www/html/wp-load.php"; vulnerability_check_run();'
kubectl --context d3strukt0r-prod-admin -n <site> exec deploy/web -c nginx -- curl -s localhost:9253/wordpress
```

## Sizing the pool

`pm.max_children = (limit - opcache (256 MB) and master - headroom) / memory per worker`, with one
worker fewer when a single heavy request (up to `memory_limit` 256M, `files/php.ini`) must fit
beside the others. The memory per worker is its own (`RssAnon` in `/proc/<pid>/status`, or
`phpfpm_memory_peak` in Prometheus), not the RSS `ps` shows, which counts the opcache - shared
once - in every worker. A worker restarts after 500 requests (`pm.max_requests`) against plugins'
memory leaks. Raise the container's limit (`phpFpm.resources`) before the count. Each site's
values show its calculation; a site that raises `opcache.memory_consumption` in `phpIni` counts
its own value instead of the 256 MB. To measure:

```shell
kubectl --context d3strukt0r-prod-admin -n <site> exec deploy/web -c php-fpm -- sh -c 'grep RssAnon /proc/[0-9]*/status'
```

## Runbooks

### Rolling out a chart change

1. Change the chart and bump `version` in `Chart.yaml`.
2. Render it for every site and compare with what runs - a template error shows here, not on
   the cluster:
   ```shell
   helm lint kubernetes/charts/wordpress -f kubernetes/components/<site>/values.yaml
   helm template <site> kubernetes/charts/wordpress -n <site> -f kubernetes/components/<site>/values.yaml > /tmp/<site>.yaml
   ```
   and a server-side dry run against the cluster, per namespace (the MariaDB objects are in
   `mariadb`): `kubectl --context d3strukt0r-prod-admin diff --server-side -n <site> -f <file>`.
   The site in the other repository is rendered with its values there.
3. Commit and push: every site rolls out together. Check each one (`Checking it`).

A chart that fails to render changes nothing: Argo CD shows the error on the Application and the
running pods stay. A renamed or removed value changes in the same commit as every site's values.

### Adding a site

1. Bucket and its two keys in `tofu/objectstorage`, the DNS records in `tofu/cloudflare`.
2. Its secrets in OpenBao, each from a 1Password item.
3. `kubernetes/components/<site>/values.yaml` (hosts, plugins, the secrets with their 1Password
   items - "Secrets") and a `README.md` with what is particular to it.
4. `kubernetes/clusters/prod/<site>.yaml`: an Application named after the site, copied from
   robines-portfolio's, with the finalizer. Its namespace into Reloader's list
   (`components/reloader`) unless `reloader: false`.

### Plugins

- **Adding or removing a plugin or theme**: a line in the site's `plugins`/`themes`, push; the
  pod restarts with it. Activate a new one in wp-admin afterwards. Before removing a plugin,
  deactivate it and click **"Daten löschen"** in the plugin list (`files/plugin-data.php`): it
  runs the plugin's own uninstall routine - what WordPress's "Löschen" would, hidden under
  `DISALLOW_FILE_MODS` - and then reads "Daten gelöscht am …"; the files go with the line in the
  values. Only plugins with such a routine get the link; one without (Google Analytics for
  WooCommerce) leaves its options behind, and some delete little unless a setting of their own
  says so (WooCommerce: `WC_REMOVE_ALL_DATA`) or keep their settings for a reinstall
  (MonsterInsights: 19 options, its cache table and 3 cron hooks stayed - check the database
  afterwards). A failure shows as a notice and in php-fpm's log.
- **A plugin release breaks the site** (a 500 with its path in the php-fpm log): pin the last good
  version (`<slug> <version>`). Every start installs the latest release otherwise.
- **A new WordPress major or PHP version** comes with the image tags (`7-php8.5-fpm` twice and
  `cli-2-php8.5` in `templates/deployment.yaml`) - a chart change, for every site at once. The
  init container `plugins` runs the database update. Dump the sites' databases first (from the
  replica; `kubectl --context d3strukt0r-prod-admin -n mariadb get mariadb mariadb -o
  jsonpath='{.status.currentPrimary}'` names the primary), then watch every site's php-fpm log
  for fatal errors - a plugin that breaks gets pinned or the tags go back:

  ```shell
  for db in $(kubectl --context d3strukt0r-prod-admin -n mariadb get databases.k8s.mariadb.com -o jsonpath='{.items[*].spec.name}'); do
    kubectl --context d3strukt0r-prod-admin -n mariadb exec mariadb-0 -c mariadb -- sh -c "mariadb-dump -uroot -p\"\$MARIADB_ROOT_PASSWORD\" --single-transaction --routines --triggers --events --hex-blob $db" | gzip > "$db.sql.gz"
  done
  ```

  PHP 8.5 since 2026-10-21 (WordPress calls 8.4 and 8.5 "beta support" until 10% of sites run
  them; WooCommerce states 8.4).
- **Flushing the object cache** (stale data after a change made outside WordPress, e.g. in
  phpMyAdmin) - FLUSHDB, on the site's own database only:
  `kubectl --context d3strukt0r-prod-admin -n <site> exec deploy/web -c php-fpm -- php -r 'require "/var/www/html/wp-load.php"; var_dump(wp_cache_flush());'`
- **Switching the object cache on**: its password (`components/valkey-cache`, "Adding an app"),
  a free number in its database table, `objectCache.database` in the site's values. valkey-cache
  restarts by itself when the site's user appears (Reloader).
- **The pod hangs in an init container**: its log says which download failed
  (`kubectl --context d3strukt0r-prod-admin -n <site> logs deploy/web -c plugins`).

## Checking it

```shell
helm template <site> kubernetes/charts/wordpress -n <site> -f kubernetes/components/<site>/values.yaml
kubectl --context d3strukt0r-prod-admin -n <site> get pods,cronjobs,jobs,externalsecrets,certificates,ingress
kubectl --context d3strukt0r-prod-admin -n mariadb get databases.k8s.mariadb.com,users.k8s.mariadb.com,grants.k8s.mariadb.com <site>
kubectl --context d3strukt0r-prod-admin -n <site> logs deploy/web -c plugins
```
