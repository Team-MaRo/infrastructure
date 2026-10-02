# trust-manager

Puts the databases' CA certificates into the namespaces of the apps that verify them
(`kubernetes/clusters/prod/trust-manager.yaml`, the chart pinned there, values and manifests in
this directory, namespace `trust-manager`). From the cert-manager project; it needs cert-manager
for its webhook's certificate. How the cluster's manifests fit together is in
[`kubernetes/README.md`](../../README.md).

## Why

Apps connect to the shared PostgreSQL and MariaDB with TLS and verify the server's certificate
(`verify-full`), so each needs the operator's CA certificate - and a pod can only mount what is in
its own namespace. trust-manager keeps one ConfigMap per CA in every namespace that asks for it,
updated when the CA rotates, instead of a copy per app.

## Design

- **Two Bundles, one per database** (`bundles.yaml`), so an app trusts only the database it uses:

  | Bundle = ConfigMap | Key | From | Namespace label |
  |---|---|---|---|
  | `postgres-ca` | `ca.crt` | CloudNativePG's Secret `postgres-ca` | `trust.d3strukt0r.dev/postgres-ca: "true"` |
  | `mariadb-ca` | `ca.crt` | mariadb-operator's Secret `mariadb-ca-bundle` | `trust.d3strukt0r.dev/mariadb-ca: "true"` |

- **Sources only from its trust namespace.** trust-manager cannot read sources from another
  namespace, so `ca-copies.yaml` copies the two certificates into `trust-manager` once - External
  Secrets' Kubernetes provider, with a ServiceAccount that may read exactly those two Secrets, and
  only their `ca.crt` (CloudNativePG's Secret also holds the CA's private key). trust-manager reads
  only the named key and writes the certificates out again from what it parsed, refusing
  anything that is not a certificate.
- **Its own namespace as trust namespace** (`app.trust.namespace`), not the default
  `cert-manager`: trust-manager may read every Secret in its trust namespace, and `cert-manager`
  holds the Cloudflare tokens and the ACME account keys.
- **ConfigMaps, not Secrets**: a CA certificate is public, and Secret targets would need
  cluster-wide rights on Secrets.
- **No Application finalizer**: the app brings the Bundle CRD, and the ConfigMaps belong to their
  Bundle - deleting a Bundle deletes its ConfigMap in every namespace, and pods that mount it no
  longer start. So the Bundles stay in this Application and are never removed casually.
- **Rotation**: CloudNativePG renews its CA (valid 90 days) 7 days before it expires, with the same
  key, so the old certificate keeps verifying meanwhile; mariadb-operator keeps old and new in its
  bundle during a rotation. The copy follows within External Secrets' refresh (an hour),
  trust-manager at once, and a mounted ConfigMap reaches running pods within about a minute
  (not with `subPath`).
- Chart v0.25.0 meets the restricted Pod Security Standard as it is; the debian default-CA package
  is off. Metrics through `service-monitor.yaml`. The webhook's certificate comes from a
  self-signed Issuer the chart creates.
- **Watch**: `Bundle` (`trust.cert-manager.io/v1alpha1`) is the current API; a later release will
  replace it with `ClusterBundle` (renamed fields, migration by the controller) - read the release
  notes before an upgrade. trust-manager#1119 reports v0.25.0 still listing Secrets in
  `cert-manager` with a different trust namespace; if its log shows that, pin v0.24.0.

## Giving an app a CA

1. The label on its `namespace.yaml` (table above).
2. A volume from the ConfigMap - `configMap: {name: postgres-ca}` (or `mariadb-ca`) - mounted as a
   directory, and the client pointed at `<mountPath>/ca.crt`.

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin get bundles.trust.cert-manager.io
kubectl --context d3strukt0r-prod-admin get configmaps -A -l trust.cert-manager.io/bundle
kubectl --context d3strukt0r-prod-admin -n <namespace> get configmap postgres-ca -o jsonpath='{.data.ca\.crt}' | openssl x509 -noout -subject -enddate
```
