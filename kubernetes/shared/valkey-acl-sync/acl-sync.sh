#!/bin/sh
# Builds Valkey's aclfile from the users Secrets, keeping out every line Valkey refuses, so one
# app's broken line can never stop Valkey from starting (README):
#   acl-sync.sh init - initContainer: makes acl-sync's own login and the first aclfile, checking
#                      each line against a short-lived Valkey on a socket.
#   acl-sync.sh run  - sidecar: rebuilds the aclfile when the Secrets change and loads it into the
#                      running Valkey with ACL LOAD (all or nothing).
# No `set -e` for the sidecar: a failed round must never end it - its pod would drop out of the
# Service. init sets it for itself.
set -u
# The rules hold patterns like ~* and %R~*, which must reach Valkey unexpanded.
set -f

MODE="${1:-run}"
USERS_DIR=/etc/valkey/users
LOCAL_DIR=/run/valkey-local
ACLFILE="$LOCAL_DIR/users.acl"
STATE_DIR="$LOCAL_DIR/state"
SOCKET="$LOCAL_DIR/check.sock"
INTERVAL="${ACL_SYNC_INTERVAL:-15}"
# A user whose Secret disappears is kept this long: an ExternalSecret that Argo CD prunes and
# recreates must not cut its app off.
REMOVAL_GRACE="${ACL_SYNC_REMOVAL_GRACE:-300}"
# A scratch user each line is tried on before it goes into the aclfile; always switched off, so
# it can never be logged in to.
CHECK_USER=acl-sync-check

log() {
  echo "acl-sync: $*"
}

# Counted by Alloy for the Valkeys' UsersBroken alerts (README).
fail() {
  echo "acl-sync: failed: $*"
  echo "acl-sync: failed: $*" >> "$STATE_DIR/failures.new"
}

# A Valkey that does not answer must not hang the loop. Replies on one line, so each log line
# stays whole.
cli() {
  if [ "$MODE" = init ]; then
    timeout 10 valkey-cli -s "$SOCKET" "$@" 2>&1 | tr '\n' ' ' | sed 's/ *$//'
  else
    timeout 10 valkey-cli --user acl-sync "$@" 2>&1 | tr '\n' ' ' | sed 's/ *$//'
  fi
}

# Sorts a Valkey reply into ok (OK or an integer), denied (acl-sync's own login or rights),
# refused (Valkey took the command and said no) and down (Valkey not answering, busy, gone).
classify() {
  case "$1" in
    OK | [0-9]*) echo ok ;;
    NOAUTH* | "AUTH failed"* | *WRONGPASS* | NOPERM*) echo denied ;;
    "ERR max number of clients"* | LOADING*) echo down ;;
    ERR*) echo refused ;;
    *) echo down ;;
  esac
}

# Tries the rules on the scratch user: prints ok, or <kind>:<Valkey's reply>.
check() {
  reply="$(cli ACL SETUSER "$CHECK_USER" reset "$@" off 2>&1)"
  kind="$(classify "$reply")"
  if [ "$kind" != ok ]; then
    echo "$kind:$reply"
    return
  fi
  reply="$(cli ACL DELUSER "$CHECK_USER" 2>&1)"
  kind="$(classify "$reply")"
  if [ "$kind" = ok ]; then
    echo ok
  else
    echo "down:could not delete $CHECK_USER ($reply)"
  fi
}

checksum() {
  set +f
  cat "$USERS_DIR"/*.conf 2> /dev/null | sha256sum
  set -f
}

previous_line() {
  if [ -f "$ACLFILE" ]; then
    awk -v name="$1" '$1 == "user" && $2 == name' "$ACLFILE"
  fi
}

# Writes the new aclfile to $ACLFILE.new; sets `pending` (a removed user still in its grace) and
# `expired` (removed users whose grace is over, to forget once loaded). Returns 1 when Valkey
# could not be asked, 2 when the result must not be applied.
build() {
  : > "$STATE_DIR/failures.new"
  cat "$LOCAL_DIR/acl-sync.line" > "$ACLFILE.new"
  seen=" acl-sync $CHECK_USER "
  own=""
  pending=0
  expired=""
  # users.conf first: Valkey's own users (default, exporter) are defined there, and an app's file
  # must never replace one of them.
  files="$USERS_DIR/users.conf"
  set +f
  for file in "$USERS_DIR"/*.conf; do
    if [ -f "$file" ] && [ "$file" != "$USERS_DIR/users.conf" ]; then
      files="$files $file"
    fi
  done
  set -f
  for file in $files; do
    # Read first: the kubelet may remove a file between listing and reading it, and a failed
    # redirection would end the script.
    content="$(cat "$file" 2> /dev/null)" || continue
    while IFS= read -r line; do
      line="$(printf '%s' "$line" | tr -d '\r')"
      # shellcheck disable=SC2086
      set -- $line
      [ "${1:-}" = user ] || continue
      if [ $# -lt 2 ]; then
        fail "$file has a user line without a name"
        continue
      fi
      name="$2"
      shift 2
      case "$seen" in
        *" $name "*)
          case "$own $CHECK_USER acl-sync " in
            *" $name "*) fail "user $name in $file would replace one of Valkey's own users; refused" ;;
            *) fail "user $name in $file is defined a second time; only the first counts" ;;
          esac
          continue
          ;;
      esac
      seen="$seen$name "
      [ "$file" = "$USERS_DIR/users.conf" ] && own="$own $name "
      rm -f "$STATE_DIR/missing.$name"
      # Valkey's own parser takes quotes and escapes; ours splits on spaces only.
      case "$line" in
        *\"* | *\'* | *\\*)
          fail "user $name in $file uses quotes or backslashes, which acl-sync does not support"
          previous_line "$name" >> "$ACLFILE.new"
          continue
          ;;
      esac
      result="$(check "$@")"
      case "$result" in
        ok) printf 'user %s %s\n' "$name" "$*" >> "$ACLFILE.new" ;;
        refused:*)
          fail "user $name in $file was refused (${result#refused:}); it keeps its rules from before until the pod is recreated, a new one has none"
          previous_line "$name" >> "$ACLFILE.new"
          ;;
        denied:*)
          fail "acl-sync cannot log in to Valkey or lacks a right (${result#denied:})"
          return 1
          ;;
        *)
          log "Valkey not reachable (${result#down:}); trying again"
          return 1
          ;;
      esac
    done << EOF
$content
EOF
  done
  # An aclfile without default makes Valkey create it open - no password, every command - so a
  # checked default line from users.conf is required.
  if ! grep -q '^user default ' "$ACLFILE.new"; then
    fail "no usable user default in $USERS_DIR/users.conf - nothing applied, Valkey would leave default open"
    return 2
  fi
  [ -f "$ACLFILE" ] || return 0
  now="$(date +%s)"
  # User names are single words.
  # shellcheck disable=SC2013
  for name in $(awk '$1 == "user" { print $2 }' "$ACLFILE"); do
    case "$seen" in *" $name "*) continue ;; esac
    stamp="$STATE_DIR/missing.$name"
    since="$(cat "$stamp" 2> /dev/null)"
    case "$since" in
      '' | *[!0-9]*)
        since="$now"
        echo "$now" > "$stamp" || true
        ;;
    esac
    if [ $((now - since)) -lt "$REMOVAL_GRACE" ]; then
      previous_line "$name" >> "$ACLFILE.new"
      pending=1
    else
      expired="$expired $name"
    fi
  done
  return 0
}

init() {
  # Here any failure must stop: nothing runs yet, and a failed initContainer is retried.
  # pipefail is POSIX since 2024; the image's dash (0.5.12) has it.
  # shellcheck disable=SC3040
  set -e -o pipefail
  umask 077
  mkdir -p "$STATE_DIR"
  password="$(head -c 32 /dev/urandom | od -An -tx1 | tr -d ' \n')"
  [ -n "$password" ]
  printf '%s' "$password" > "$LOCAL_DIR/acl-sync.password"
  hash="$(printf '%s' "$password" | sha256sum | cut -d' ' -f1)"
  # What acl-sync runs: try and delete users, load the aclfile. acl|setuser alone lets it give
  # itself anything - as good as admin rights, but only for whoever is inside the pod.
  printf 'user acl-sync on #%s resetchannels -@all +acl|setuser +acl|deluser +acl|load\n' "$hash" \
    > "$LOCAL_DIR/acl-sync.line"

  valkey-server --port 0 --unixsocket "$SOCKET" --unixsocketperm 700 --dir "$LOCAL_DIR" \
    --save "" --appendonly no --daemonize no > /dev/null 2>&1 &
  server=$!
  for _ in $(seq 1 50); do
    if [ "$(cli PING 2> /dev/null)" = PONG ]; then
      break
    fi
    sleep 0.2
  done
  # build's result decides below; the short-lived Valkey must be stopped first either way.
  status=0
  build || status=$?
  cli SHUTDOWN NOSAVE > /dev/null 2>&1 || true
  kill "$server" 2> /dev/null || true
  wait "$server" || true
  rm -f "$SOCKET"
  # Without a checking Valkey, a usable default or acl-sync's rights the pod must not start; it
  # is retried.
  [ "$status" -eq 0 ] || exit 1
  mv "$ACLFILE.new" "$ACLFILE"
  log "first aclfile written, $(grep -c '^user ' "$ACLFILE") users"
}

run() {
  trap 'exit 0' TERM INT
  VALKEYCLI_AUTH="$(cat "$LOCAL_DIR/acl-sync.password")"
  export VALKEYCLI_AUTH
  last=""
  pending=0
  while true; do
    current="$(checksum)"
    if [ "$current" != "$last" ] || [ "$pending" -eq 1 ]; then
      status=0
      build || status=$?
      # A failed round, or files that changed while being read, are built again next round.
      if [ "$status" -ne 0 ] || [ "$(checksum)" != "$current" ]; then
        last=""
      elif ! cmp -s "$ACLFILE.new" "$ACLFILE"; then
        cp "$ACLFILE" "$ACLFILE.old"
        mv "$ACLFILE.new" "$ACLFILE"
        reply="$(cli ACL LOAD 2>&1)"
        case "$(classify "$reply")" in
          ok)
            log "loaded $(grep -c '^user ' "$ACLFILE") users"
            for name in $expired; do
              rm -f "$STATE_DIR/missing.$name"
            done
            last="$current"
            ;;
          down)
            mv "$ACLFILE.old" "$ACLFILE"
            log "Valkey not reachable for ACL LOAD ($reply); trying again"
            last=""
            ;;
          *)
            mv "$ACLFILE.old" "$ACLFILE"
            fail "ACL LOAD refused the aclfile ($reply); the previous one stays"
            last=""
            ;;
        esac
      else
        if [ -z "$last" ]; then
          log "checked $(grep -c '^user ' "$ACLFILE") users, nothing to change"
        fi
        last="$current"
      fi
      mv "$STATE_DIR/failures.new" "$STATE_DIR/failures" 2> /dev/null
    elif [ -s "$STATE_DIR/failures" ]; then
      # Nothing changed, but a line is still refused: repeat it, so its alert stays on.
      cat "$STATE_DIR/failures"
    fi
    sleep "$INTERVAL" &
    wait $!
  done
}

case "$MODE" in
  init) init ;;
  run) run ;;
  *)
    echo "usage: acl-sync.sh init|run" >&2
    exit 2
    ;;
esac
