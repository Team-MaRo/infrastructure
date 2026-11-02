# mariadb-operator

The community mariadb-operator, which runs the apps' database
(`kubernetes/clusters/prod/mariadb-operator*.yaml`, charts pinned, values in this directory): a
primary and one replica with automatic failover, on 2 volumes (user decision, 2026-09-28). The
instance itself, its backups and runbooks are in [`../mariadb/README.md`](../mariadb/README.md).
How the cluster's manifests fit together is in [`kubernetes/README.md`](../../README.md).

## Design

- **Why not Galera** (Percona XtraDB Cluster, MariaDB Galera): Galera decides by majority vote,
  so 2 members lose the vote when one crashes and the survivor stops accepting writes. That
  needs 3 volumes, or a data-less arbitrator (garbd), which no maintained operator supports.
  Replication instead lets the operator, through the Kubernetes API, pick the new primary.
- **The known weak spot, accepted:** on a hard node loss the failover can hang until the node
  returns (17 minutes seen on k3s), and a node that stays dead needs manual steps
  ([mariadb-operator#1628](https://github.com/mariadb-operator/mariadb-operator/issues/1628)).
  The MariaDB instance's settings, alert and runbook mitigate it (see
  [`../mariadb/README.md`](../mariadb/README.md)).
- **The CRDs are their own Application, which never prunes.** Deleting a CRD deletes every
  resource of its kind, so a MariaDB and its pods would go with it. The chart is 800 KB
  (`ServerSideApply=true` required). Bump both Applications together.
- **The webhook's certificate comes from cert-manager** (a self-signed Issuer of the chart's
  own; an ACME issuer cannot certify `.svc` names), so the chart's cert-controller Deployment
  is not rendered. The chart has no Helm hook Jobs.
- **The operator and its webhook each run twice, on different nodes** (`ha`, `webhook.ha`,
  required anti-affinity, PDBs). The operator performs the failover, so a single replica on
  the primary's node would die with it and the failover would wait about five minutes for
  Kubernetes to move it; with two, the standby takes over once the leader's 15-second lease
  runs out. Every change to a MariaDB passes the webhook with `failurePolicy: Fail`, the
  failover's own included: in the first failover test (2026-09-28, the primary's node powered
  off) the single webhook pod died with the node, the failover could not write, and it hung
  until the node was declared gone with the out-of-service taint.
- **Restricted Pod Security needs explicit securityContexts** - the chart sets none. The
  images the operator deploys are written fully qualified in `config.*` (docker.io, quay.io),
  so the registry check does not depend on short-name resolution.
- Kubernetes 1.37 is not yet in the operator's CI (1.36 when adopted).
- **Memory** (`values.yaml`): the operator 128Mi / 256Mi, since it watches every namespace; the
  webhook 48Mi / 96Mi (p99 41 MiB, peak 43 MiB over 2026-10-23..11-02), above the default
  request of 32Mi.

## The operator is stuck

The alert `ControllerReconcileStuck` with `namespace: mariadb-operator` means the leader has been
working on one item for over 15 minutes. Until it finishes, nothing changed in a MariaDB reaches
its pods and the operator performs no failover. `kubectl get mariadb` still shows `Running`;
the sign is a change from git that stays in the MariaDB object but never reaches the
StatefulSet. The log stays empty at `INFO`, and a pass that hangs reports no error.

Restart the operator - its two pods only; the MariaDB pods keep running, and the new leader
runs a fresh pass that applies whatever was waiting:

```shell
kubectl --context d3strukt0r-prod-admin rollout restart deployment/mariadb-operator -n mariadb-operator
```

Seen so far (26.10.1): from 2026-10-29 10:15 every pass of the `mariadb` queue hung for about
3.3 hours, finished, and the next one hung again within about 10 minutes; before that twice in
the night of 2026-10-27 (up to 2.4 hours). Noticed on 2026-11-02, when the agent's new memory
values did not arrive; the restart cleared it. The cause is unknown - if it returns, take the
leader's log at `--log-level=DEBUG` during the hang before restarting, for an upstream issue.
