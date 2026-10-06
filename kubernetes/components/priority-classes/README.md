# Priority classes

Back to [`kubernetes/README.md`](../../README.md).

The cluster's PriorityClasses (Application `priority-classes`, no finalizer). A pod names one in
`priorityClassName`; pods without one have priority 0.

## `stateful-core`

For the pods holding data everything else depends on: the shared MariaDB
([`../mariadb/README.md`](../mariadb/README.md)), the shared PostgreSQL
([`../postgres/README.md`](../postgres/README.md)) and OpenBao
([`../openbao/README.md`](../openbao/README.md)). Value 1000000 - far below k3s's own
`system-cluster-critical` and `system-node-critical` (2 billion), which stay on top.

- **What it changes: the eviction order.** When a node drops below the kubelet's memory
  threshold (`evictionHard` in [`ansible/roles/k3s/README.md`](../../../ansible/roles/k3s/README.md)),
  the kubelet evicts pods using more than their requests, lower priority first - so these go
  last. A pod within its requests is evicted only if nothing else frees enough.
- **What it does not change: scheduling.** `preemptionPolicy: Never` - one of these that does
  not fit on a node waits like any other pod and pushes nothing off. Today a drained node's
  pods do not all fit on the other two; with preemption every drain (kured, upgrades) could turn
  into a chain of evictions. Worth reconsidering once requests are realistic.
- **No finalizer**: a pod naming a class that does not exist is refused at creation, so
  removing the Application must not delete the classes.
- **Adding a workload**: only for state the rest of the cluster cannot do without. Everything
  in this class competes only with itself, so a crowded class protects nobody.

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin get priorityclass
kubectl --context d3strukt0r-prod-admin get pods -A -o custom-columns=NS:.metadata.namespace,POD:.metadata.name,PRIORITY:.spec.priorityClassName | grep stateful-core
```
