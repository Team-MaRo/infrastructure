#!/usr/bin/env bash
# Reports whether a domain has a DS record in its parent zone, which is what actually
# activates DNSSEC. Used as the program behind a terraform external data source, so it
# reads {"domain": "..."} on stdin and every emitted value has to be a string.
#
# dig exits 0 even for NXDOMAIN, and `dig +short DS` prints nothing both for a domain
# without DNSSEC and for a domain that does not exist. The rcode is therefore the only
# thing that tells those two apart, and a missing domain has to be an error rather than
# a quiet "no DNSSEC".
set -euo pipefail

domain=$(grep -o '"domain"[[:space:]]*:[[:space:]]*"[^"]*"' | sed 's/.*"\([^"]*\)"$/\1/' || true)

if [ -z "$domain" ]; then
  echo "ds-lookup: no domain given on stdin" >&2
  exit 1
fi

out=$(dig +time=5 +tries=2 +noall +comments +answer DS "$domain" 2>/dev/null || true)
status=$(printf '%s' "$out" | sed -n 's/.*status: \([A-Z]*\),.*/\1/p' | head -1)

case "$status" in
  NOERROR) ;;
  "")        echo "ds-lookup: no response from the resolver for $domain" >&2; exit 1 ;;
  NXDOMAIN)  echo "ds-lookup: $domain does not exist (NXDOMAIN) - lapsed or misspelled?" >&2; exit 1 ;;
  *)         echo "ds-lookup: DS lookup for $domain returned $status" >&2; exit 1 ;;
esac

if printf '%s\n' "$out" | grep -qE '[[:space:]]IN[[:space:]]+DS[[:space:]]'; then
  printf '{"has_ds":"true"}\n'
else
  printf '{"has_ds":"false"}\n'
fi
