# node_metrics

Numbers about the node that node-exporter cannot read itself, written as text files for its
textfile collector, each by a systemd timer: a small script of ours, or Debian's collector
package where it has the metric; node-exporter (kube-prometheus-stack, in `kube-system`) reads every `*.prom` file in
`node_metrics_textfile_dir` (`/var/lib/prometheus/node-exporter`) through its existing mount of
the host's root (`--collector.textfile.directory=/host/root/var/lib/prometheus/node-exporter`,
[`kubernetes/components/kube-prometheus-stack/README.md`](../../../kubernetes/components/kube-prometheus-stack/README.md)).

## inotify

`files/inotify-metrics.sh`, every minute (`node-metrics-inotify.timer`), writes `inotify.prom`:

| Metric | Meaning |
|---|---|
| `node_inotify_instances{uid}` | inotify instances held open, summed per owning user (every fd linking to `anon_inode:inotify`) |
| `node_inotify_max_user_instances` | the per-user limit, `fs.inotify.max_user_instances` |

The limit is per user, and on a node nearly everything runs as root: containerd, k3s and every
container running as UID 0 share root's instances. Debian's default of 128 was nearly used up
(prod-01: 114, 2026-11-20); the k3s role raises it to 1024
([`roles/k3s/README.md`](../k3s/README.md), "Kernel parameters and node labels"). The
prometheus-community `inotify-instances` script was not used: it labels every series with the
process ID, which churns series in Prometheus, and needs Python's prometheus_client.

The script writes to a temporary file and renames it, so node-exporter never reads half a file.
Alerts (kube-prometheus-stack `rules.yaml`): `NodeInotifyInstancesHigh` at 80 % of the limit
for a user, and `NodeTextfileStale` when a file has not been rewritten for 10 minutes (the
timer stopped).

## Pending apt updates

Debian's `prometheus-node-exporter-collectors` package, its `apt_info.py` run every 15 minutes
by its own `prometheus-node-exporter-apt.timer`, writes `apt.prom`:

| Metric | Meaning |
|---|---|
| `apt_upgrades_pending{origin,arch}` | packages with an update, per origin (`Debian:trixie-security/stable-security`; a package in several origins joins them with `,`) |
| `apt_upgrades_held{origin,arch}` | updates held back |
| `apt_autoremove_pending` | packages `apt autoremove` would remove |
| `apt_package_cache_timestamp_seconds` | when the package lists were last fetched |
| `node_reboot_required` | `/run/reboot-required` exists (kured acts on it; its own metric drives `NodeRebootPending`) |

It only reads apt's cache: the lists are fetched by os_updates' 03:00 `apt-daily` run
([`roles/os_updates/README.md`](../os_updates/README.md)), and unattended-upgrades installs at
03:30. Installed without Recommends (ipmitool, nvme-cli, smartmontools, jq). The package enables
the timers of all its collectors; the four for hardware the nodes do not have (ipmitool-sensor,
mellanox-hca-temp, nvme, smartmon) are masked before the install, or they would fail every run
and `NodeSystemdServiceFailed` would report them.

Alerts (kube-prometheus-stack `rules.yaml`): `NodeUpdatesPending` when updates from `stable` or
`stable-security` - the origins unattended-upgrades installs from - have waited 48 hours
(`stable-updates` stays pending until Debian's next point release, so it is left out),
`NodePackageListsStale` when the lists were not fetched for two days, and `NodeTextfileStale`
when `apt.prom` was not rewritten for 40 minutes.

## Running it

Run by `prod.yml` on every node, tagged `node_metrics`:

```shell
cd ansible
ansible-playbook prod.yml --tags node_metrics
ansible prod -b -m command -a 'cat /var/lib/prometheus/node-exporter/inotify.prom'
ansible prod -b -m command -a 'cat /var/lib/prometheus/node-exporter/apt.prom'
ansible prod -m command -a "systemctl list-timers 'prometheus-node-exporter-*' --no-pager"   # only the apt timer
```

Which processes hold the instances, on one node:

```shell
ansible prod-01 -b -m shell -a 'for f in $(find /proc/[0-9]*/fd -lname anon_inode:inotify 2>/dev/null); do cat ${f%/fd/*}/comm; done | sort | uniq -c | sort -rn | head'
```
