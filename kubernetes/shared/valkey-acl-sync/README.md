# valkey-acl-sync

A Kustomize Component: `acl-sync`, which builds a Valkey's users from its users Secrets and loads
them while it runs, for both Valkeys
([`../../components/valkey-cache/README.md`](../../components/valkey-cache/README.md),
[`../../components/valkey-state/README.md`](../../components/valkey-state/README.md)). Listing it
adds the script (`acl-sync.sh`, as the ConfigMap `valkey-acl-sync`), an initContainer, a sidecar
and two volumes to the Valkey's workload (`sidecar.yaml`, patched onto the kustomization's
Deployment or StatefulSet).

## Why

The apps bring their own users: each a Secret `db-<n>` with one `user <app> <rules>` line, mounted
with `users` and all other optional `db-1` to `db-15` as one projected volume. Valkey reads users
only at start, and a new app's Secret appears after the pod runs - so a new user needed a
restart, and Reloader's `reloadOnCreate` restarted every annotated app whenever Reloader itself
restarted (stakater/Reloader#1089; `components/reloader`). And had Valkey read those files
itself, one app's broken or glued line would have stopped it from starting, for every app on it.

## How it works

- **Valkey reads only an `aclfile`** in an emptyDir in memory (`/run/valkey-local/users.acl`,
  `aclfile` in `valkey.conf`), never the Secrets' files. `acl-sync` writes it, and **every line
  in it has been checked first**: tried on a scratch user (`ACL SETUSER acl-sync-check reset
  <rules> off`, then deleted - `off` last, so the scratch user can never be logged in to). A line
  Valkey refuses never reaches the file, so Valkey always starts.
- **`acl-sync.sh init`** (initContainer `acl-sync-init`) makes acl-sync's own login - a random
  password and a line `user acl-sync ... +acl|setuser +acl|deluser +acl|load` - and the first
  aclfile, checking against a short-lived Valkey on a socket in the emptyDir. Nothing stored in
  OpenBao or 1Password; a new password at every start, read by `valkey-cli` from
  `VALKEYCLI_AUTH`, not the command line. `acl|setuser` alone lets that user give itself any
  right, so it is as good as admin - but the password exists only in the pod's memory.
- **Valkey's own users cannot be taken over**: `users.conf` is read first, and an app's file
  that names one of its users (`default`, `exporter`) is refused, as are `acl-sync` and
  `acl-sync-check`. And **no aclfile goes out without a checked `default` line**: Valkey creates a
  missing `default` open - no password, every command - and with `protected-mode no` that would
  let every allowed namespace in. Without one, `init` fails (the pod does not start) and the
  sidecar loads nothing.
- **One Valkey version**: the containers run Valkey's image with the tag from the Valkey's
  `kustomization.yaml` (`images:`), so the lines are checked by the Valkey that loads them.
- **`acl-sync.sh run`** (sidecar `acl-sync`) checks the mounted files every 15 seconds. The kubelet
  updates the projected volume in place - a changed Secret, and an optional one that appears or
  disappears - within one to two minutes (37-81 s on kind, 2026-11-16). On a change it rebuilds the
  file against the running Valkey and loads it with `ACL LOAD`, which takes the whole file or
  nothing.
- **What loading does to connections** (tested on kind): users whose line did not change keep
  their open connections; a new password keeps them too, only new logins need it; a user that is
  gone has its connections closed.
- **A user whose Secret disappears stays for five minutes** (`ACL_SYNC_REMOVAL_GRACE`, 300 s): an
  ExternalSecret that Argo CD prunes and recreates must not cut its app off.
- **Safety**: a file that changed while being read is read again next round, as is any round
  that failed. Valkey not answering (restarting, `maxclients`, a 10-second timeout on every call)
  is retried, not reported; acl-sync's own login or rights failing is reported. A removed user's
  grace is forgotten only once the file without it has loaded. The
  sidecar runs without `set -e`, so a failed round never ends it (each command that can fail is
  handled where it runs); it stops at once on SIGTERM. `init` does the opposite
  (`set -e -o pipefail`): any failure stops it - an empty password, no `default` user, no
  checking Valkey - and Kubernetes retries it, since nothing runs yet.
- **Same image as Valkey** (`valkey/valkey`, the tag from the Valkey's kustomization:
  `valkey-server`, `valkey-cli`, `sh`), as the
  image's user 999, read-only root. Its memory is on the namespace's defaults.

## When a line is refused

The app's user keeps the rules of its last good line until the pod is recreated (a container
restart keeps the emptyDir); a new app - and in a new pod every app with a refused line - gets
no user at all and cannot use that Valkey. Valkey and every other app are unaffected, at
runtime and at every start. A refused `default` line is the exception: see above.
`acl-sync` logs `acl-sync: failed: user <name> in <file> was refused (<Valkey's error>)` every
15 seconds while it lasts, as it does for a user defined in two files (only the first counts) and
for a line with quotes or backslashes (Valkey's own parser takes them, `acl-sync` splits on
spaces only - our lines never have any). Alloy counts these lines
(`loki_process_custom_acl_sync_failed_total`, `acl_sync:failed_lines:new10m` in
`components/alloy/rules.yaml`), and `ValkeyCacheUsersBroken` / `ValkeyStateUsersBroken` fire.
Fix the app's `db-<n>` template; the alert clears ten minutes after the last failed line.

A sidecar that exits or turns unready on an error was not an option: the pod would drop out of
its Service, and the whole Valkey with it.

## Using it

In the Valkey's `kustomization.yaml`:

```yaml
components:
  - ../../shared/valkey-acl-sync
```

The patch goes onto the kustomization's Deployment or StatefulSet named `valkey-*`. It needs a
container named `valkey` and the projected volume `users` with the users Secrets (the patch adds
the rest); `valkey.conf` the line `aclfile /run/valkey-local/users.acl` and no `user` lines of
its own - Valkey refuses both together; and the kustomization an `images:` entry with Valkey's
tag.
