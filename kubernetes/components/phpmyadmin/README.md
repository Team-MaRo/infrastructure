# phpMyAdmin

The web UI for the shared MariaDB at `https://phpmyadmin.d3strukt0r.dev`
(`kubernetes/clusters/prod/phpmyadmin.yaml`, plain manifests in this directory, image pinned,
namespace `phpmyadmin`). How the cluster's manifests fit together is in
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
- **The official image `docker.io/phpmyadmin`, Apache variant, plain manifests.** There is no
  maintained chart from the phpMyAdmin team, and Bitnami's image and chart are frozen since
  2025. The tag is pinned; it is re-pushed when the base image changes, so the same tag can
  bring newer system packages on a pod restart.
- **It runs as www-data (uid 33), although the image starts as root.** Everything the
  entrypoint writes - the session secret in `/etc/phpmyadmin`, the sessions in `/sessions`,
  `/var/www/html/tmp` - already belongs to www-data. Apache keeps listening on port 80, which a
  non-root process may bind only with the pod sysctl `net.ipv4.ip_unprivileged_port_start=0`;
  it is one of the namespaced sysctls the restricted standard allows and applies to the pod's
  network namespace only. Not `APACHE_PORT` or `HIDE_PHP_VERSION`: for those the entrypoint
  rewrites root-owned files, which fails as www-data. The root filesystem is not read-only,
  since the entrypoint writes into the image at start.
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
- **One replica, `Recreate`**: sessions are files in the pod. Memory: request 128Mi, limit
  640Mi - idle it needs far less, but PHP may use up to 512 MB for one large import or export
  (`MEMORY_LIMIT`). Imports up to 100 MB (`UPLOAD_LIMIT`, default 2 MB) - the most Cloudflare's
  proxy accepts in one request on the free plan, so a higher value would not help.
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
```

In phpMyAdmin's SQL tab, `SHOW SESSION STATUS LIKE 'Ssl_cipher'` shows the cipher of the TLS
connection.

## Upgrading

Bump the tag in `deployment.yaml`. phpMyAdmin releases are rare (5.2.3 in 2025-10); the
security announcements are at https://www.phpmyadmin.net/security/. 5.x reaches its end once 6.0
is out - that upgrade needs its release notes read first.

## Break-glass: without the gate

When Zitadel or oauth2-proxy is down, the Ingress refuses everything (fail closed). A
port-forward to phpMyAdmin does not help: `PMA_ABSOLUTE_URI` points its links at the public
name. Use the `mariadb` client inside the MariaDB pod instead, as the runbooks in
[`../mariadb/README.md`](../mariadb/README.md) do - the database never depends on phpMyAdmin.
