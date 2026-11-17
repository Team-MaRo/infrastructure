#!/usr/bin/env bash
# Read-only report of every version pin in this repo against upstream: changes nothing.
# Usage: inventory.sh [filter]     filter: substring of the file path or the image/chart name
set -euo pipefail

filter=${1:-}
ctx=d3strukt0r-prod-admin
cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
mkdir "$tmp/cache"
pending=$tmp/pending matched=$tmp/matched
: >"$pending"
: >"$matched"

k() { kubectl --context "$ctx" "$@"; }
files() { git ls-files -co --exclude-standard "$@"; }
want() { [ -z "$filter" ] || [[ $1 == *"$filter"* || $2 == *"$filter"* ]]; }
# One row to resolve: loc kind name current running src key. Empty fields are "-", or read
# would collapse the tabs.
add() {
  want "${1%%:*}" "$3" || return 0
  printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$1" "$2" "$3" "${4:--}" "${5:--}" "$6" "${7:--}" >>"$pending"
}
cache_file() { printf '%s/cache/%s' "$tmp" "$(printf '%s' "$1" | tr -c 'A-Za-z0-9._-' '_')"; }

# Image repository -> registry/path, Docker Hub spelled out.
canon() {
  local r=$1 host=${1%%/*}
  if [[ $r != */* || ( $host != *.* && $host != *:* && $host != localhost ) ]]; then
    host=docker.io
  else
    r=${r#*/}
  fi
  [[ $host == index.docker.io || $host == registry-1.docker.io ]] && host=docker.io
  [[ $host == docker.io && $r != */* ]] && r=library/$r
  printf '%s/%s\n' "$host" "$r"
}
# Image reference -> "repository<TAB>tag", tag "-" when there is none; a digest is dropped.
split_ref() {
  local ref=${1%%@*}
  if [[ ${ref##*/} == *:* ]]; then printf '%s\t%s\n' "${ref%:*}" "${ref##*:}"; else printf '%s\t-\n' "$ref"; fi
}

# Our own registry is never looked up; a tag without digits floats.
add_image() { # loc kind repo tag [name]
  local c src=img
  c=$(canon "$3")
  echo "$c" >>"$matched"
  if [[ $c == registry.d3strukt0r.dev/* ]]; then src=unrecognised
  elif [[ $4 == - ]]; then
    # A system-upgrade-controller Plan's image takes its tag from the Plan's channel.
    if grep -q '^kind: Plan' "${1%%:*}" 2>/dev/null && grep -q '^ *channel:' "${1%%:*}"; then
      src=floating
    else
      src=unrecognised
    fi
  elif [[ $4 != *[0-9]* ]]; then src=floating
  fi
  add "$1" "$2" "${5:-$c}" "$4" "$(running_tags "$c")" "$src" "$c"
}

pods_json=$(k get pods -A -o json 2>/dev/null) || { echo "warning: kubectl could not read the pods; RUNNING stays empty" >&2; pods_json='{"items":[]}'; }
apps_json=$(k -n argocd get applications -o json 2>/dev/null) || apps_json='{"items":[]}'
nodes_json=$(k get nodes -o json 2>/dev/null) || nodes_json='{"items":[]}'

jq -r '.items[] | .metadata.namespace as $ns | (.spec.containers + (.spec.initContainers // []))[] | [.image, $ns] | @tsv' <<<"$pods_json" |
  sort -u | while IFS=$'\t' read -r ref ns; do
    IFS=$'\t' read -r repo tag < <(split_ref "$ref")
    printf '%s\t%s\t%s\n' "$(canon "$repo")" "$tag" "$ns"
  done | sort -u >"$tmp/running"
running_tags() { awk -F'\t' -v r="$1" '$1 == r { print $2 }' "$tmp/running" | sort -uV | paste -sd, -; }
running_with_tag() { awk -F'\t' -v t="$1" '$2 == t { print $1 }' "$tmp/running" | sort -u; }

# 1. Chart ranges in Argo CD Applications.
for f in kubernetes/clusters/*/*.yaml; do
  yq -o tsv 'select(.kind == "Application") | .metadata.name as $app
    | (.spec.sources // [.spec.source]) | to_entries[] | select(.value.chart)
    | [$app, .key, .value.chart, .value.repoURL, .value.targetRevision, (.value.targetRevision | line)]' "$f" 2>/dev/null |
    while IFS=$'\t' read -r app idx chart repo rev line; do
      run=$(jq -r --arg n "$app" --argjson i "$idx" '.items[] | select(.metadata.name == $n)
        | .status.sync | (.revisions // [.revision])[$i] // empty
        | select(test("^[0-9a-f]{40}$") | not)' <<<"$apps_json")
      src=chart key="${repo%/}|$chart"
      if [[ $repo != *://* || $repo == oci://* ]]; then src=chartoci key="${repo#oci://}/$chart"; fi
      [[ $rev =~ ^~?v?[0-9]+(\.[0-9]+)*$ ]] || src=unrecognised
      add "$f:$line" chart "$chart" "$rev" "$run" "$src" "$key"
    done
done

# 2. Upstream manifests with the version in their URL.
while IFS=: read -r f line text; do
  if [[ $text =~ https://raw\.githubusercontent\.com/([^/]+)/([^/]+)/([^/]+)/ ||
    $text =~ https://github\.com/([^/]+)/([^/]+)/releases/download/([^/]+)/ ]]; then
    org=${BASH_REMATCH[1]} repo=${BASH_REMATCH[2]} tag=${BASH_REMATCH[3]}
    needle=$(tr -d '-' <<<"$repo" | tr '[:upper:]' '[:lower:]')
    imgs=$(awk -F'\t' -v n="$needle" '{ c = tolower($1); gsub(/-/, "", c); if (index(c, n)) print $1 }' "$tmp/running" | sort -u)
    [ -z "$imgs" ] || echo "$imgs" >>"$matched"
    run=$(for i in $imgs; do running_tags "$i"; done | tr ',' '\n' | sort -uV | paste -sd, -)
    add "$f:$line" manifest "$org/$repo" "$tag" "$run" gh "$org/$repo"
  fi
done < <(files 'kubernetes/*kustomization.yaml' | xargs grep -HnE 'https://(raw\.githubusercontent\.com|github\.com)/' || true)

# 3. Kustomize images:
: >"$tmp/kimages"
for f in $(files 'kubernetes/*kustomization.yaml'); do
  yq -o tsv '.images[]? | [.name, (.newName // "-"), (.newTag // "-"), ((.newTag // .name) | line)]' "$f" 2>/dev/null |
    while IFS=$'\t' read -r name newname tag line; do
      echo "$name" >>"$tmp/kimages"
      [[ $newname == - ]] && newname=$name
      add_image "$f:$line" kustomize-image "$newname" "$tag"
    done
done

# 4. image:/imageName: lines in our own manifests; templated lines are the chart's values' job.
while IFS= read -r hit; do
  f=${hit%%:*} rest=${hit#*:} line=${rest%%:*} text=${rest#*:}
  [[ $text == *'{{'* ]] && continue
  [[ $text =~ image(Name)?:[[:space:]]*[\"\']?([^\"\'[:space:]]+) ]] || continue
  ref=${BASH_REMATCH[2]}
  [[ $ref == \#* ]] && continue
  IFS=$'\t' read -r repo tag < <(split_ref "$ref")
  grep -qxF "$f $ref" "$tmp/seen" 2>/dev/null && continue
  echo "$f $ref" >>"$tmp/seen"
  if [[ $tag == - ]] && grep -qxF "$repo" "$tmp/kimages"; then
    canon "$repo" >>"$matched"
    continue
  fi
  add_image "$f:$line" image "$repo" "$tag"
done < <(files 'kubernetes/*.yaml' 'kubernetes/*.yml' | xargs grep -HnE '^[[:space:]]*(-[[:space:]]*)?image(Name)?:[[:space:]]*[^[:space:]#]' || true)

# 5. Image tags set in Helm values (image maps), also inside a HelmChartConfig's valuesContent.
# The repository comes from the map, else the chart's default at the same path, else the one
# running image carrying that tag.
chart_default() { # values file, dotted path
  local app hit
  hit=$(grep -lF "\$repo/$1" kubernetes/clusters/*/*.yaml 2>/dev/null | head -1) || true
  [ -n "$hit" ] || return 0
  app=$(F="\$repo/$1" yq -o tsv '(.spec.sources // [.spec.source])[] | select(.chart and (.helm.valueFiles // [] | contains([strenv(F)])))
    | [.repoURL, .chart, .targetRevision]' "$hit" | head -1)
  [ -n "$app" ] || return 0
  local repo chart rev out
  IFS=$'\t' read -r repo chart rev <<<"$app"
  out=$(cache_file "values $repo $chart $rev")
  [ -f "$out" ] || helm show values --repo "$repo" "$chart" --version "$rev" >"$out" 2>/dev/null || : >"$out"
  P=.$2 yq -p yaml -o tsv 'eval(strenv(P)) | [(.registry // ""), (.repository // "")] | join("/")' "$out" 2>/dev/null | sed 's|^/||' || true
}
image_maps() { # yaml on stdin -> dotted path, tag, registry, repository, line
  yq -o tsv '.. | select(tag == "!!map" and has("tag") and (.tag | tag) != "!!map" and (.tag | tag) != "!!null")
    | select((path | .[-1] // "" | test("(?i)image$")) or has("repository"))
    | [(path | join(".")), .tag, (.registry // "-"), (.repository // "-"), (.tag | line)]' - 2>/dev/null || true
}
override_rows() { # file, line offset; yaml on stdin
  image_maps | while IFS=$'\t' read -r path tag reg repo line; do
    local full=
    if [[ $repo != - ]]; then
      full=$repo
      [[ $reg != - ]] && full=$reg/$repo
    else
      full=$(chart_default "$1" "$path")
    fi
    if [[ -z $full || $full == */ ]]; then
      full=$(running_with_tag "$tag")
      [[ $(wc -l <<<"$full") -eq 1 ]] || full=
    fi
    loc="$1:$((line + $2))"
    name=$path
    if [ -z "$full" ]; then add "$loc" override "$name" "$tag" "" unrecognised; continue; fi
    add_image "$loc" override "$full" "$tag" "$(canon "$full")"
  done
}
for f in $(files 'kubernetes/*.yaml' | grep -v '/templates/' | grep -v 'kustomization.yaml$'); do
  override_rows "$f" 0 <"$f"
  vc=$(yq 'select(.kind == "HelmChartConfig") | .spec.valuesContent | line' "$f" 2>/dev/null) || true
  [[ $vc =~ ^[0-9]+$ ]] && yq 'select(.kind == "HelmChartConfig") | .spec.valuesContent' "$f" | override_rows "$f" "$vc"
done

# 6. Other pins: k3s, go run module@version, docker run in scripts, GitHub Actions.
k3s_running=$(jq -r '[.items[].status.nodeInfo.kubeletVersion] | unique | join(",")' <<<"$nodes_json")
while IFS=: read -r f line text; do
  v=$(sed -E 's/.*k3s_version:[[:space:]]*["'\'']?([^"'\''[:space:]]+).*/\1/' <<<"$text")
  add "$f:$line" k3s k3s-io/k3s "$v" "$k3s_running" gh k3s-io/k3s
done < <(files 'ansible/*' | xargs grep -HnE '^[[:space:]]*k3s_version:' || true)

while IFS=: read -r f line text; do
  [[ $text =~ go\ run\ ([^[:space:]@]+)@([^[:space:]]+) ]] || continue
  mod=${BASH_REMATCH[1]} v=${BASH_REMATCH[2]}
  if [[ $mod =~ ^github\.com/([^/]+)/([^/]+) ]]; then
    add "$f:$line" go-module "$mod" "$v" "" gh "${BASH_REMATCH[1]}/${BASH_REMATCH[2]}"
  else
    add "$f:$line" go-module "$mod" "$v" "" unrecognised
  fi
done < <(files | xargs grep -HnE 'go run [[:alnum:]._-]+\.[[:alpha:]]+/[^[:space:]@]+@v[0-9]' 2>/dev/null || true)

# docker run's image is the first argument that is neither an option nor an option's value.
for f in $(files 'kubernetes/*.sh'); do
  awk '/\\$/ { buf = buf substr($0, 1, length($0) - 1) " "; if (!start) start = NR; next }
    { print (start ? start : NR) "\t" buf $0; buf = ""; start = 0 }' "$f" |
    while IFS=$'\t' read -r line cmd; do
      [[ $cmd == *'docker run'* || $cmd == *'podman run'* ]] || continue
      read -ra words <<<"${cmd#*run }"
      skip=0 ref=
      for w in "${words[@]}"; do
        if ((skip)); then skip=0; continue; fi
        case $w in
          --*=*) ;;
          -v | -p | -e | -w | -u | -l | --name | --network | --network-alias | --entrypoint | --mount | --env | --user | --platform | --workdir | --volume | --publish | --label) skip=1 ;;
          -*) ;;
          *) ref=${w//[\"\']/}; break ;;
        esac
      done
      [[ -n $ref && $ref != *'$'* ]] || continue
      n=$(awk -v s="$line" -v r="$ref" 'NR >= s && index($0, r) { print NR; exit }' "$f")
      IFS=$'\t' read -r repo tag < <(split_ref "$ref")
      add_image "$f:${n:-$line}" docker-run "$repo" "$tag"
    done
done

while IFS=: read -r f line text; do
  [[ $text =~ uses:[[:space:]]*[\"\']?([^/@[:space:]]+)/([^/@[:space:]]+)[^@[:space:]]*@([^[:space:]\"\']+) ]] || continue
  org=${BASH_REMATCH[1]} repo=${BASH_REMATCH[2]} v=${BASH_REMATCH[3]}
  src=gh
  [[ $v =~ ^v?[0-9]+$ ]] && src=ghmajor
  [[ $v =~ ^[0-9a-f]{40}$ ]] && src=unrecognised
  add "$f:$line" action "$org/$repo" "$v" "" "$src" "$org/$repo"
done < <(files '.github/workflows/*' | xargs grep -HnE '^[[:space:]]*-?[[:space:]]*uses:[[:space:]]*[^.]' || true)

# 7. Images only a chart or an operator sets.
sort -u "$matched" -o "$matched"
cut -f1 "$tmp/running" | sort -u | comm -23 - "$matched" | while read -r c; do
  ns=$(awk -F'\t' -v r="$c" '$1 == r { print $3 }' "$tmp/running" | sort -u | paste -sd, -)
  tags=$(running_tags "$c")
  first=${tags%%,*} src=img
  [[ $first == - ]] && src=unrecognised
  [[ $first != *[0-9]* ]] && src=floating
  add "cluster/$ns" running-only "$c" "$first" "$tags" "$src" "$c"
done

# Fetch every tag list, release list and chart index once, eight at a time.
fetch_registry() { # registry/path -> tags
  local host=${1%%/*} path=${1#*/} url www realm service scope token final h auth=()
  [[ $host == docker.io ]] && host=registry-1.docker.io
  url="https://$host/v2/$path/tags/list?n=1000"
  www=$(curl -sS -o /dev/null -D - "$url" | tr -d '\r' | grep -i '^www-authenticate:') || true
  if [ -n "$www" ]; then
    realm=$(sed -nE 's/.*realm="([^"]*)".*/\1/p' <<<"$www")
    service=$(sed -nE 's/.*service="([^"]*)".*/\1/p' <<<"$www")
    scope=$(sed -nE 's/.*scope="([^"]*)".*/\1/p' <<<"$www")
    token=$(curl -fsS -G "$realm" --data-urlencode "service=$service" \
      --data-urlencode "scope=${scope:-repository:$path:pull}" | jq -r '.token // .access_token')
    auth=(-H "Authorization: Bearer $token")
  fi
  h=$(mktemp "$tmp/h.XXXXXX")
  while [ -n "$url" ]; do
    final=$(curl -fsSL "${auth[@]}" -D "$h" -o "$h.body" -w '%{url_effective}' "$url") || return 1
    jq -r '.tags[]?' "$h.body" || return 1
    url=$(tr -d '\r' <"$h" | sed -nE 's/^[Ll]ink: *<([^>]*)>.*rel="?next.*/\1/p' | tail -1)
    [[ -z $url || $url == http* ]] || url=$(sed -E 's|^(https://[^/]+).*|\1|' <<<"$final")$url
  done
}
fetch() { # src key
  local out
  out=$(cache_file "$1 $2")
  {
    case $1 in
      img | chartoci) fetch_registry "$2" ;;
      gh | ghmajor) gh release list -R "$2" --exclude-pre-releases -L 50 --json tagName -q '.[].tagName' ;;
      chart) curl -fsSL "$2/index.yaml" ;;
    esac
  } >"$out.part" 2>/dev/null && mv "$out.part" "$out" || rm -f "$out.part"
}
while IFS=$'\t' read -r src key; do
  while (($(jobs -rp | wc -l) >= 8)); do wait -n || true; done
  fetch "$src" "$key" &
done < <(awk -F'\t' '$6 ~ /^(img|chartoci|gh|ghmajor)$/ { print $6 "\t" $7 } $6 == "chart" { split($7, a, "|"); print "chart\t" a[1] }' "$pending" | sort -u)
wait

# Newest tag of the same shape as cur: the same prefix, number of version parts and suffix, a
# build stamp (8+ digits) compared after the version. mode: image (compare cur), range (compare
# the newest on cur's line), major (only the major counts).
pick='
function parse(t, P,   v, n, c, r) {
  delete P; P["ok"] = 0
  if (!match(t, /[0-9]/)) return
  P["pre"] = substr(t, 1, RSTART - 1); t = substr(t, RSTART)
  match(t, /^[0-9]+(\.[0-9]+)*/); v = substr(t, 1, RLENGTH); r = substr(t, RLENGTH + 1)
  n = split(v, c, ".")
  P["n"] = n; P["c1"] = c[1] + 0; P["c2"] = c[2] + 0; P["c3"] = c[3] + 0; P["c4"] = c[4] + 0
  P["stamp"] = 0
  if (match(r, /[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]+/)) P["stamp"] = substr(r, RSTART, RLENGTH) + 0
  gsub(/[0-9][0-9][0-9][0-9][0-9][0-9][0-9][0-9]+/, "#", r)
  if (mode != "image") { n = ""; if (mode == "major") r = "" }
  P["shape"] = P["pre"] "|" n "|" r
  P["ok"] = 1
}
function cmp(A, B,   i) {
  for (i = 1; i <= 4; i++) if (A["c" i] != B["c" i]) return A["c" i] < B["c" i] ? -1 : 1
  if (A["stamp"] != B["stamp"]) return A["stamp"] < B["stamp"] ? -1 : 1
  return 0
}
function copy(A, B,   k) { delete B; for (k in A) B[k] = A[k] }
BEGIN { parse(cur, C) }
C["ok"] {
  parse($1, T)
  if (!T["ok"] || T["shape"] != C["shape"]) next
  if (best == "" || cmp(T, N) > 0) { copy(T, N); best = $1 }
  same = T["c1"] == C["c1"] && (mode == "major" || C["n"] < 2 || T["c2"] == C["c2"])
  if (same && (sbest == "" || cmp(T, S) > 0)) { copy(T, S); sbest = $1 }
}
END {
  if (!C["ok"]) { print "-\t-\tfloating"; exit }
  if (best == "" || (mode == "range" && sbest == "")) { print (sbest == "" ? "-" : sbest) "\t" (best == "" ? "-" : best) "\tunrecognised"; exit }
  if (mode == "range") copy(S, C)
  flag = "ok"
  if (mode == "image" && C["n"] < 3 && !C["stamp"]) flag = "floating"
  else if (cmp(C, N) < 0) {
    if (C["c1"] != N["c1"]) flag = "behind-major"
    else if (mode == "major") flag = "ok"
    else if (C["c2"] != N["c2"]) flag = "behind-minor"
    else flag = "behind-patch"
  }
  print (sbest == "" ? "-" : sbest) "\t" best "\t" flag
}'

{
  printf 'LOCATION\tKIND\tNAME\tCURRENT\tRUNNING\tNEWEST-SAME-LINE\tNEWEST\tFLAG\n'
  while IFS=$'\t' read -r loc kind name cur run src key; do
    same=- newest=- flag=$src
    case $src in
      img | chartoci | gh | ghmajor | chart)
        mode=image
        [[ $src == ghmajor ]] && mode=major
        [[ $cur == \~* ]] && mode=range
        if [[ $src == chart ]]; then
          idx=$(cache_file "chart ${key%|*}")
          list=$(cache_file "versions $key")
          [ -f "$idx" ] && C=${key#*|} yq -p yaml -o tsv '.entries[strenv(C)][] | [.version, (.appVersion // "-")]' "$idx" >"$list" 2>/dev/null || true
        else
          list=$(cache_file "$src $key")
        fi
        if [ -s "$list" ]; then
          IFS=$'\t' read -r same newest flag < <(cut -f1 "$list" | awk -v cur="${cur#\~}" -v mode="$mode" "$pick")
          if [[ $src == chart && $same != - ]]; then
            app=$(awk -F'\t' -v v="$same" '$1 == v { print $2; exit }' "$list")
            cur="$cur(app:${app:--})"
          fi
        else
          flag=lookup-failed
        fi
        [[ $kind == override && $flag != unrecognised && $flag != lookup-failed ]] && flag=override
        ;;
    esac
    [[ $kind == running-only ]] && cur=-
    name=${name#docker.io/library/} name=${name#docker.io/}
    printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' "$loc" "$kind" "$name" "$cur" "$run" "$same" "$newest" "$flag"
  done <"$pending"
} | column -t -s $'\t'
