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
| init `core` | `wordpress:7-php8.4-fpm` | writes the CA bundle, copies WordPress core and writes `wp-config.php` into `html`, a directory it creates itself in the emptyDir - the image's `tar` exits 2 when it cannot set the target's permissions, as in the root-owned volume; the other containers mount `html` as a `subPath` |
| init `plugins` | `wordpress:cli-2-php8.4` | installs the plugins and themes of the values and S3-Uploads; once the database holds the site, `wp core update-db` and the language packs |
| `php-fpm` | `wordpress:7-php8.4-fpm` | runs WordPress |
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
| `objectCache.database` | `0` | the site's database in valkey-cache, which turns on the object cache; 0: off |
| `reloader` | `true` | the Reloader annotation; needs the namespace in Reloader's list |
| `phpFpm.pool` | `{}` | `pm.*` settings, e.g. `max_children: 6`; empty: the image's five workers |
| `phpFpm.resources` | 128Mi / 512Mi | the php-fpm container's memory |
| `wpCli.memoryLimit` | empty | PHP's `memory_limit` for wp-cli in the init container (`WP_CLI_PHP_ARGS`) |
| `wpCli.resources` | limit 256Mi | the init container `plugins`' memory |
| `nginx.contentSecurityPolicy` | empty | the CSP header; empty: none |
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
  `DISALLOW_FILE_MODS` keeps WordPress from installing or updating anything itself, so the list
  in git is what runs; which plugins are active stays in the database. A start needs
  wordpress.org and GitHub; the running site does not. Keel rolls out new images of the floating
  tags (`components/keel`).
- **Its own address.** `WP_HOME` and `WP_SITEURL` are the first host, whatever the database says.
- **Uploads in a private bucket.** S3-Uploads, loaded as a must-use plugin
  (`files/s3-uploads-hetzner.php`, which also loads its autoloader first - without that its
  wp-cli command breaks every `wp` command), writes them with the site's key; their objects are
  private and their URLs point at the site itself, so `/wp-content/uploads/...` reaches nginx,
  which hands it to the uploads proxy. The proxy signs with a read-only key (the bucket policy
  denies it every write) and streams the file; it caches up to 256 MB, a deleted file for up to an
  hour. It addresses the bucket virtual-host style (`S3_STYLE=virtual`): in path style the gateway
  signs `Host: nbg1.your-objectstorage.com:443`, which Hetzner rejects as
  `SignatureDoesNotMatch` - shown to the browser as 404. Its log's "connect() to [2a01:…]:443
  failed (101: Network is unreachable)" is harmless: Hetzner's endpoint has an IPv6 address too,
  the pods have none, and nginx falls back to IPv4. Private files plugins keep under uploads
  (invoices, logs) go into `nginx.deniedUploadPaths`: their `.htaccess` works only under Apache.
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
  `wp-content`/`wp-includes` except TinyMCE's loader, no xmlrpc, `wp-config*`, readme or licence,
  no dotfiles, nothing executable under uploads; security headers on every answer, a CSP only
  where the values give one. `absolute_redirect off` keeps nginx's own redirects on the site's
  address instead of `http://…:8081`.
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
  ends the Zitadel session too.
- **The role comes from Zitadel**: `files/oidc.php`, a must-use plugin, gives the WordPress role
  named in the user's Zitadel role `<site>:<role>` (`robines-portfolio:administrator` ->
  administrator; with several, administrator > shop_manager > editor > author > contributor >
  subscriber) - at the first login, when it creates the user, and at every later one. Without
  such a role, or with an address Zitadel has not verified, it refuses the login.
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

`bool(true)` means linked. Someone without an account from before needs nothing: their first
login creates one.

## Sizing the pool

`pm.max_children = (limit - opcache and master - headroom) / memory per worker`, with one worker
fewer when a single heavy request (up to `memory_limit` 256M, `files/php.ini`) must fit beside
the others. The memory per worker is its own (`RssAnon` in `/proc/<pid>/status`, or
`phpfpm_memory_peak` in Prometheus), not the RSS `ps` shows, which counts the opcache - shared
once - in every worker. A worker restarts after 500 requests (`pm.max_requests`) against plugins'
memory leaks. Raise the container's limit (`phpFpm.resources`) before the count. Each site's
values show its calculation. To measure:

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
  pod restarts with it. Activate a new one in wp-admin afterwards. A removed one's settings stay
  in the database.
- **A plugin release breaks the site** (a 500 with its path in the php-fpm log): pin the last good
  version (`<slug> <version>`). Every start installs the latest release otherwise.
- **A new WordPress major version** comes with the image tag (`7-php8.4-fpm`, two places in
  `templates/deployment.yaml`) - a chart change, for every site at once. The init container
  `plugins` runs the database update.
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
