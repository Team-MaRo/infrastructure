# Alloy

Log collection on every node (`kubernetes/clusters/prod/alloy.yaml`, chart `grafana/alloy`
pinned, values and extra manifests in this directory). How the cluster's manifests fit together
is in [`kubernetes/README.md`](../../README.md).

Alloy runs on every node and sends that node's container logs (`/var/log/pods`) and whole
journal (k3s, kernel, apt, sshd), plus the cluster's Kubernetes events, to Loki, which keeps
them 30 days. Storage, labels and searching are in [`../loki/README.md`](../loki/README.md).

## Design

- **Alloy runs in `kube-system`**, the PSA-exempt namespace, because it mounts `/var/log` from
  the host. The log files belong to root with mode 0640, so it runs as UID 0 - but with no
  capabilities, no privilege escalation and a read-only root filesystem; as the files' owner,
  root needs no capability to read them. Its read positions live on the node
  (`/var/lib/alloy`), so a restarted pod neither re-sends nor skips lines, and it mounts
  `/etc/machine-id`, by which the journal reader finds the node's own journal.
- **Its chart's RBAC is not used.** Alloy's chart role reads every Secret and ConfigMap, so
  `rbac.create: false` and `clusterrole.yaml` grants only pods, namespaces and events.
- **Events are collected once**: the Alloy instances form a cluster, and a
  `loki.source.kubernetes_events` watching all namespaces runs on one of them. The API keeps
  events for an hour; Loki keeps them 30 days.
- **Labels**: `namespace`, `pod`, `container`, `node`, `app` for containers; `job="node-journal"`,
  `unit`, `node` for the journal; `job="loki.source.kubernetes_events"` for events. Anything
  else is searched in the line, not labelled - Loki stays fast with few labels.
- Alloy sends no usage statistics (`enableReporting: false`).
