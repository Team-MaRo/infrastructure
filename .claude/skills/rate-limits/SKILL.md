---
name: rate-limits
description: Check how close each app's cloudflare-only rate limit comes to refusing genuine use - one namespace or the whole cluster, as a table, over the last 7 days or any window. Use it whenever someone reports "Too Many Requests" or 429s on a site or UI, before raising or lowering a namespace's rate limit, after changing one, or when asked "how close are we to the rate limit", "is the limit too tight", "which apps hit the rate limit".
argument-hint: "[namespace] [--since 7d]"
---

# Rate limits

Every proxied route goes through its namespace's copy of `cloudflare-only`, whose
`cloudflare-rate-limit` is a token bucket per visitor (`CF-Connecting-IP`): `burst` tokens at
most, refilled at `average` a second, one per request. The default (20/100) is in
`kubernetes/charts/cloudflare-only/files/middleware.yaml`; an app whose genuine use it refuses
raises its own copy - why, how and the 80 % target are in that chart's README, "Raising the limit
for one app". This skill measures; changing a limit follows that README.

## Run

```sh
.claude/skills/rate-limits/scripts/rate-limits.sh [namespace] [--since 7d]
```

Read-only. Without a namespace it covers every namespace with a `cloudflare-rate-limit`; `--since`
takes `<n>d` or `<n>h` (default 7d, at most the 30 days Tempo and Loki keep). The whole cluster
over 7 days takes about 3 minutes - Tempo answers the searches largely one at a time, so asking
for four namespaces at once (as the script does) hardly shortens it; one namespace takes 20-60 s.
Say so before starting it, and run one namespace when the question is about one app.

## The table

| Column | Meaning |
|---|---|
| LIMIT, SET | average/burst now, and when they were last set |
| REQUESTS | requests the app answered in the window (Traefik's spans in Tempo); left out: 429s and other 4xx (mostly scanners' 404s and requests without a login) and any request to a probed path (`scripts/probe-paths.re` - behind a login gate a probe gets a redirect) |
| PEAK/S | the busiest single second |
| BUCKET-USED, AT | replaying those requests through today's limit: how far the bucket emptied at worst, and when |
| WOULD-REFUSE | requests that replay would have refused |
| 429-GENUINE | real refusals since SET (Traefik's access log in Loki) from visitors who never probed for secrets |
| 429-SCANNER | real refusals in the whole window from visitors who did (`.env`, `wp-config`, `.git`, backups, ...) - the limit working |
| STATUS | `refusing` (genuine 429s since SET), `tight` (80 % or more, or the replay refuses), `ok`, `quiet` (no traffic) |

Two simplifications, both on the safe side - the real usage is lower:

- **All visitors share one bucket in the replay.** Spans carry no visitor address. For a UI one
  person uses (Grafana, pgAdmin) that is exact; for a public site with many visitors (arepazo,
  Zitadel's login) it overstates, so a `tight` there needs a look at who was busy at AT before
  anything changes.
- **All three Traefik nodes share one bucket.** Each node keeps its own; Cloudflare spreads a
  visitor's requests over them.

## What to do with it

- **`refusing`**: find the visitor and paths (Loki: `{namespace="kube-system",
  container="traefik"} |= "\"DownstreamStatus\":429" | json | RequestHost="<host>"`). Genuine use
  - size the namespace's limit so the replay of that use takes about 80 % of the burst (README).
  A missed scanner - its paths belong in `scripts/probe-paths.re` (one regex, used for both the
  429s and the replay).
- **`tight`**: check whether the peak at AT was one person (genuine heavy use: consider the
  README's procedure) or many visitors at once (the one-bucket simplification; nothing to do). A
  peak on Zitadel (`/oauth/v2/authorize`, `/ui/v2/login/login`) in the same second as one on
  oauth2-proxy is usually a scanner following the login gate's redirects - oauth2-proxy answers
  unknown paths with a redirect to the login, not a 404 (2026-11-17: 181 probes, 181 x 2 at
  Zitadel); its log (`kubectl --context d3strukt0r-prod-admin -n oauth2-proxy logs deploy/oauth2-proxy`)
  shows the probed paths.
- **After raising a limit** whose sizing used a single session: run it again once more use has
  accumulated, and lower it towards 80 % if there is room (Grafana and the wedding site went
  100/500 -> 50/300 and 50/1000 -> 50/300 that way).
- Present the table as the script prints it, then one line per `refusing` or `tight` row with
  what it means.
