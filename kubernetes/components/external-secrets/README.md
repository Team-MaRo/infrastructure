# External Secrets

Back to [`kubernetes/README.md`](../../README.md).

Workloads never talk to OpenBao. External Secrets reads values from it and writes ordinary
Kubernetes Secrets, through one `ClusterSecretStore` named `openbao` - OpenBao's `secret/`
engine, logged into with the operator's own service account (role `external-secrets` in
`tofu/openbao`, see [`tofu/openbao/README.md`](../../../tofu/openbao/README.md)).
`kubernetes/clusters/prod/external-secrets.yaml` deploys the chart `external-secrets` 2.11.0
(pinned) plus this directory: its values (`values.yaml`, memory) and the store
(`cluster-secret-store.yaml`).

## How it is set up

- `ServerSideApply=true` is required - the chart's CRDs exceed client-side apply's annotation
  limit - and the store carries `SkipDryRunOnMissingResource=true` because its CRD arrives in
  the same sync.
- Release name and namespace are both `external-secrets`, which yields exactly the service
  account the OpenBao role is bound to; the store sets no `serviceAccountRef`, so the operator
  logs in with its own token.
- It only sets what differs from the CRD defaults (`version` v2 and `mountPath` kubernetes are
  defaults).
- The store may read all of `secret/`, so every namespace that references it reaches every
  value - fine while one admin runs the cluster.

## Using it

A value goes into OpenBao by hand (see `tofu/openbao/README.md` for the port-forward and
token), and an `ExternalSecret` next to the workload pulls it in:

```yaml
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: example
  namespace: example
spec:
  secretStoreRef:
    kind: ClusterSecretStore
    name: openbao
  data:
    - secretKey: password    # key in the Kubernetes Secret
      remoteRef:
        key: example         # path under secret/, i.e. secret/example
        property: password   # field of that OpenBao secret
```

The Kubernetes Secret it creates gets the ExternalSecret's name; `target.name` is set only
where the two must differ.
