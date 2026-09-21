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

# domains

Ten domains registered at Infomaniak, all delegated to Cloudflare. Infomaniak is
registrar only, so nothing about them is *managed* from here - `tofu/domains.tf`
declares the expected nameservers and DNSSEC state, and every `tofu plan` checks them
against the TLD registry and warns on drift.

Infomaniak's API can write nameservers but not read them, so detection and correction
are split: the check reads the registry over DNS, and a `terracurl_request` in
`tofu/domains_nameservers.tf` issues the `PUT` to fix it. That resource only exists for
domains that have drifted, so in a steady state it plans nothing.

Correcting drift needs an Infomaniak API token, created at
<https://manager.infomaniak.com/v3/ng/profile/user/token/list> with scopes
`domain:write` and `domain:read`. The write scope is what the `PUT` needs; the read
scope is there only so the token can be verified with a harmless `GET`.

The token is displayed once, and is deactivated after a year of inactivity - which
this one will reach, because it is used only when delegation drifts. A 401 later means
recreate it, not that something is broken.

```sh
export TF_VAR_infomaniak_token=...
```

Check it before relying on it - 200 means token and scope are good, 401 a bad token,
403 a missing scope:

```sh
curl -sS -o /dev/null -w '%{http_code}\n' \
  -H "Authorization: Bearer $TF_VAR_infomaniak_token" \
  https://api.infomaniak.com/2/domains/domains
```

The token is sent through write-only attributes, so it never reaches the state file.
It is only needed when a correction is actually pending; ordinary plans do not use it.

The DNSSEC probe shells out to `dig`, so it has to be on PATH.
