# Cluster RBAC

Back to [`kubernetes/README.md`](../../README.md).

Cluster-wide rights for people who log in to kubectl through Zitadel (Application
`cluster-rbac`, with the finalizer). How that login works - the everyday kubeconfig and the
break-glass admin certificate - is "`kubeconfig.yml`" in
[`ansible/README.md`](../../../ansible/README.md); how the API server accepts Zitadel's tokens
is "Authentication: Zitadel's ID tokens" in
[`ansible/roles/k3s/README.md`](../../../ansible/roles/k3s/README.md).

- **Rights come from `kubernetes/components/cluster-rbac/`**: the ClusterRoleBinding
  `zitadel-infra-admin` (`infra-admin.yaml`) binds group `zitadel:infra-admin` to
  `cluster-admin` - so the role `infra-admin` in Zitadel's project `Infrastructure` makes you
  cluster admin. RBAC that belongs to one app stays in that app's component.
- The API server prefixes every username and group from a Zitadel token with `zitadel:`, so
  these bindings can never match a built-in `system:` name.
- **Revoking**: Kubernetes never asks Zitadel whether a token is still good. A removed role or
  a deactivated user keeps working until the ID token expires - at most 1 hour, the instance's
  token lifetime (`zitadel_default_oidc_settings` in `tofu/zitadel/policies.tf`); then the
  refresh fails. Deleting the binding ends it at once. The refresh token lapses after 30 days
  unused and 90 days after the browser login in any case - a browser login comes back then.

## Checking it

Every day through Zitadel, context `d3strukt0r-prod` (written by `ansible/kubeconfig.yml`, with
kubelogin installed): the first command opens the browser, then the ID token lasts an hour and
renews itself without the browser - for 90 days after the login, or until 30 days unused.

```shell
kubectl --context d3strukt0r-prod auth whoami   # zitadel:<username>, group zitadel:infra-admin
```

The role `infra-admin` in Zitadel's project `Infrastructure` makes that user cluster admin
(`components/cluster-rbac/`). Break-glass, with Zitadel down, is context
`d3strukt0r-prod-admin`, the k3s admin certificate.
