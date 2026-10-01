# Hetzner CSI driver

Back to [`kubernetes/README.md`](../../README.md).

Persistent storage is Hetzner Volumes, never the node disk. `kubernetes/components/hcloud-csi/`
deploys `hetznercloud/csi-driver` (pinned raw manifest, v2.23.0) and its default StorageClass
`hcloud-volumes`; `patches/` sets `reclaimPolicy: Retain` and the memory per container. k3s's
local-path is disabled (`k3s_config`), so nothing can put persistent data on a node's 40 GB
disk.

## A volume is not node storage

A PersistentVolumeClaim becomes a Hetzner Cloud Volume through this driver. Hetzner keeps every
block on three physical servers and attaches the volume over the network to one server at a
time; when a pod moves, the driver moves the volume with it, and deleting a server detaches,
never deletes. The driver finds its server through the metadata service, so no cloud
controller manager is needed.

- **Limits:** `ReadWriteOnce` only - it cannot be shared by pods on different nodes - 16
  volumes per server (the driver reports it to the scheduler), 10 GB minimum, nbg1 only, price
  linear per GB. A node that vanished without being deleted keeps its volumes until tainted
  `node.kubernetes.io/out-of-service`.
- **`reclaimPolicy: Retain`** (`patches/reclaim-retain.yaml`): Hetzner has **no backups of
  volumes** and Argo CD prunes, so a deleted PVC leaves a `Released` PV and the volume, to be
  removed by hand (see below). reclaimPolicy is immutable on a StorageClass.
- **Consolidate at the service, not the disk.** PVCs are namespaced and RWO, so sharing one
  between apps does not work. One Postgres cluster (CloudNativePG, a database per app,
  backups to S3), files in S3, Redis without a volume. Databases want the volume mounted
  directly - never NFS, SMB or S3 as primary storage. A shared POSIX folder, if ever needed,
  is an NFS server on one volume.
- **The token** (`kube-system/hcloud`, from `ansible/secrets.yml`) is project-wide
  read+write - Hetzner cannot scope tokens, and volumes must live in the servers' project.
  The driver's code calls only volume endpoints plus a read of servers; server
  `delete_protection` and tofu drift detection are the backstop. It is a dedicated token,
  revocable without touching OpenTofu's. It is not in git: `ansible/secrets.yml` writes it
  from 1Password (see [`ansible/roles/cluster_secrets/README.md`](../../../ansible/roles/cluster_secrets/README.md)).
- **Kubernetes 1.37 is not yet in the driver's CI** (k3s 1.33-1.36 when adopted).

## Removing a released volume

**Deleting a PVC does not delete the volume.** The class uses `reclaimPolicy: Retain`,
because Hetzner keeps no backups of volumes and Argo CD prunes whatever leaves git. The PV
turns `Released`, and the volume stays - and is billed - until removed by hand:

```shell
kubectl --context d3strukt0r-prod-admin get pv                     # STATUS Released
kubectl --context d3strukt0r-prod-admin get pv <pv> -o jsonpath='{.spec.csi.volumeHandle}'; echo
kubectl --context d3strukt0r-prod-admin delete pv <pv>
hcloud volume delete <volume id>
```

## Upgrading

Upgrading is bumping the version in the URL in `components/hcloud-csi/kustomization.yaml`.
