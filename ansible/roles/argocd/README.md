# argocd

Run by `ansible/argocd.yml`. Argo CD cannot deploy itself the first time, so this installs it
**once** from `kubernetes/components/argocd/` and applies
`kubernetes/clusters/<cluster_name>/root.yaml` (`cluster_name: prod` in
`inventories/group_vars/prod.yml`). From then on Argo CD manages itself and every other
component from `kubernetes/` on `master`, and this finds it installed and does nothing - the
same bootstrap-once trade the k3s role makes with `creates:`. How Argo CD runs afterwards is
[`kubernetes/components/argocd/README.md`](../../../kubernetes/components/argocd/README.md).

## How it works

- **The guard**: it checks for the `argocd-server` Deployment and skips everything if
  present. A guard error other than `NotFound` (typically 6443 unreachable) fails loudly,
  with `kubectl`'s own error, instead of being read as "not installed".
- **That guard is deliberate in both directions.** Ansible must never re-apply those
  manifests once Argo CD owns them: a working copy that differs from `master` would be
  fighting Argo CD's self-heal. And on a rebuilt cluster the guard is absent, so it
  bootstraps again.
- **It runs `kubectl` from the admin's machine**, not on the nodes, with the admin
  kubeconfig (`admin_kubeconfig`, same group_vars), applying straight from the working copy.
  So it runs after `kubeconfig.yml`, and only from an address in `admin_ips` - port 6443 is
  closed to everything else; nothing is copied to the nodes. The play targets `prod-01` only
  to pick up the group's variables; it makes no SSH connection.
- Like `kubeconfig.yml` it is not imported by `site.yml`.

## Running it

**Push before running it.** The root Application syncs from GitHub, not from your working
copy. See [`kubernetes/components/argocd/README.md`](../../../kubernetes/components/argocd/README.md)
for what happens after the bootstrap.

```shell
cd ansible
ansible-playbook argocd.yml
```
