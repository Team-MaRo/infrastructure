# mailomat watch

Every event mailomat records for the cluster's mail, copied into Loki, and an alert when a mail is
lost (`kubernetes/clusters/prod/mailomat-watch.yaml`, plain manifests in this directory, namespace
`mailomat-watch`). How mail leaves the cluster through mailomat is in
[`tofu/cloudflare/README.md`](../../../tofu/cloudflare/README.md) ("Sending through mailomat").

## Why

mailomat's free plan sends 200 mails in a rolling 30 days, refuses everything above that, and warns
no one; it offers no usage figure (support, 2026-11-19). Its Event API does work on the free
plan, one token per sending domain - but mailomat keeps the events only briefly (advertised 24
hours; on 2026-11-19 a message's events were still there three days later, until its delivery
ended). This copies every event into our Loki, which keeps it 30 days, and counts them, so a lost
mail raises an alert from mailomat's own events - not from a count of ours or from log lines.

## How it works

- **`watch.py`** (stdlib Python in `python:3.15-slim`, mounted from a ConfigMap, Keel) runs as a
  **Deployment** with one pod - not a CronJob, since Prometheus has to scrape its counters.
  `Recreate`, so two pods never log the same events.
- **Every 5 minutes, per domain**, it asks `GET https://api.mailomat.swiss/events` (`Accept:
  application/json` - `application/ld+json` answers 406) for the newest events, 100 per page,
  newest first, until a page holds one it has already seen. The API has no date filter.
- **Each new event is one log line**, oldest first: the complete JSON as mailomat returns it,
  plus `"domain"` and `"level"`. Alloy takes the pod's log into Loki like any other. The level is
  set here so Loki does not guess it from words in the line (it marked every retry red, since the
  payload says "failure" and "error"): `failure_perm` and `not_accepted` - a lost mail - are
  `error`, `failure_tmp` - a retry, normal - is `warn`, everything else `info`. Red in Grafana's
  log volume therefore means what `MailomatMailLost` reports.
- **Event types**: `accepted` (once per mail, with all recipients), `delivered`, `failure_tmp`
  (every failed attempt; mailomat retries for about three days), `failure_perm` (given up for
  good), `not_accepted` (refused, e.g. over the limit), `opened`/`clicked` (only with tracking,
  which is off). The `payload` holds the headers (From, To, **subject**), the size, the SMTP user
  (which app), and for failures the receiving server's answer, its MX and IP, attempt number and
  TLS. No body, no attachments.
- **A domain's first pass logs what mailomat still has without counting it** - history, not news
  for an alert. From then on every new event is counted.

## The state

The IDs of the last 1000 events per domain live in the ConfigMap `mailomat-watch-state`, which
**the pod creates and updates itself - deliberately not in git** (Argo CD's self-heal would wipe
it). Saved after the lines are logged: a crash in between logs a few events twice rather than
losing them (de-duplicate by `id` in a query). Its ServiceAccount may create ConfigMaps in this
namespace and read and replace exactly that one (`rbac.yaml`).

## Metrics and alerts

On `:9100/metrics` (PodMonitor), `/healthz` for the liveness probe:

| Metric | Meaning |
|---|---|
| `mailomat_events_total{domain, event_type}` | events seen since the pod started |
| `mailomat_poll_last_success_timestamp_seconds{domain}` | the last time a domain's events were fetched (starts at the pod's start) |
| `mailomat_poll_errors_total{domain}` | failed fetches (token revoked, API down) |

- **`MailomatMailLost`** - a `failure_perm` or `not_accepted` in the last 30 minutes, per domain.
  The mail is gone. The alert names the Loki query showing recipient, subject and the receiving
  server's answer. `failure_tmp` does not alert: retries are normal, the final outcome counts.
- **`MailomatWatchStale`** - a domain not fetched for 3 hours, or the pod not scraped. A token
  was revoked or the pod is down; after about a day mailomat no longer has the events.

There is **no alert on the 200-mail limit**: mailomat publishes no usage figure, and a count
rebuilt from our copies is ours, not mailomat's. Going over the limit shows up as
`not_accepted`, which `MailomatMailLost` reports.

## Searching sent mail

In Grafana (Explore, Loki):

```logql
{namespace="mailomat-watch", container="watch"} | json                                              # everything
{namespace="mailomat-watch", container="watch"} | json | recipient="someone@example.com"            # one recipient
{namespace="mailomat-watch", container="watch"} | json | eventType=~"failure_perm|not_accepted"     # lost mails
sum by (domain) (count_over_time({namespace="mailomat-watch", container="watch"} | json | eventType="accepted" [30d]))   # mails per domain, by hand
```

## The tokens

One mailomat API token per sending domain - every domain registered in mailomat, used or not, so
a new sender is covered from its first mail. **Each app brings its own token**: a Secret in this
namespace labelled `d3strukt0r.dev/mailomat-token: "true"`, every key a sending domain
(`mailomat.<domain>`), its value that domain's token. The pod lists those Secrets on every pass
(its Role may list Secrets here, nothing else), so an app's token arrives within 5 minutes and
leaves with the app - no restart, no list here. A domain that disappears also stops being
reported, so `MailomatWatchStale` does not fire for an app that was removed.

| Domain | Brought by | Token in OpenBao |
|---|---|---|
| `mailomat.arepazo.ch`, `mailomat.robines.space`, `mailomat.wundexpertinplus.com`, the private repository's site | the WordPress chart, `mailomatWatch.domain` in the site's values (`kubernetes/charts/wordpress/templates/mailomat-token.yaml`) | field `api-token` of `secret/<site>-mailomat`, next to the site's SMTP login |
| `mailomat.d3strukt0r.dev` (Zitadel, Matomo - no single owner) | this component, `external-secrets.yaml` (`mailomat-token-shared`) | `secret/mailomat-watch`, one field per domain; 1Password [`Mailomat | API | mailomat.d3strukt0r.dev`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=3h3sz5lq3fi2n647in6k73iham&h=my.1password.com) |

Creating a token: mailomat → the domain (selector at the top) → API → "Neues API Token
erstellen" (shown once). Into 1Password and OpenBao:

- **A WordPress site's** - a field of the site's existing mailomat item, then OpenBao:
  ```shell
  op item edit 'Mailomat | Prod | <site>' --account my.1password.com 'API.token[password]=<token>'
  ```
  ```shell
  bao kv patch secret/<site>-mailomat "api-token=$(op item get 'Mailomat | Prod | <site>' --account my.1password.com --fields API.token --reveal)"
  ```
- **A shared domain's** - its own item `Mailomat | API | mailomat.<domain>`, then OpenBao (the
  first one with `bao kv put` instead of `patch`, which needs the secret to exist):
  ```shell
  op item create --account my.1password.com --category 'API Credential' --title 'Mailomat | API | mailomat.<domain>' 'credential[password]=<token>'
  ```
  ```shell
  d=mailomat.<domain>; bao kv patch secret/mailomat-watch "$d=$(op item get "Mailomat | API | $d" --account my.1password.com --fields credential --reveal)"
  ```

A new app that sends mail sets its own Secret the same way (any name, the label, one key per
domain); nothing here changes.

## Runbooks

### When an alert fires

- **`MailomatMailLost`**: run the query from the alert. `not_accepted` from every domain at once
  usually means the free plan's limit - wait until older mails leave the 30-day window, or send
  less; `failure_perm` for one recipient is that address (gone, full, blocking us) - tell the
  sender's app owner.
- **`MailomatWatchStale`**: `kubectl --context d3strukt0r-prod-admin -n mailomat-watch logs
  deploy/mailomat-watch | grep '"error"'`. A `401` is a revoked or wrong token - create a new one
  and rebuild the secret (above).

### See what it logs now

```shell
kubectl --context d3strukt0r-prod-admin -n mailomat-watch logs deploy/mailomat-watch --tail=20
```
