# hostname

Runs before `k3s` in both plays of `prod.yml`. cloud-init sets each node's hostname and
`/etc/hosts` on every boot from Hetzner's metadata - but the metadata keeps the name a
server was *created* with, so renaming a server in tofu never reaches the host by itself.
This role writes `/etc/cloud/cloud.cfg.d/90-hostname.cfg` with `hostname:` set to the
inventory name (the current Hetzner name), which cloud-init then uses instead, and applies
it immediately so the k3s role, which takes the node name from the hostname at install
time, already sees it.

Why the server name matters - it is the OS hostname, the Kubernetes node name and the etcd
member name - and how a rename is done is in
[`tofu/hcloud/README.md`](../../../tofu/hcloud/README.md).

## Running it

Tagged `hostname`, so it can run on its own:

```shell
cd ansible
ansible-playbook prod.yml --tags hostname
```
