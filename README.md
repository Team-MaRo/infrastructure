# cloud-init

```yaml
#include
https://raw.githubusercontent.com/Team-MaRo/infrastructure/refs/heads/master/cloud-init/k3s.yaml
```

# tofu

OpenTofu config for the Hetzner Cloud side of the cluster. State lives in the
Hetzner Object Storage bucket `d3strukt0r-tfstate` (nbg1) via the S3 backend,
with native locking.

The infrastructure was created by hand first and adopted afterwards, so
`tofu/imports.tf` holds the `import` blocks for every resource.

Credentials come from the environment, never from a file in this repo:

```sh
export HCLOUD_TOKEN=...
export AWS_ACCESS_KEY_ID=...      # Hetzner Object Storage access key
export AWS_SECRET_ACCESS_KEY=...
```

```sh
cd tofu
tofu init
tofu plan
```

Two things the servers depend on that OpenTofu cannot see:

- `user_data` and `ssh_keys` are not returned by the Hetzner API, so both are
  under `ignore_changes` on `hcloud_server`. Changing either in the config has
  no effect on existing nodes - it only applies to newly created ones.
- The firewall is attached purely through the `role=k3s` label selector. Adding
  a `hcloud_firewall_attachment` resource would conflict with it.

The servers, the network and its subnet carry `prevent_destroy = true`. The
server types are cost-optimized and may not be available again once released,
so any plan that would destroy or replace one fails instead. Retiring a node
means removing that line first, deliberately.
