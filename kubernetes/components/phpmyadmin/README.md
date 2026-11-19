# phpMyAdmin

The web UI for the shared MariaDB at `https://phpmyadmin.d3strukt0r.dev`
(`kubernetes/clusters/prod/phpmyadmin.yaml`, plain manifests in this directory, the image's 5.x
line kept current by Keel, namespace `phpmyadmin`). How the cluster's manifests fit together is in
[`kubernetes/README.md`](../../README.md); the database itself is in
[`../mariadb/README.md`](../mariadb/README.md).

## Design

- **Two logins.** The Zitadel gate first (oauth2-proxy, role `infra-admin`, the Middleware
  `oauth2-proxy` in this namespace - see [`../oauth2-proxy/README.md`](../oauth2-proxy/README.md)),
  then phpMyAdmin's own login with a MariaDB user (`auth_type` cookie, the image's default when
  no user is configured). So a stolen gate session alone reaches no data, MariaDB's grants still
  decide what each login may do, and no administrator password is stored for phpMyAdmin. For
  full access log in as `root` with the password from 1Password
  [`MariaDB | Prod | Root & replication`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=gsadufiuptw2el3nhww22ub22m&h=my.1password.com).
- **The official image `docker.io/phpmyadmin`, fpm variant on Debian, with nginx - as Matomo.**
  There is no maintained chart from the phpMyAdmin team, and Bitnami's image and chart are frozen
  since 2025. Three containers: the init container `copy` copies the code (66 MB) from the image
  into an emptyDir, `php-fpm` runs it, `nginx` (`nginx-unprivileged`, port 8080) serves the
  static files and passes PHP to php-fpm on `127.0.0.1:9000` (`default.conf`, `php-fpm.conf`).
  Debian, not Alpine: PHP on musl is measurably slower. Until 2026-11-16 it was the Apache
  variant, which had to bind port 80 as www-data and needed the pod sysctl
  `net.ipv4.ip_unprivileged_port_start=0`; nginx on 8080 needs nothing of the kind.
- **Keel keeps it current** (`phpmyadmin:5-fpm` and `nginx-unprivileged:mainline`, both rolled
  out when a new image appears under the tag - [`../keel/README.md`](../keel/README.md)): `5-fpm`
  follows every 5.x release and the base image's PHP fixes (PHP2Shell, 2026-10, arrived that way
  in the other PHP apps). 6.0 is not under that tag.
- **Only phpMyAdmin's four entry points reach PHP** (`index.php`, `url.php`,
  `js/messages.php`, `show_config_errors.php`); every other `.php`, its internal directories
  (`libraries`, `templates`, `vendor`, `tmp`, `sql`, `locale`), dotfiles and the build files
  answer 403. nginx takes uploads up to 100 MB (`client_max_body_size`, the same as
  `UPLOAD_LIMIT`) and waits up to 600 s for PHP (`MAX_EXECUTION_TIME`). Tested on kind
  (2026-11-16): login, a 50 MB import in 8.6 s, a 50 MB export, the 403s.
- **It runs as www-data (uid 33), nginx as its own user (101) with group 33**, although the image
  starts as root: everything the entrypoint writes - the session secret in `/etc/phpmyadmin`,
  the sessions in `/sessions`, `tmp/` in the code - already belongs to www-data. The code is
  copied without the image's modes (its directories are read-only, so `cp -a` could not fill
  them). Not `HIDE_PHP_VERSION`: for it the entrypoint rewrites a root-owned file. The root
  filesystem is not read-only, since the entrypoint writes into `/etc/phpmyadmin` at start.
- **No errors in the page** (`php.ini`, mounted into `conf.d`): the image loads no `php.ini`, so
  PHP's defaults would show errors with file paths and internal state in the page
  (`display_errors`), against phpMyAdmin's "Securing your phpMyAdmin installation"; they go to
  the container log instead (`log_errors`), as in the WordPress and Matomo images.
  `expose_php` stays on (the PHP version in `X-Powered-By`), as in the other PHP apps: hiding the
  version is obscurity, patching is the protection. The rest of that list is covered: `libraries/`, `templates/` and the
  temp directory denied in nginx, `setup/` removed by the image, cookie login behind an
  authentication proxy (the Zitadel gate), HTTPS through Cloudflare. HSTS is not sent - a
  cluster-wide question, not phpMyAdmin's. phpMyAdmin publishes no nginx configuration of its
  own; `default.conf` follows the layout of the image.
- **TLS to MariaDB**, verified against mariadb-operator's CA (`PMA_SSL`, `PMA_SSL_VERIFY`,
  `PMA_SSL_CA`), the ConfigMap `mariadb-ca` that trust-manager puts into this namespace (the
  namespace's label, [`../trust-manager/README.md`](../trust-manager/README.md)). It connects to
  `mariadb-primary.mariadb.svc`, which follows a failover.
- **The configuration storage is a database of its own**, `phpmyadmin` in the shared MariaDB
  (`database.yaml`): bookmarks, SQL history, the designer and relations, settings saved on the
  server, recent and favourite tables. phpMyAdmin reaches it as the control user `phpmyadmin`,
  whoever is logged in; that user has every privilege on its own database and nothing else, and
  must connect with TLS. The objects live in the `mariadb` namespace with `cleanupPolicy: Skip`,
  so removing phpMyAdmin keeps the database and the user - the pattern every MariaDB app follows
  ("Adding an app" in [`../mariadb/README.md`](../mariadb/README.md)). Its password reaches the
  pod as a file (`PMA_CONTROLPASS_FILE`), not as an environment variable.
- **The session secret is not stored.** The entrypoint generates a new `blowfish_secret` on
  every start, so a restart only logs everyone out of phpMyAdmin - no secret to keep.
- **One replica, `Recreate`**: sessions are files in the pod. Memory: request 112Mi
  (real-memory p95 101 MiB; working set p95 126 MiB, peak 128 MiB over 2026-10-23..11-02), limit
  640Mi - PHP may use up to 512 MB for one
  large import or export (`MEMORY_LIMIT`). Imports up to 100 MB (`UPLOAD_LIMIT`, default 2 MB) -
  the most Cloudflare's proxy accepts in one request on the free plan, so a higher value would not
  help.
- **Restarted by Reloader** when Secret `phpmyadmin` changes in OpenBao: the entrypoint reads
  `PMA_CONTROLPASS_FILE` into the environment at start
  ([`../reloader/README.md`](../reloader/README.md)).

## Before the first sync

The control user's password must be in OpenBao (logging in as in
[OpenBao](../openbao/README.md)):

```shell
export BAO_ADDR=https://openbao.d3strukt0r.dev
bao login -method=oidc -no-print
op item create --account my.1password.com --vault Private --category login --title 'MariaDB | Prod | phpmyadmin' \
  --url 'https://phpmyadmin.d3strukt0r.dev/' --generate-password='letters,digits,symbols,20' 'username=phpmyadmin' >/dev/null
jq -n --arg p "$(op item get 'MariaDB | Prod | phpmyadmin' --account my.1password.com --vault Private --fields password --reveal)" \
  'if ($p|length)!=20 then error("password must be 20 characters - 1Password lookup failed?") else {"password":$p} end' \
| bao kv put secret/mariadb-apps/phpmyadmin -
```

1Password: [`MariaDB | Prod | phpmyadmin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=ylqengf5omkwaov4jlytatw4lu&h=my.1password.com).

The record `phpmyadmin.d3strukt0r.dev` comes from `tofu/cloudflare` (without it the wildcard
sends the name to the old server).

On the first login phpMyAdmin notices that the tables of its configuration storage are missing
and offers to create them ("Find out why" in the footer, then "Create"); the control user may do
that in its own database.

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin -n phpmyadmin get pods,externalsecrets,certificates
kubectl --context d3strukt0r-prod-admin -n mariadb get databases.k8s.mariadb.com,users.k8s.mariadb.com,grants.k8s.mariadb.com
kubectl --context d3strukt0r-prod-admin -n phpmyadmin logs deploy/phpmyadmin -c php-fpm
kubectl --context d3strukt0r-prod-admin -n phpmyadmin logs deploy/phpmyadmin -c nginx
```

In phpMyAdmin's SQL tab, `SHOW SESSION STATUS LIKE 'Ssl_cipher'` shows the cipher of the TLS
connection.

## Upgrading

Every 5.x release and new base image arrives by itself through Keel (`5-fpm`). **If a feature
suddenly answers 403 after a new release** (nginx's log shows the path), the release brought a new
PHP entry point: add it to the entry points in `default.conf` - only those four reach PHP. 6.0 is
a deliberate change of the tag in `deployment.yaml`, in the init container and in `php-fpm` alike,
after reading its release notes and checking the entry points. phpMyAdmin releases are rare
(5.2.3 in 2025-10); the security announcements are at https://www.phpmyadmin.net/security/.

## Break-glass: without the gate

When Zitadel or oauth2-proxy is down, the Ingress refuses everything (fail closed). A
port-forward to phpMyAdmin does not help: `PMA_ABSOLUTE_URI` points its links at the public
name. Use the `mariadb` client inside the MariaDB pod instead, as the runbooks in
[`../mariadb/README.md`](../mariadb/README.md) do - the database never depends on phpMyAdmin.
