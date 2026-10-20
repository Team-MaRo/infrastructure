# external-secret-defaults

A Kustomize Component: every ExternalSecret in a kustomization that lists it gets

- `secretStoreRef`: the `ClusterSecretStore` `openbao`, the cluster's only store
  ([`../../components/external-secrets/README.md`](../../components/external-secrets/README.md));
- `refreshInterval: 5m` - the CRD's default is an hour, so a value changed in OpenBao would take
  that long to reach its Secret, and the apps restarted by Reloader.

So an ExternalSecret holds only what is its own - its name and `data` (or `target`) - and the
two settings shared by all of them live here once.

## Using it

In the component's `kustomization.yaml`:

```yaml
components:
  - ../../shared/external-secret-defaults
```

and the ExternalSecrets there leave out `refreshInterval` and `secretStoreRef`. The patch sets
both on every ExternalSecret it renders, so one that needs another store cannot live in that
kustomization (trust-manager's CA copies, which read through namespaced stores, do not list it).

## Where it does not reach

Kustomize applies it, so it reaches nothing that Kustomize does not render: the WordPress chart's
ExternalSecrets (Helm, `charts/wordpress/templates`), ExternalSecrets in Helm values, and
directories synced as plain manifests without a `kustomization.yaml`
(`clusters/prod/etcd-snapshots/`, the private repository's) - those set both fields themselves.

## Checking it

The rendered output still carries both fields:

```shell
kubectl kustomize kubernetes/components/<app> | yq 'select(.kind == "ExternalSecret") | .spec | {"refreshInterval": .refreshInterval, "store": .secretStoreRef}'
```
