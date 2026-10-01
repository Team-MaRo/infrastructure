---
paths:
  - "ansible/**"
---

# Ansible

- Read `ansible/README.md` and the role's own `README.md` before changing a role or playbook;
  update the role's README in the same change.
- Run everything from `ansible/` - `ansible.cfg` is only read from the current directory.
- Never write a playbook with `hosts: all`: the default inventory holds `prod` and `home`,
  and only `hosts:` keeps a playbook to its group.
- No feature flags (`*_enabled`): a role is either in a playbook's `roles:` list or it is not.
- `prod-01` is written out literally in host patterns and kept in step with `k3s_init_node`.
- Install tasks are guarded by `creates:`; anything that may change later goes into
  `k3s_config` (a drop-in), not into the install flags.
- The admin runs playbooks. An agent may run `ansible-playbook <playbook> --check --diff`
  and `ansible-inventory --graph`.
- Secrets come from 1Password through `op` at run time, go to `kubectl` on stdin and are
  `no_log`; there is no Ansible Vault.
