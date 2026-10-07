# system-upgrade-controller

Back to [`kubernetes/README.md`](../../README.md).

k3s upgrades itself. `kubernetes/components/system-upgrade-controller/` deploys Rancher's
system-upgrade-controller (pinned release manifests, v0.20.2, in `kustomization.yaml`) and two
Plans in `plan.yaml`: `server` for the three servers and `agent` for the workers (nodes without
the `control-plane` label). For each node it runs a privileged Job that replaces the k3s binary
and restarts k3s. Ansible's `k3s_version` is therefore only the
version a node is *installed* with; the Ansible install never upgrades (`creates:`), and a new
node catches up in the next window.

## How it upgrades

- **Channel `v1.37`, not `stable` - yet.** `stable` was still 1.36 when the cluster was
  installed at 1.37.0, and k3s-upgrade refuses to downgrade: the Job fails with
  `Current … is higher` and leaves the node cordoned. Once `stable` reaches 1.37 the channel
  switches to `stable`, which makes minor upgrades automatic too. Until then a minor is a
  one-line change to the channel, **never skipping a minor** (Kubernetes' skew policy).
  Before any minor, check that the Hetzner CSI driver supports it - its CI lagged by one.
- **Window 02:30-03:30 Europe/Zurich, daily**, after the scheduled etcd snapshot at
  00:00 UTC, so each upgrade starts with a fresh snapshot in Object Storage, and before the
  OS updates at 03:30 (see "The night's maintenance order" in
  [`AGENTS.md`](../../../AGENTS.md)). The channel is polled every 15 minutes; Jobs start only
  inside the window but may run past it.
- **Workers after servers.** The `agent` Plan's `prepare` step (`prepare server`) waits until
  the `server` Plan has finished, so a worker is never newer than the servers. Its channel and
  window are the server Plan's, and the two channels must be changed together.
- **One node at a time, cordoned but not drained.** Restarting k3s leaves running pods alone
  (the containerd shims survive), so a drain would only move volumes around. The API is
  briefly unavailable on each node; etcd keeps quorum with two of three.
- **A failed Job leaves its node cordoned.** Look at the Job's log in `system-upgrade`, fix
  the cause, then `kubectl uncordon <node>`.
- **On a worker, one failed pod per upgrade is normal.** The first pod replaces the binary and
  restarts k3s-agent, which runs the kubelet that runs the pod - its end is never reported, so
  it shows `Unknown`, exit code 255. The Job's retry finds `Binary already been replaced`, exits
  0 and uncordons the node: the Job ends with one success and one failure (first seen
  2026-10-07 on prod-04). Only a Job without a success, or a node left cordoned, needs a look.
- The script exits early when the binary is already the target version, so re-running a
  Plan is harmless.

## Watching it

```shell
kubectl --context d3strukt0r-prod-admin -n system-upgrade get plan -o wide          # the version each Plan aims for
kubectl --context d3strukt0r-prod-admin -n system-upgrade get jobs                 # one per node and upgrade
kubectl --context d3strukt0r-prod-admin get nodes                                  # versions, SchedulingDisabled
```

A node left `SchedulingDisabled` after a failed Job: read the Job's log, fix the cause, then
`kubectl uncordon <node>`.

## Moving to the next minor

The channel is `v1.37` until `stable` reaches 1.37, then `stable`:

```shell
curl -s https://update.k3s.io/v1-release/channels | jq -r '.data[] | select(.id=="stable") | .latest'
```

Until the switch, moving to the next minor is changing the channel to the next one - one
minor at a time, after checking that the Hetzner CSI driver supports it.
