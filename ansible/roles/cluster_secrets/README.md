# cluster_secrets

Run by `ansible/secrets.yml`. Writes the Secrets the cluster needs before OpenBao can supply
any - `kube-system/hcloud`, the CSI driver's Hetzner token, which OpenBao itself needs for
its volumes, and `openbao/openbao-seal`, OpenBao's own seal key. The seal key's namespace is
created by the openbao Application, so after a fresh install run this once that has synced.

The list is `cluster_secrets` in `inventories/group_vars/prod.yml`. Each entry names a
Secret, a key, and the 1Password item and field its value comes from: one field of one
1Password item in `onepassword_account` (`my.1password.com`, pinned because a second, work
account is signed in too) and `onepassword_vault` (`Private`). How 1Password items are
referenced in this repo is under "Conventions" in [`AGENTS.md`](../../../AGENTS.md).

## How it works

- **1Password is the bootstrap root of trust only.** The `op` CLI reads the values on the
  admin's machine (Touch ID prompt); nothing in the cluster talks to 1Password. There is no
  Ansible Vault - it would only be a second place for the same secrets.
- **Values go to `kubectl` on stdin**, never argv (visible in the process list), and every
  task is `no_log`.
- **Server-side apply, field manager `ansible`**: client-side apply would copy the value
  into the `last-applied-configuration` annotation. `kubectl diff --server-side` runs
  first, read-only, so `--check` is accurate, and only what differs is applied, so a re-run
  reports `changed=0`.
- Same shape as `argocd.yml`: runs from the Mac with the admin kubeconfig, from an address in
  `admin_ips`, targets `prod-01` only for its variables and makes no SSH connection, not
  imported by `site.yml`. These Secrets are not in git, so Argo CD neither prunes nor
  overwrites them.

## Running it

```shell
cd ansible
ansible-playbook secrets.yml --check --diff
ansible-playbook secrets.yml
```

Rotating one is changing it in 1Password and re-running this.
