# cloud-init

Node bootstrap, fetched over HTTPS at first boot rather than embedded in the server. The
`user_data` on every node in `../tofu/k3s` is just this two-line `#include`:

```yaml
#include
https://raw.githubusercontent.com/Team-MaRo/infrastructure/refs/heads/master/cloud-init/k3s.yaml
```

Two consequences of that indirection:

- Editing `k3s.yaml` and pushing changes what **newly created or rebuilt** nodes get,
  immediately, with no OpenTofu run involved.
- It does nothing to running nodes. Anything that has to change on a live node is
  Ansible's job - see [`../ansible/`](../ansible/README.md).

Because the URL points at `master`, a rebuilt node picks up whatever is on that branch at
the time, not whatever existed when the node was first created.

## What `k3s.yaml` does

- Creates the `d3strukt0r` user with NOPASSWD sudo and one SSH key, disables the root
  account, and turns off password authentication.
- Drops `/etc/ssh/sshd_config.d/00-hardening.conf`, named to sort *before* cloud-init's
  own `50-cloud-init.conf`, because sshd takes the first value it sees for a given
  option. It sets `PermitRootLogin no`, `MaxAuthTries 3`, no X11 or agent forwarding, and
  `AllowUsers d3strukt0r` - which is why Ansible connects as that user and no other.
- Installs and enables fail2ban, with `backend = systemd` because Debian cloud images
  ship without rsyslog and therefore have no `/var/log/auth.log`.
- Writes `/etc/sysctl.d/99-k3s.conf` enabling IPv4 and IPv6 forwarding, required for pod
  networking. Ansible's swap role uses a separate `99-swap.conf`; no keys overlap.
- Reboots once when it is done.
