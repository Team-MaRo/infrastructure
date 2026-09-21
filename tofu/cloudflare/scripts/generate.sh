#!/usr/bin/env bash
# Regenerates HCL and import blocks for every zone in both Cloudflare accounts, into
# tofu/cloudflare/generated/. That directory is gitignored and is NOT part of the root
# module - OpenTofu only parses .tf files in the module directory itself, not in
# subdirectories - so generated output can never accidentally take effect.
#
# The output is raw material to be refactored into the module by hand, not something
# to use as-is. cf-terraforming names every resource terraform_managed_resource_<id>
# and omits the provider attribute, which this module requires because it declares two
# aliased providers and no default.
#
# Needs: cf-terraforming, tofu, and both tokens exported.
set -euo pipefail

here=$(cd "$(dirname "$0")/.." && pwd)
out="$here/generated"
tofu_bin=$(command -v tofu)

# Kept in sync with settings.tf. Six security and TLS settings, out of the ~60 in the
# Cloudflare catalogue.
settings="ssl,always_use_https,min_tls_version,automatic_https_rewrites,tls_1_3,security_level"

command -v cf-terraforming >/dev/null || { echo "cf-terraforming not installed: brew install cf-terraforming" >&2; exit 1; }
[ -d "$here/.terraform" ] || { echo "run 'tofu init' in $here first" >&2; exit 1; }

rm -rf "$out" && mkdir -p "$out"

for account in personal arepazo; do
  var="TF_VAR_cloudflare_token_${account}"
  token="${!var:-}"
  [ -n "$token" ] || { echo "$var is not set" >&2; exit 1; }
  export CLOUDFLARE_API_TOKEN="$token"

  zones=$(curl -sS -H "Authorization: Bearer $token" \
    "https://api.cloudflare.com/client/v4/zones?per_page=50" |
    python3 -c 'import json,sys
d = json.load(sys.stdin)
if not d.get("success"):
    sys.exit("zone listing failed: %s" % d.get("errors"))
for z in d["result"]:
    print(z["id"], z["name"])')

  echo "== account $account"
  while read -r zone_id zone_name; do
    [ -n "$zone_id" ] || continue
    slug=${zone_name//./_}
    slug=${slug//-/_}
    echo "   $zone_name ($zone_id)"

    # Record which account and zone id each zone has, so the hand-written zone and
    # zone_setting import blocks can be built from it afterwards.
    printf '%s\t%s\t%s\n' "$account" "$zone_id" "$zone_name" >> "$out/zones.tsv"

    for rt in cloudflare_dns_record cloudflare_zone cloudflare_zone_setting; do
      extra=()
      [ "$rt" = cloudflare_zone_setting ] && extra=(--resource-id "cloudflare_zone_setting=$settings")

      cf-terraforming generate \
        --zone "$zone_id" \
        --resource-type "$rt" \
        --terraform-binary-path "$tofu_bin" \
        --terraform-install-path "$here" \
        "${extra[@]}" \
        > "$out/${slug}.${rt}.tf" 2> "$out/${slug}.${rt}.generate.log" || true

      # Only cloudflare_dns_record is listed as import-capable for the v5 provider.
      if [ "$rt" = cloudflare_dns_record ]; then
        cf-terraforming import \
          --zone "$zone_id" \
          --resource-type "$rt" \
          --modern-import-block \
          --terraform-binary-path "$tofu_bin" \
          --terraform-install-path "$here" \
          > "$out/${slug}.${rt}.imports.tf" 2> "$out/${slug}.${rt}.import.log" || true
      fi
    done
  done <<< "$zones"
done

echo
echo "written to $out"
echo "  $(find "$out" -name '*.tf' | wc -l | tr -d ' ') hcl files, $(grep -hc '^resource' "$out"/*.tf 2>/dev/null | paste -sd+ - | bc 2>/dev/null || echo '?') resources"
echo "check the .log files for anything cf-terraforming could not generate."
