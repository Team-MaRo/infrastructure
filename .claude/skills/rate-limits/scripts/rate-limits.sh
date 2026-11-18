#!/usr/bin/env bash
# Read-only report: how close each namespace's cloudflare-only rate limit comes to refusing its
# traffic. Changes nothing.
# Usage: rate-limits.sh [namespace] [--since 7d]     --since: <n>d or <n>h, at most Tempo's 30 days
set -euo pipefail

ns_filter= since=7d
while [ $# -gt 0 ]; do
  case $1 in
    --since) since=$2; shift 2 ;;
    --since=*) since=${1#*=}; shift ;;
    -h|--help) sed -n '2,4p' "$0"; exit 0 ;;
    *) ns_filter=$1; shift ;;
  esac
done
case $since in
  *d) window=$(( ${since%d} * 86400 )) ;;
  *h) window=$(( ${since%h} * 3600 )) ;;
  *) echo "--since takes <n>d or <n>h" >&2; exit 2 ;;
esac

ctx=d3strukt0r-prod-admin
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
end=$(date +%s)
start=$(( end - window ))
tempo=/api/v1/namespaces/tempo/services/http:tempo:3200/proxy/api/search
loki=/api/v1/namespaces/loki/services/http:loki:3100/proxy/loki/api/v1/query

k() { kubectl --context "$ctx" "$@"; }
uri() { jq -rn --arg v "$1" '$v|@uri'; }

# Each namespace's copy of the limit: "namespace average burst changed", changed = when its
# numbers were last set (epoch seconds).
k get middlewares.traefik.io -A -o json --show-managed-fields \
  | jq -r '.items[] | select(.metadata.name == "cloudflare-rate-limit")
      | .metadata.creationTimestamp as $created
      | "\(.metadata.namespace) \(.spec.rateLimit.average) \(.spec.rateLimit.burst) \([.metadata.managedFields[]?.time // empty] | max // $created | fromdateiso8601)"' \
  | { if [ -n "$ns_filter" ]; then awk -v n="$ns_filter" '$1 == n'; else cat; fi; } >"$tmp/limits"
if [ ! -s "$tmp/limits" ]; then
  echo "no cloudflare-rate-limit Middleware${ns_filter:+ in namespace $ns_filter}" >&2
  exit 1
fi

# The hosts each namespace routes: "namespace host", from Ingresses and IngressRoutes.
{
  k get ingresses -A -o json | jq -r '.items[] | .metadata.namespace as $n | .spec.rules[]?.host // empty | "\($n) \(.)"'
  k get ingressroutes.traefik.io -A -o json \
    | jq -r '.items[] | .metadata.namespace as $n | .spec.routes[].match | [scan("Host\\(`([^`]+)`\\)")[0]][] | "\($n) \(.)"'
} | sort -u >"$tmp/hosts"

# Every request to a namespace's hosts in [from, to) the app answered (2xx, 3xx, 5xx), from
# Traefik's spans in Tempo: "start time (ns) path" per line. Left out: refusals (429, answered at
# once, so replaying them shows bursts that never reached the app - they are counted from the log
# instead) and other 4xx, nearly all scanners' probes (404) and requests without a login (401,
# 403). Probes behind a login gate get a redirect instead; replay.py drops those by their path - a
# regex in the query itself made Tempo several times slower. A search returns at most 5000, so a
# full window is halved until it fits.
fetch() {
  local q=$1 from=$2 to=$3 out=$4 got
  got=$(k get --raw "$tempo?q=$(uri "$q | select(span.url.path)")&start=$from&end=$to&limit=5000&spss=1" \
    | jq -r '.traces[]? | "\(.startTimeUnixNano) \([.spanSet.spans[0].attributes[]? | select(.key == "url.path") | .value.stringValue][0] // "-")"')
  if [ "$(printf '%s' "$got" | grep -c .)" -ge 5000 ] && [ $(( to - from )) -gt 60 ]; then
    local mid=$(( (from + to) / 2 ))
    fetch "$q" "$from" "$mid" "$out"
    fetch "$q" "$mid" "$to" "$out"
  elif [ -n "$got" ]; then
    printf '%s\n' "$got" >>"$out"
  fi
}

# Traefik's 429s from its access log in Loki: "epoch host visitor path" per line, fetched in 6 h
# steps (Loki refuses longer ranges here), each halved while it hits the 5000-line limit.
fetch429() {
  local from=$1 to=$2 got q='{namespace="kube-system", container="traefik"} |= "\"DownstreamStatus\":429"'
  got=$(k get --raw "${loki}_range?query=$(uri "$q")&start=${from}000000000&end=${to}000000000&limit=5000" \
    | jq -r '.data.result[].values[] | (.[0][:10]) as $t | .[1] | fromjson?
        | "\($t) \(.RequestHost) \(.["request_Cf-Connecting-Ip"] // "-") \(.RequestPath)"')
  if [ "$(printf '%s' "$got" | grep -c .)" -ge 5000 ] && [ $(( to - from )) -gt 60 ]; then
    local mid=$(( (from + to) / 2 ))
    fetch429 "$from" "$mid"
    fetch429 "$mid" "$to"
  elif [ -n "$got" ]; then
    printf '%s\n' "$got"
  fi
}
for (( t = start; t < end; t += 21600 )); do
  fetch429 "$t" $(( t + 21600 > end ? end : t + 21600 ))
done >"$tmp/429"

# One namespace's row: its requests from Tempo, replayed with its 429s.
row() {
  local ns=$1 average=$2 burst=$3 changed=$4 hosts cond from to
  hosts=$(awk -v n="$ns" '$1 == n {print $2}' "$tmp/hosts")
  [ -n "$hosts" ] || return 0
  cond=$(printf '%s\n' "$hosts" | awk '{printf "%sspan.server.address = \"%s\"", (NR > 1 ? " || " : ""), $1}')
  : >"$tmp/ts-$ns"
  for (( from = start; from < end; from += 43200 )); do
    to=$(( from + 43200 > end ? end : from + 43200 ))
    fetch "{ resource.service.name = \"traefik\" && ($cond) && (span.http.response.status_code < 400 || span.http.response.status_code >= 500) }" "$from" "$to" "$tmp/ts-$ns"
  done
  sort -n -o "$tmp/ts-$ns" "$tmp/ts-$ns"
  python3 -I "$here/replay.py" "$tmp/ts-$ns" "$tmp/429" "$here/probe-paths.re" "$ns" "$average" \
    "$burst" "$changed" "$(printf '%s' "$hosts" | paste -sd, -)" >"$tmp/row-$ns"
}

# Four namespaces at a time: nearly all the time is spent waiting for Tempo.
while read -r ns average burst changed; do
  while [ "$(jobs -rp | wc -l)" -ge 4 ]; do wait -n; done
  row "$ns" "$average" "$burst" "$changed" &
done <"$tmp/limits"
wait
cat "$tmp"/row-* 2>/dev/null >"$tmp/rows" || : >"$tmp/rows"

echo "Window: $(date -d "@$start" '+%Y-%m-%d %H:%M') to $(date -d "@$end" '+%Y-%m-%d %H:%M') ($since)"
{
  printf 'NAMESPACE\tLIMIT\tSET\tREQUESTS\tPEAK/S\tBUCKET-USED\tAT\tWOULD-REFUSE\t429-GENUINE\t429-SCANNER\tSTATUS\tHOSTS\n'
  sort -t$'\t' -k6,6nr "$tmp/rows"
} | column -t -s$'\t'
