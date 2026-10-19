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

Beside them: the Service and Ingress `web`, the Certificate `web-tls`, the ExternalSecrets `web`
and `uploads-proxy`, the PodMonitor `php-fpm`, and in the namespace `mariadb` the site's
`Database`, `User`, `Grant` and the ExternalSecret `<site>-db` (`templates/database.yaml`).

## Names from the release

The Application's name is the release name and the namespace; everything else follows from it,
so the values never repeat it:

| What | Name |
|---|---|
| Database and its user | the name with `_` for `-` (`robines_portfolio`) |
| Bucket | `d3strukt0r-prod-<site>` (`tofu/objectstorage`) |
| OpenBao | `secret/<site>` (keys and salts), `secret/mariadb-apps/<site>`, `secret/<site>-s3`, `secret/<site>-uploads-proxy-s3`, `secret/<site>-mailomat` |
| PHP's configuration | `zz-<site>.ini`, `zz-<site>.conf` |

## Values

Only what differs between sites; `values.yaml` holds the defaults and says what each one does.

| Value | Default | What |
|---|---|---|
| `hosts` | (required) | the site's names; the first is WordPress's address (`WP_HOME`, the probes' and WP-Cron's `Host`), the others redirect to it |
| `plugins`, `themes` | `[]` | wordpress.org slugs, `<slug>` or `<slug> <version>` |
| `languages` | `[]` | language packs for core, plugins and themes |
| `reloader` | `true` | the Reloader annotation; needs the namespace in Reloader's list |
| `phpFpm.pool` | `{}` | `pm.*` settings, e.g. `max_children: 6`; empty: the image's five workers |
| `phpFpm.resources` | 128Mi / 512Mi | the php-fpm container's memory |
| `wpCli.memoryLimit` | empty | PHP's `memory_limit` for wp-cli in the init container (`WP_CLI_PHP_ARGS`) |
| `wpCli.resources` | limit 256Mi | the init container `plugins`' memory |
| `nginx.contentSecurityPolicy` | empty | the CSP header; empty: none |
| `nginx.deniedUploadPaths` | `[]` | directories under uploads nginx refuses (regular expressions) |
| `ingress.middlewares` | `[]` | Traefik middlewares on the site's router |

A site's `values.yaml` also names, in comments, the 1Password item behind every OpenBao value
(title and link).

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
- **The chart creates its namespace** (`templates/namespace.yaml`), against Helm's convention of
  `--create-namespace`: the sites' Applications carry the finalizer, and Argo CD never deletes a
  namespace it created through `CreateNamespace=true` - only one among the Application's own
  objects goes with it. The namespace also carries trust-manager's label for the MariaDB CA. The
  chart is only ever rendered by Argo CD, one namespace per site, so Helm's reason (a release
  failing on, or taking over, an existing namespace) does not apply.
- **The ConfigMap keeps its name** (`web-config`); the pod template carries its checksum, so a
  changed file still rolls the pod.

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
2. OpenBao: the five entries from "Names from the release", each from a 1Password item.
3. `kubernetes/components/<site>/values.yaml` (hosts, plugins, the 1Password references) and a
   `README.md` with what is particular to it.
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
- **The pod hangs in an init container**: its log says which download failed
  (`kubectl --context d3strukt0r-prod-admin -n <site> logs deploy/web -c plugins`).

## Checking it

```shell
helm template <site> kubernetes/charts/wordpress -n <site> -f kubernetes/components/<site>/values.yaml
kubectl --context d3strukt0r-prod-admin -n <site> get pods,cronjobs,jobs,externalsecrets,certificates,ingress
kubectl --context d3strukt0r-prod-admin -n mariadb get databases.k8s.mariadb.com,users.k8s.mariadb.com,grants.k8s.mariadb.com <site>
kubectl --context d3strukt0r-prod-admin -n <site> logs deploy/web -c plugins
```
