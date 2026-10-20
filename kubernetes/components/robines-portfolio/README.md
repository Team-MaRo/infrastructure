# robines-portfolio

The old WordPress portfolio, public at `https://old.robines.space`
(`kubernetes/clusters/prod/robines-portfolio.yaml`, namespace `robines-portfolio`). `robines.space`
and `www.robines.space` are a Cloudflare Worker - the live site - and have nothing to do with
this. Moved from prod-old, where it ran under Docker Compose with the whole web root on a bind
mount and plugins installed through wp-admin.

It is the WordPress chart ([`../../charts/wordpress/README.md`](../../charts/wordpress/README.md):
what runs, the design, the runbooks) with the values in `values.yaml`.

The database is `robines_portfolio` in the shared MariaDB, the uploads are in the bucket
`d3strukt0r-prod-robines-portfolio` (`tofu/objectstorage`).

## What is particular to it

- **Its own address, `old.robines.space`.** The URLs in the content were rewritten from
  `robines.space` at the move.
- **Plugins**: everything prod-old had, active or not, except the premium theme `mintraro-pro`,
  which is inactive and not on wordpress.org.
- **`de_CH`**: the site runs in Swiss German; its language packs are installed at every start.
- **Object cache in valkey-cache, database 1** (since 2026-10-20; the chart's "An object cache in
  valkey-cache").
- **Login through Zitadel** (since 2026-10-20; the chart's "Login through Zitadel"): the project
  `robines-portfolio` in `tofu/zitadel`, its roles `robines-portfolio:administrator` (the user and
  his wife) and `robines-portfolio:editor`. The wife's account `ruby` is linked to her Zitadel
  user by hand. **Break-glass: the local `admin`** (`info@robines.space`, an address no Zitadel
  user has), its password in 1Password
  [`Robines Portfolio CMS | Prod | Admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=amt2kkr67jxqe5uajaz5ine34i&h=my.1password.com).
- **A permissive Content-Security-Policy** like prod-old's, with the frames the site embeds
  (YouTube, Twitter).
- **The chart's pool** (image's five workers, 512Mi): its calculation is in `values.yaml`; the
  five have never all been busy. To recompute, see "Sizing the pool" in the chart's README.
- **Watched by Gatus** ("Robines Portfolio (old)" on the status page, `components/gatus`): the home
  page every five minutes.
- **Not for search engines.** The live site is the Worker at robines.space; this archive tells
  search engines `noindex, nofollow` (wp-admin → Settings → Reading → "Discourage search engines",
  set 2026-10-04 - in the database, not in git). Before the move its pages named robines.space as
  their address, so the rewrite to old.robines.space made them a site of their own.
- **Mail through mailomat** (since 2026-10-18; before, WP Mail SMTP was on PHP's `mail()`, which
  needs a local `sendmail` the image does not have, so nothing went out - on prod-old neither).
  Password resets and form notifications (WPForms, Gutena Forms) leave through mailomat's SMTP
  user `web@mailomat.robines.space`; the sender stays wp-admin's (`info@robines.space`). The
  domain's own mail stays at addy; mailomat signs as `mailomat.robines.space`, which counts for
  `robines.space` since its DMARC record is relaxed
  ([`../../../tofu/cloudflare/README.md`](../../../tofu/cloudflare/README.md), "Sending through
  mailomat"). No IP restriction on the SMTP user: the password is its only guard. The mailomat
  plan's 200 mails a month are shared with the other sites
  ([`../arepazo/README.md`](../arepazo/README.md)).
- **Not migrated**: prod-old's Duplicator package (`wp-content/backups-dup-lite`, 222 MB, from
  2025-10-14) and the `robines-portfolio.d3strukt0r.dev` name.

## Secrets

| OpenBao | Fields | From |
|---|---|---|
| `secret/robines-portfolio` | `auth-key`, `secure-auth-key`, `logged-in-key`, `nonce-key`, `auth-salt`, `secure-auth-salt`, `logged-in-salt`, `nonce-salt` (prod-old's) | 1Password [`Robines Portfolio CMS | Prod | App`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=fkyq7sgplhkvasmf3aiwh2acge&h=my.1password.com) |
| `secret/mariadb-apps/robines-portfolio` | `password` (20 characters) | 1Password [`MariaDB | Prod | robines-portfolio`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=swneckmu7nc3tkxv5c2njudv2q&h=my.1password.com) |
| `secret/robines-portfolio-s3` | `access-key`, `secret-key` (read and write) | 1Password [`Hetzner | S3 | prod robines-portfolio`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=j3phtimnb5zbwonzf655meqrb4&h=my.1password.com) |
| `secret/robines-portfolio-uploads-proxy-s3` | `access-key`, `secret-key` (read only) | 1Password [`Hetzner | S3 | prod robines-portfolio uploads-proxy`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=3icb7ypn2ub7bk7x2yzbwhkrfm&h=my.1password.com) |
| `secret/valkey-apps/robines-portfolio` | `password` (20 characters; its user in valkey-cache) | 1Password [`Valkey | Prod | robines-portfolio`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=27cbbdterppy35hbjvjoivmmn4&h=my.1password.com) |
| `secret/robines-portfolio-oidc` | `client-secret` (its app in Zitadel) | 1Password [`Zitadel | Prod | robines-portfolio`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=p5zrexzctw27dvxg2y7e4czvla&h=my.1password.com) |
| `secret/robines-portfolio-mailomat` | `username`, `password` (the SMTP user at mailomat) | 1Password [`Mailomat | Prod | robines-portfolio`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=u747dsojova7eol3x2ikl3qxm4&h=my.1password.com) (an E-Mail item, SMTP section) |

A changed value in OpenBao reaches the pod through External Secrets (within five minutes, or at once
with `kubectl --context d3strukt0r-prod-admin -n robines-portfolio annotate externalsecret --all force-sync=$(date +%s) --overwrite`),
and Reloader then restarts it (`components/reloader`).

## Checking it

```shell
helm template robines-portfolio kubernetes/charts/wordpress -n robines-portfolio -f kubernetes/components/robines-portfolio/values.yaml
kubectl --context d3strukt0r-prod-admin -n robines-portfolio get pods,cronjobs,jobs,externalsecrets,certificates,ingress
kubectl --context d3strukt0r-prod-admin -n mariadb get databases.k8s.mariadb.com,users.k8s.mariadb.com,grants.k8s.mariadb.com robines-portfolio
kubectl --context d3strukt0r-prod-admin -n robines-portfolio logs deploy/web -c plugins
```
