# RabbitMQ operators

Back to [`kubernetes/README.md`](../../README.md).

The two operators behind the shared RabbitMQ broker, deployed by the Application
`rabbitmq-operators` (`kubernetes/clusters/prod/rabbitmq-operators.yaml`) into
`rabbitmq-system`:

- **Cluster Operator** (`cluster-operator/`, v2.23.3): runs the broker from a `RabbitmqCluster`
  object as a StatefulSet. The broker itself is `kubernetes/components/rabbitmq`.
- **Messaging Topology Operator** (`topology-operator/`, v1.20.6): turns `Vhost`, `User`,
  `Permission`, `Policy` and similar objects into the broker's definitions, so every app's vhost
  and user live in Git, in the app's own component. Its webhook certificate comes from
  cert-manager (the `-with-certmanager` manifest).

## Design

- **Pinned release manifests, not a chart**: RabbitMQ publishes no Helm chart for its operators
  (the chart people used was Bitnami's, whose free images are gone). Each manifest sits in its
  own directory because both declare the namespace `rabbitmq-system`, which Kustomize refuses
  twice - `topology-operator/namespace.yaml` deletes the second copy.
- **Both already meet the restricted Pod Security Standard**; the namespace is not exempt.
- **The topology operator trusts the broker's own CA**: the broker has no plain-text listener, so
  the operator declares vhosts and users over HTTPS. Its namespace carries the label
  `trust.d3strukt0r.dev/rabbitmq-ca` (`cluster-operator/namespace.yaml`), trust-manager puts the
  CA there as ConfigMap `rabbitmq-ca`, and `patches/topology-operator-ca.yaml` mounts it and adds
  its directory to Go's `SSL_CERT_DIR`. The volume is optional, so the operator starts before the
  ConfigMap exists. Tested in kind 2026-11-19: Vhost, User and Permission became Ready over HTTPS.
- **Resources** (`patches/resources.yaml`, both operators): no CPU limit (they throttle an idle
  node) and the cluster's default request of 50m instead of upstream's 200m and 300m; memory
  **48Mi / 128Mi** instead of upstream's 500Mi / 500Mi and 128Mi / 512Mi, which would have
  reserved most of the room a drained `prod-01` needs. Measured 2026-11-19 in a local kind
  cluster under restricted Pod Security, with a broker, a vhost, a user and a permission, loaded
  with as many Secrets and ConfigMaps as prod (about 170 each, 10 MiB of data - the operators
  keep an in-memory copy of what they watch): working set 16-37 MiB each, cgroup peak 60 MiB
  (Cluster Operator) and 66 MiB (topology operator); the limit is that peak ÷ 0.8 with room for
  prod's larger object count. Re-sized by the rule once prod has a week of data.
- **No Application finalizer**: the app brings 14 CRDs, and deleting a CRD deletes every object of
  its kind - the broker and every app's definitions.

## Upgrading

Bump the version in the operator's `kustomization.yaml` after reading its release notes, render
(`kubectl kustomize kubernetes/components/rabbitmq-operators`) and diff against the current one.

- **A Cluster Operator upgrade restarts the broker** when the new version changes the
  StatefulSet it generates (its release notes say so). To choose the moment, pause the broker's
  reconciliation first (`rabbitmq.com/pauseReconciliation: "true"` on the `RabbitmqCluster`),
  upgrade, then remove the label at a quiet time.
- **The Cluster Operator's default broker image lags** (v2.23.3 defaults to RabbitMQ 4.3.4); the
  broker's image is set in `kubernetes/components/rabbitmq`, independent of the operator.
