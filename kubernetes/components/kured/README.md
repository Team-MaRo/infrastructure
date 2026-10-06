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
  node cordoned and waiting. `kubectl -n kube-system logs -l name=kured` shows it.
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
