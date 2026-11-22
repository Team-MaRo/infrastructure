#!/bin/sh
# Writes the node's inotify use for node-exporter's textfile collector: instances per owning
# user, and the per-user limit (README). Run as root, so every process is visible.
set -eu

out="$1"
tmp="$out.$$.tmp"
trap 'rm -f "$tmp"' EXIT

{
  echo "# HELP node_inotify_instances inotify instances held open, per owning user."
  echo "# TYPE node_inotify_instances gauge"
  # Each inotify instance is an fd linking to anon_inode:inotify; its owner is the owner of
  # the process directory. Processes that exit meanwhile are skipped by the 2>/dev/null.
  find /proc/[0-9]*/fd -lname 'anon_inode:inotify' 2>/dev/null \
    | while read -r fd; do stat -c %u "${fd%/fd/*}" 2>/dev/null || true; done \
    | sort | uniq -c \
    | awk '{ printf "node_inotify_instances{uid=\"%s\"} %s\n", $2, $1 }'
  echo "# HELP node_inotify_max_user_instances The per-user limit fs.inotify.max_user_instances."
  echo "# TYPE node_inotify_max_user_instances gauge"
  echo "node_inotify_max_user_instances $(cat /proc/sys/fs/inotify/max_user_instances)"
} > "$tmp"

chmod 0644 "$tmp"
mv "$tmp" "$out"
trap - EXIT
