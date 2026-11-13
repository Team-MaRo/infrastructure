# wedding-manuele-robine

The wedding website at `https://manuele-robine.wedding` (`kubernetes/clusters/prod/wedding-manuele-robine.yaml`,
plain manifests in this directory, namespace `wedding-manuele-robine`). Moved from prod-old, where it
ran under Docker Compose. How the cluster's manifests fit together is in
[`kubernetes/README.md`](../../README.md).

## What runs

| Part | Image | Reached at |
|---|---|---|
| `pwa` - the website (nginx) | `d3strukt0r/wedding-manuele-robine:pwa-latest` | `manuele-robine.wedding`, `www.manuele-robine.wedding`, `wedding-manuele-robine.d3strukt0r.dev` |
| `api` - Symfony (nginx, php-fpm, a Messenger worker, cron) | `d3strukt0r/wedding-manuele-robine:api-latest` | `api-wedding-manuele-robine.d3strukt0r.dev`, and `http://api` from the PWA |
| Database `wedding_manuele_robine` | the shared MariaDB | `mariadb-primary.mariadb.svc` (`database.yaml`) |
| Media (uploads) | Hetzner Object Storage | bucket `d3strukt0r-prod-wedding-manuele-robine`, private, served by the API |

## Design

- **Root images, exempt from restricted Pod Security** - the one app on
  `k3s_psa_exempt_namespaces` (`ansible/roles/k3s/defaults/main.yml`). Both images run
  supervisord, nginx and php-fpm as root on port 80. **TODO: make the images rootless** (an
  unprivileged user, port 8080), then remove the exemption and give the pods the restricted
  securityContext.
- **The database without TLS** - **TODO: TLS to MariaDB.** Doctrine's PDO only connects with TLS
  given options in the app's `doctrine.yaml` (`PDO::MYSQL_ATTR_SSL_CA` with the CA from
  trust-manager's `mariadb-ca`); then label the namespace `trust.d3strukt0r.dev/mariadb-ca` and set
  `require.ssl` on the `User`. Until then the connection is plain inside the cluster; between
  nodes WireGuard encrypts it.
- **Floating tags, rolled out by Keel**; both Deployments carry Keel's annotations,
  `imagePullPolicy: Always` and the image without `docker.io/`
  ([`../keel/README.md`](../keel/README.md)). Watchtower did this on prod-old.
- **Secrets from OpenBao** (`external-secrets.yaml`): `secret/wedding-manuele-robine` (APP_SECRET,
  the JWT passphrase and key pair, the Google Maps key), `secret/mariadb-apps/wedding-manuele-robine`
  (the database password, letters and digits - it sits in `DATABASE_URL`) and
  `secret/wedding-manuele-robine-s3` (the bucket's key). The JWT keys are mounted as files at
  `config/jwt/`.
- **One API replica, `Recreate`**: it runs Doctrine's migrations at start, and its sessions are
  files on an emptyDir - a restart logs admins out, nothing else is lost.
- **One container does everything** - supervisord runs nginx, php-fpm, the Messenger worker and
  cron, and the start script runs the migrations. **TODO: split the API by kind of process**, one
  image with a command per role, so the same image runs under Docker Compose as well:
  - **web** - nginx and php-fpm as two containers of one Deployment (nginx's image a build target
    holding `public/`); several replicas once sessions live in Valkey (`valkey-state`) instead of
    files;
  - **Messenger worker** - a second Deployment, same image, command `messenger:consume`;
  - **scheduled tasks** - a CronJob per task, same image, the console command as its command
    (`concurrencyPolicy: Forbid`, each run a Job with its own logs, a failed run alerts like any
    Job); in Compose a `cron` service from the same image running supercronic (a cron for
    containers, non-root, logging to stdout) with a crontab of the same commands - the schedule
    is kept in both places;
  - **migrations** - a Job as an Argo CD `PreSync` hook; in Compose a one-shot service the others
    wait for (`depends_on: condition: service_completed_successfully`).

  Then the single replica and `Recreate` can go.
- **Media at the bucket's root** (`S3_STORAGE_PREFIX` empty): the bucket is the app's own. On
  DigitalOcean they were under `prod/wedding-manuele-robine` in a shared bucket. The app signs for
  region `eu-west-1` (hard-coded), which Hetzner accepts.
- **php-fpm's pool figures in Prometheus** (job `wedding-manuele-robine/php-fpm`,
  `pod-monitor.yaml`; alerts and the dashboard "php-fpm" are kube-prometheus-stack's), without
  touching the image: two files from the ConfigMap `api-config` go into the image's own include
  folders. `config/php-fpm-metrics.conf` becomes `/etc/php/8.3/fpm/pool.d/zz-metrics.conf` - a
  status pool of its own (`pm.status_listen`), which answers while every worker is busy, a
  slow log (requests over 2 s write their PHP backtrace into the container log) and a memory
  limit per request of 256M (the image's `php.ini` sets none, so one runaway request could fill
  the container's 704Mi and kill every other request with it);
  `pm.status_path` is already in the image's `www.conf`. `config/nginx-metrics.conf` is copied
  by the init container `nginx-config` into `/etc/nginx/conf.d/php-fpm-metrics.conf` (an
  emptyDir; the image's conf.d is otherwise empty) - not mounted, because the image's start
  script runs envsubst on every file under `/etc/nginx` and writes it back, and a ConfigMap mount
  is read-only (mounted, the API did not start, 2026-10-18). It is a server on port 9253 that
  passes php-fpm's own Prometheus output (`openmetrics`) on as `/metrics` - the same block as
  in the WordPress sites, see [`../../charts/wordpress/README.md`](../../charts/wordpress/README.md).
  **The pool path carries the image's PHP version (8.3)**: on a new PHP version the file is no
  longer read and `PhpFpmStatusDown` fires - then change the version in `api.yaml`.
- **Memory**, over 2026-10-23..11-02, the request from real memory's p95: the API 240Mi / 704Mi
  (real p95 237 MiB; working set p95 313 MiB, peak 316 MiB - plus one request at `memory_limit`
  256M, which 512Mi no longer held); the PWA 144Mi / 320Mi (142; 158, peak 158 MiB).
- Dropped from prod-old: the app's own MariaDB (now the shared one, backed up by the operator),
  phpMyAdmin (the shared one at `phpmyadmin.d3strukt0r.dev`) and tiredofit/db-backup.

## How it moved from prod-old (2026-10-02)

1. **Secrets** into 1Password and OpenBao - the values from prod-old's `.env` and `jwt/`, read
   over SSH into `op` and `bao kv put` without ever printing them.
2. **Deploy**; the API's migrations created the schema in the empty database.
3. **Tested before DNS moved**, against a node's own IP (not `prod.d3strukt0r.dev`, which is
   proxied and so reaches whatever the name's DNS points at):
   `curl --resolve manuele-robine.wedding:443:<node IP> https://manuele-robine.wedding/`.
4. **Database from the last full backup.** prod-old's live database had been empty since
   2026-04-29 (its data directory was no longer mounted), so the data came from the last full
   tiredofit dump, `db-backup-d3strukt0r/prod/wedding-manuele-robine/mariadb_db_db_20260429-092658.sql.gz`
   on DigitalOcean, checked against its `.sha1` and imported into `wedding_manuele_robine` on the
   primary. The dump has no `CREATE DATABASE`/`USE` and drops each table before creating it.
5. **Media**: `aws s3 sync` of `s3://eu-prod-d3strukt0r/prod/wedding-manuele-robine/` from
   DigitalOcean to a local directory, then to the bucket's root - 2248 objects, 8.95 GB. Six rows
   of `file` (two uploads in three sizes each) had no object on DigitalOcean either.
6. **DNS** in `tofu/cloudflare`: the apex of `manuele-robine.wedding` and the two
   `d3strukt0r.dev` names to `prod.d3strukt0r.dev`.

Gatus checks the website and the API's `/ping` (group Apps).

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin -n wedding-manuele-robine get pods,externalsecrets,certificates,ingress
kubectl --context d3strukt0r-prod-admin -n wedding-manuele-robine logs deploy/api
kubectl --context d3strukt0r-prod-admin -n mariadb get databases.k8s.mariadb.com,users.k8s.mariadb.com,grants.k8s.mariadb.com wedding-manuele-robine
```
