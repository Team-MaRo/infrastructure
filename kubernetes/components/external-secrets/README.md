# External Secrets

Back to [`kubernetes/README.md`](../../README.md).

Workloads never talk to OpenBao. External Secrets reads values from it and writes ordinary
Kubernetes Secrets, through one `ClusterSecretStore` named `openbao` - OpenBao's `secret/`
engine, logged into with the operator's own service account (role `external-secrets` in
`tofu/openbao`, see [`tofu/openbao/README.md`](../../../tofu/openbao/README.md)).
`kubernetes/clusters/prod/external-secrets.yaml` deploys the chart `external-secrets` 2.11.0
(pinned) plus this directory: its values (`values.yaml`, memory and the ServiceMonitors), the
store (`cluster-secret-store.yaml`) and the alert (`rules.yaml`).

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
token), and an `ExternalSecret` next to the workload pulls it in. The store and the refresh
interval come from the Kustomize Component
[`../../shared/external-secret-defaults/`](../../shared/external-secret-defaults/README.md),
which the component's `kustomization.yaml` lists under `components:`; the ExternalSecret itself
holds only what is its own:

```yaml
apiVersion: external-secrets.io/v1
kind: ExternalSecret
metadata:
  name: example
  namespace: example
spec:
  data:
    - secretKey: password    # key in the Kubernetes Secret
      remoteRef:
        key: example         # path under secret/, i.e. secret/example
        property: password   # field of that OpenBao secret
```

The Kubernetes Secret it creates gets the ExternalSecret's name; `target.name` is set only
where the two must differ.

**Each key is listed in `data`** (`secretKey` -> `key`/`property`), never pulled in with
`dataFrom` (user rule, 2026-10-20): git then shows every value a Secret holds and where it comes
from, and a field added to the OpenBao entry later does not land in the Secret - and in the
pods reading it - unnoticed.

**Every ExternalSecret sets `refreshInterval: 5m`** (user decision, 2026-10-06), so a value
changed in OpenBao reaches its Secret within five minutes - and through Reloader the pods that
read it at start. Without it the CRD's default of one hour applies; External Secrets has no
setting for a cluster-wide default, so the shared Component writes it into each one when
Kustomize renders - Helm-rendered ones (the WordPress chart, chart values) and directories
without a kustomization (`clusters/prod/etcd-snapshots`, the private repository's) still set
it themselves. The cost is negligible: about eight reads a minute against OpenBao, and External
Secrets rewrites a Secret only when its data changed, so an unchanged value restarts nothing. For an immediate refresh:
`kubectl --context d3strukt0r-prod-admin -n <namespace> annotate externalsecret --all force-sync=$(date +%s) --overwrite`.

**While OpenBao is unreachable nothing breaks**: a failed refresh leaves the Kubernetes Secret
as it is - neither emptied nor deleted - and only marks the ExternalSecret `Ready=False`
(`SecretSyncedError`); it retries on its own. Pods keep the last values; only new
ExternalSecrets and changed values wait until OpenBao is back.

## An ExternalSecret does not sync

Prometheus scrapes External Secrets (`serviceMonitor.enabled` in `values.yaml`), and the alert
**`ExternalSecretNotSynced`** (`rules.yaml`) fires when an ExternalSecret has not been `Ready`
for 15 minutes - three failed refreshes, so a short OpenBao restart stays quiet. Its Secret
still holds the last values; what is lost is that changes no longer arrive.

```shell
kubectl --context d3strukt0r-prod-admin get externalsecret -A | grep -v SecretSynced
kubectl --context d3strukt0r-prod-admin -n <namespace> describe externalsecret <name>   # the error
kubectl --context d3strukt0r-prod-admin -n openbao get pods                             # is OpenBao up?
```

Usual causes: OpenBao down or sealed (it unseals itself on start), a path or property in the
ExternalSecret that does not exist in OpenBao (a typo, or the value was never written with
`bao kv put`), or the store's login failing (`kubectl get clustersecretstore openbao`). Once
the cause is fixed, `force-sync` (above) syncs at once.

