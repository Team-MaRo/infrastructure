# pgAdmin

The web UI for the shared PostgreSQL at `https://pgadmin.d3strukt0r.dev`
(`kubernetes/clusters/prod/pgadmin.yaml`, plain manifests in this directory, image pinned,
namespace `pgadmin`). How the cluster's manifests fit together is in
[`kubernetes/README.md`](../../README.md); the database itself is in
[`../postgres/README.md`](../postgres/README.md).

## Design

- **Two logins, two purposes.** pgAdmin has user accounts of its own (each with its saved
  servers, preferences and query history), and logging in to them is **Zitadel only**
  (`AUTHENTICATION_SOURCES = ['oauth2']`): pgAdmin creates the account on the first login, and
  only for a token whose `groups` contain `infra-admin` (`OAUTH2_ADDITIONAL_CLAIMS`, the groups
  webhook's claim). Opening a server inside pgAdmin is a PostgreSQL login, with a role and its
  password - PostgreSQL knows nothing about Zitadel. For full access that is the role `admin`
  ("The role `admin`" in [`../postgres/README.md`](../postgres/README.md)).
- **No oauth2-proxy gate in front** (user decision): pgAdmin does the Zitadel check itself, so
  its login page faces the internet. pgAdmin publishes security fixes nearly every month,
  several of them for flaws reachable before a login - keeping the image current matters more
  here than for the other UIs (see "Upgrading").
- **A public Zitadel client** (`tofu/zitadel/apps_pgadmin.tf`): authorization code with PKCE and
  no secret, like Grafana's; the client ID, not secret, is in `config_system.py`. Logging out of
  pgAdmin ends the Zitadel session too (`OAUTH2_LOGOUT_URL`).
- **pgAdmin saves no database passwords** (`ALLOW_SAVE_PASSWORD = False`, user decision). A role's
  password lives in PostgreSQL (as a hash) and in 1Password; pgAdmin asks for it when a server
  is opened and keeps it for that session only. Saving would mean a second, encrypted copy in
  pgAdmin's database, unlocked with a master password per pgAdmin user (with a Zitadel login
  pgAdmin has no password of the user to derive the key from) - so the master password is off
  too (`MASTER_PASSWORD_REQUIRED = False`).
- **The configuration database is in the shared PostgreSQL**, `pgadmin` with the role `pgadmin`
  (`database.yaml`, the pattern every PostgreSQL app follows), reached with `verify-full` against
  CloudNativePG's CA (ConfigMap `postgres-ca` from trust-manager). The CA is mounted where libpq looks by default,
  `~/.postgresql/root.crt`, rather than named with `sslrootcert`: pgAdmin drops file paths such
  as `sslrootcert` from the copy of a shared server it makes for each user (so one user cannot
  point at another's files), and the default location applies to every connection anyway.
  pgAdmin migrates the database at start. So the pod needs no volume: `/var/lib/pgadmin` is an
  emptyDir holding only sessions and the file manager's storage - a restart logs everyone out
  and empties the file manager, nothing else. The URI is
  passed as `PGADMIN_CONFIG_CONFIG_DATABASE_URI_FILE`, not in `config_system.py`: the entrypoint
  checks that variable to decide whether this is a first start.
- **An internal user is created anyway** (`PGADMIN_DEFAULT_EMAIL`, password from
  `secret/pgadmin`): pgAdmin's first start requires one. With only the Zitadel login enabled
  nobody can log in with it; it owns the shared server.
- **One shared server, from git**: `servers.json` defines "PostgreSQL"
  (`postgres-rw.postgres.svc`, which follows a failover, `verify-full`, default user `admin`),
  shared with every pgAdmin user and loaded again on every start
  (`PGADMIN_REPLACE_SERVERS_ON_STARTUP`), so the file stays the truth. Servers a user adds
  themselves stay theirs.
- **The image runs as its own user, 5050.** Since 9.16 its entrypoint notices the missing
  privileges (`allowPrivilegeEscalation: false`) and runs without the capability it would need
  for port 80; the port is set to 8080 explicitly. `PGADMIN_DISABLE_POSTFIX`: otherwise the
  entrypoint starts a mail server for password resets through `sudo`, which fails without
  privilege escalation - and Zitadel handles passwords. The root filesystem is not read-only:
  the entrypoint writes its config into the image at start.
- **Settings that differ from pgAdmin's defaults** (`config_system.py`, loaded last):
  `SESSION_COOKIE_SECURE`, and `ENHANCED_COOKIE_PROTECTION = False` - it ties a session to the
  client's IP address, but behind Cloudflare's proxy pgAdmin sees a Cloudflare address that can
  change between two requests (Traefik does not trust Cloudflare's forwarded headers).
- **One replica, `Recreate`**: the old pod must stop before the new one migrates the database.
  8 Gunicorn threads instead of 25 for a handful of users. Memory: request 256Mi, limit 1Gi -
  large result sets are held in memory; adjust once it has run for a while.
- **Restarted by Reloader** when Secret `pgadmin` changes in OpenBao: the entrypoint reads the
  `*_FILE` secrets at start ([`../reloader/README.md`](../reloader/README.md)). The ConfigMap's
  name is hashed, so a change to it rolls the pod anyway.

## Before the first sync

Three values in 1Password and OpenBao (logging in as in [OpenBao](../openbao/README.md)):

```shell
export BAO_ADDR=https://openbao.d3strukt0r.dev
bao login -method=oidc -no-print
op item create --account my.1password.com --vault Private --category login --title 'PostgreSQL | Prod | admin' \
  --url 'https://pgadmin.d3strukt0r.dev/' --generate-password='letters,digits,symbols,20' 'username=admin' >/dev/null
op item create --account my.1password.com --vault Private --category login --title 'PostgreSQL | Prod | pgadmin' \
  --url 'https://pgadmin.d3strukt0r.dev/' --generate-password='letters,digits,20' 'username=pgadmin' >/dev/null
op item create --account my.1password.com --vault Private --category login --title 'pgAdmin | Prod | Default user' \
  --url 'https://pgadmin.d3strukt0r.dev/' --generate-password='letters,digits,symbols,20' 'username=pgadmin@d3strukt0r.dev' >/dev/null
jq -n --arg p "$(op item get 'PostgreSQL | Prod | admin' --account my.1password.com --vault Private --fields password --reveal)" \
  'if ($p|length)!=20 then error("password must be 20 characters - 1Password lookup failed?") else {"password":$p} end' \
| bao kv put secret/postgres-admin -
jq -n --arg p "$(op item get 'PostgreSQL | Prod | pgadmin' --account my.1password.com --vault Private --fields password --reveal)" \
  'if ($p|length)!=20 then error("password must be 20 characters - 1Password lookup failed?") else {"password":$p} end' \
| bao kv put secret/postgres-apps/pgadmin -
jq -n --arg p "$(op item get 'pgAdmin | Prod | Default user' --account my.1password.com --vault Private --fields password --reveal)" \
  'if ($p|length)!=20 then error("password must be 20 characters - 1Password lookup failed?") else {"default-password":$p} end' \
| bao kv put secret/pgadmin -
```

The configuration database's password has letters and digits only, since it sits inside a URL.

1Password: [`PostgreSQL | Prod | admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=4apn53q53ypoi2ruryq6voicz4&h=my.1password.com), [`PostgreSQL | Prod | pgadmin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=57fj3tlsl6rvegv46lio457qnq&h=my.1password.com),
[`pgAdmin | Prod | Default user`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=d3272yhi4gncweqqpt4jcqm5qi&h=my.1password.com).

Then `tofu/zitadel` (the app) and `tofu/cloudflare` (the record `pgadmin.d3strukt0r.dev`).

The first login: "Login with Zitadel", open "PostgreSQL" under "prod" and enter the password of
`admin` from 1Password.

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin -n pgadmin get pods,externalsecrets,certificates
kubectl --context d3strukt0r-prod-admin -n postgres get databaseroles.postgresql.cnpg.io,databases.postgresql.cnpg.io
```

In pgAdmin's query tool, `SELECT ssl, version, cipher FROM pg_stat_ssl WHERE pid = pg_backend_pid();`
shows the connection's TLS.

## Upgrading

Bump the tag and the digest in `deployment.yaml` together (pgAdmin re-pushes even version tags,
so the digest is what pins). pgAdmin releases about monthly, nearly every release with security
fixes (https://www.pgadmin.org/news/); read the release notes for changes to the container or to
OAuth2. The configuration database is migrated on start.

## Break-glass: without pgAdmin

When Zitadel or pgAdmin is down, `psql` inside the PostgreSQL pod works without either; the
runbooks in [`../postgres/README.md`](../postgres/README.md) use it.
