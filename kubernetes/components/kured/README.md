# kured

Back to [`kubernetes/README.md`](../../README.md).

Reboots are kured's (`kubernetes/components/kured/`, pinned release manifest 1.23.0 plus the
reboot window in `kustomization.yaml`, in `kube-system`). A kernel update leaves
`/var/run/reboot-required` behind - written by unattended-upgrades' hook in
`/etc/kernel/postinst.d/` - and kured, checking every 10 minutes inside its window (04:30-06:00
Zurich time), takes a lock on its own DaemonSet, cordons and **drains** the node, reboots it and
uncordons it once it is back; then the next node may take the lock. How that window fits
between the k3s upgrades and the OS updates is "The night's maintenance order" in
[`AGENTS.md`](../../../AGENTS.md).

## How it reboots

- **Drain, unlike the k3s upgrade.** A reboot does stop every container, so pods are
  evicted first. OpenBao moves to another node and its volume re-attaches there, so it is
  briefly unavailable; External Secrets only delays refreshes meanwhile.
- **A drain that cannot finish blocks the reboot** (no `--force-reboot`), and nothing ever
  times it out (`--drain-timeout` 0): a PodDisruptionBudget allowing no disruption keeps the
  node cordoned and waiting. `kubectl -n kube-system logs -l name=kured` shows it. A drain
  that has started keeps going after the window; deleting the kured pod on that node stops it
  (the new one waits for the next window), then `kubectl uncordon`.
- **Never the MariaDB primary's node** (`--blocking-pod-selector=k8s.mariadb.com/role=primary`,
  since 2026-11-03): an evicted primary comes back unable to rejoin and is rebuilt from a backup
  (`../mariadb/README.md`, "A planned reboot never evicts the primary"). kured checks the
  selector before it takes the lock or cordons, and only against pods on its own node, so a
  blocked node stays schedulable and the next node may reboot meanwhile; its log says "Reboot
  required, but blocked at this time". `--annotate-nodes` marks a node that needs a reboot with
  `weave.works/kured-most-recent-reboot-needed` (removed once it needs none); mariadb's CronJob
  `mariadb-primary-off-rebooting-node` sees that on the primary's node and switches the primary
  away, and kured reboots the node on its next round (at most 10 minutes later).
- **`NodeRebootPending`** (warning) fires when a node has needed a reboot for over 26 hours -
  a blocked node is never cordoned, so `NodeCordonedTooLong` does not see it. The metric
  `kured_reboot_required` comes from the PodMonitor `pod-monitor.yaml`. Check kured's log on the
  node and the last job of `mariadb-primary-off-rebooting-node` in `mariadb`: it switches only
  while the MariaDB and both its pods are Ready.
- **Memory 16Mi / 64Mi** (a patch in `kustomization.yaml`; real-memory p95 14 MiB; working set
  p95 17 MiB, peak 17 MiB over 2026-10-23..11-02); kube-system has no LimitRange.
- **Only kernels set the sentinel.** A host service using an updated library keeps the old
  copy until it restarts - there is no `needrestart` on these nodes - but containers bring
  their own libraries anyway, so this only concerns the few host daemons.

## Watching and testing it

kured reboots a node when a kernel update asks for it (`/var/run/reboot-required`), one
node at a time and only between 04:30 and 06:00 Zurich time: cordon, drain, reboot,
uncordon.

```shell
kubectl --context d3strukt0r-prod-admin -n kube-system logs -l name=kured --prefix   # every node
```

To test it, or to have a node rebooted in the next window without a kernel update, create
the file yourself:

```shell
ssh prod-03 sudo touch /var/run/reboot-required
```

`/var/run` is a tmpfs, so the file disappears with the reboot.
