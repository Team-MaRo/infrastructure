---
name: resize-server
description: Resize (upgrade or downgrade) a prod node's Hetzner server type, e.g. "upgrade prod-02 to cx43", "scale prod-04 down to cx23", "cx43 is available, let's upgrade". Use it for any server_type change of prod-01..04 - it holds the order (database primaries off the node, drain, tofu plan/apply, uncordon) and a one-shot preflight script, so nothing has to be looked up while the admin waits.
---

# Resizing a prod node

The admin wants this done fast: run the preflight once, then hand over **all** commands for the
whole sequence in one message, in order. Check between steps only where it decides the next step,
with one command each - no exploration.

## What a resize is here

- Server types live in `tofu/hcloud/locals.tf` (`servers.<node>.server_type`). Every server has
  `keep_disk = true`: a resize changes CPU and RAM only. The disk stays at 40 GB, whatever size
  the type lists (prod-01 as cx43 still has 40 GB), so a downgrade stays possible. It powers the
  server off and on (prod-01 to cx43, 2026-11-05: `tofu apply` took 2m20s) - an in-place
  update, never a replacement.
- prod-01..03 are k3s servers with etcd (three members, quorum two): resize **one at a time**,
  the next only when the previous is back and Ready. prod-04 is the worker.
- The admin runs `tofu apply`, the switchovers, drain and uncordon (repo rule: changes are the
  admin's). The agent edits `locals.tf`, runs `tofu fmt`/`validate`/`plan`, and checks.

## 1. Preflight (read-only, one run)

```sh
.claude/skills/resize-server/scripts/preflight.sh <node> <type>
```

It stops with `PREFLIGHT FAILED` when a read fails or comes back empty - then nothing it printed
counts. Otherwise it shows: whether the type is orderable in nbg1, every node's Ready state and
whether it is due for a kured reboot (and a stale kured mark to remove), the database
primaries and which instance to switch to, the pods on the node (volume pods marked), disruption
budgets that allow no eviction, memory requests against allocatable per node with an estimate for
the new type, the kured lock and the time.

**Stop and say why** - and hand over no commands - if:

- `orderable in nbg1: NO` (or `not offered`). Nothing to do until hetzner-watch's ntfy message
  says `server-type/<type>/nbg1: not orderable -> orderable` (it checks every 15 minutes); then
  run the preflight again. It watches only the types in `WATCH_SERVER_TYPES` in
  `kubernetes/components/hetzner-watch/cronjob.yaml` (comma-separated, e.g. `cx43,cx53`) - offer
  to add the type there, or the message never comes.
- The kured lock is held, it is 01:00-06:00 (the night's maintenance windows in AGENTS.md - the
  etcd snapshot moves to 01:00 in winter) or close enough that the resize would run into it (it
  takes about half an hour), or another node is not Ready.
- A **downgrade** whose `as <type>` estimate is below the node's own requests (they come back to
  it after the uncordon), or where the other nodes' free room (allocatable minus requested) cannot
  take the node's requests during the drain. Above about 85% of the estimate it fits but is
  tight - say so.

A budget with 0 allowed only matters for pods on this node: `postgres-primary` protects the
Postgres primary and goes away with the promote.

Otherwise go straight on: the admin's resize request is the go-ahead for editing `locals.tf` and
planning - both local and reverted in a second; everything that changes the cluster or Hetzner
stays the admin's to run.

## 2. Change the type and plan

Edit `server_type` of the node in `tofu/hcloud/locals.tf`, then:

```sh
cd tofu/hcloud && tofu fmt -check && tofu validate && tofu plan
```

The plan must show exactly `hcloud_server.this["<node>"] will be updated in-place`,
`server_type = "<old>" -> "<new>"`, `Plan: 0 to add, 1 to change, 0 to destroy`. Anything else
(replace, destroy, other resources) is a bug - stop.

## 3. Hand over the commands, in this order

Only include a switchover when the preflight shows that primary on the node.

1. **Cordon first** - nothing new lands on the node, and mariadb's CronJob
   `mariadb-primary-off-rebooting-node` never switches the primary back onto a cordoned node (it
   would if another node has a reboot pending). CloudNativePG may switch Postgres away on its own
   when its primary's node is cordoned:
   ```sh
   kubectl --context d3strukt0r-prod-admin cordon <node>
   ```
2. **Postgres primary on the node** - if `PRIMARY` still shows the node's instance after the
   cordon, promote the instance the preflight names (the budget `postgres-primary` otherwise
   blocks the drain); drain only once `PRIMARY` shows the other one:
   ```sh
   kubectl --context d3strukt0r-prod-admin -n postgres get cluster postgres
   kubectl cnpg promote postgres <other-instance> -n postgres --context d3strukt0r-prod-admin
   ```
   (the plugin wants `--context` after the plugin name)
3. **MariaDB primary on the node** - switch it to the index the preflight names; an evicted
   primary fails over unclean and gets rebuilt (MariaDB README "A planned reboot never evicts the
   primary"):
   ```sh
   kubectl --context d3strukt0r-prod-admin -n mariadb patch mariadb mariadb --type=merge -p '{"spec":{"replication":{"primary":{"podIndex":<other>}}}}'
   kubectl --context d3strukt0r-prod-admin -n mariadb get mariadb mariadb
   ```
   Drain only once `PRIMARY` shows `mariadb-<other>` and it is Ready: a drain during the switch
   can evict the operator itself (prod-02, 2026-11-05: the switch took a minute longer, finished by
   the operator's new pod).
4. **Drain** (volume pods - OpenBao, valkey-state, Loki, Tempo - pause while their volume moves):
   ```sh
   kubectl --context d3strukt0r-prod-admin drain <node> --ignore-daemonsets --delete-emptydir-data --timeout=15m
   ```
5. **Resize** - check availability once more right before (it can vanish within the hour: CX43
   was orderable for prod-01 and gone 20 minutes later for prod-02, 2026-11-05). `[false]`: do
   **not** apply - the apply would power the server off and then fail; uncordon (step 6) and wait
   for hetzner-watch. `[true]`: apply, and answer `yes` after checking the plan matches section 2:
   ```sh
   hcloud --context d3strukt0r-infrastructure server-type describe <type> -o json | jq '[.locations[] | select(.name=="nbg1") | .available]'
   cd ~/Projects/infrastructure/tofu/hcloud && tofu apply
   ```
   **If it fails with `resource_unavailable`**: the server is already **off** and still the old
   type - etcd runs on two members until it is back. Power it on at once, wait until the node
   shows `Ready` (Ctrl-C then), uncordon it - that replaces step 6 - and set `server_type` back in
   `locals.tf` (state still holds the old type, so the next plan is clean again):
   ```sh
   hcloud --context d3strukt0r-infrastructure server poweron <node>
   kubectl --context d3strukt0r-prod-admin get node <node> -w
   kubectl --context d3strukt0r-prod-admin uncordon <node>
   ```
6. **Schedulable again:**
   ```sh
   kubectl --context d3strukt0r-prod-admin uncordon <node>
   ```

Between 4 and 5, check once that only DaemonSet pods are left on the node and nothing is stuck
elsewhere:

```sh
kubectl --context d3strukt0r-prod-admin get pods -A --field-selector spec.nodeName=<node> --no-headers
kubectl --context d3strukt0r-prod-admin get pods -A --no-headers | grep -vE 'Running|Completed'
```

A pod starting elsewhere does not block the resize; one Pending for lack of room does.

## 4. After

- `kubectl --context d3strukt0r-prod-admin get node <node>`: Ready, new allocatable memory;
  Postgres (`kubectl cnpg status postgres -n postgres --context d3strukt0r-prod-admin`) and
  MariaDB (`kubectl --context d3strukt0r-prod-admin -n mariadb get mariadb`) healthy; Argo CD apps
  Synced/Healthy; no new alerts.
- The resize was a reboot outside kured: if the node still carries kured's mark
  `weave.works/kured-reboot-in-progress`, it is stale now (the reboot it asked for happened) -
  hand over its removal:
  ```sh
  kubectl --context d3strukt0r-prod-admin annotate node <node> weave.works/kured-reboot-in-progress-
  ```
- Docs in the same change: the server-type line in `tofu/hcloud/README.md`, and the per-node
  memory figures in `ansible/roles/k3s/README.md`. A bigger server may want a larger
  `kubeReserved` (its reasoning is in that README) - mention it, don't change it on the side.
- Commit (on the admin's go, per the commit rules): `feat: resize <node> to <type>` or
  `feat: scale <node> down to <type>`.
- The volume pods stay where they moved; the node comes back nearly empty (no automatic
  rebalancing). The same goes for the database primaries: leave them on the instance they were
  switched to, a switch back is only another disruption.
