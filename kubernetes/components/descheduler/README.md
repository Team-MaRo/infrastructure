# descheduler

Back to [`kubernetes/README.md`](../../README.md).

Kubernetes places a pod once and never moves it. After a night with reboots, the pods kured's
drains moved stay where they landed, and the last node rebooted comes back holding little more
than its DaemonSets. The descheduler (kubernetes-sigs, `kubernetes/clusters/prod/descheduler.yaml`,
the chart pinned there, values in this directory, namespace `descheduler`) runs once a morning
and evicts pods from full nodes; the scheduler, which prefers the emptiest node, puts their
replacements on the empty one.

## What it does

- **A CronJob at 06:30 Zurich time** (`timeZone: Europe/Zurich`), after kured's window
  (04:30-06:00, [`../kured/README.md`](../kured/README.md)) with room for a drain that started
  just before 06:00 - see "The night's maintenance order" in [`AGENTS.md`](../../../AGENTS.md).
  It looks at the nodes once, evicts, and exits. Never during the day.
- **Only `LowNodeUtilization`.** The values replace the chart's profile, which would also
  evict for duplicates, restarts, affinity and taints. It counts **requests**, as the scheduler
  does, not live usage: a node below 20 % in all of CPU, memory and pod count receives pods; a
  node above 50 % in any of them gives pods away. **Without a node below 20 % it does nothing**,
  so on a morning without reboots it is a no-op. A cordoned node (one still rebooting) never
  counts as one that receives pods.
- **At most 10 evictions per run** (`maxNoOfPodsToEvictTotal`), should the thresholds ever
  misjudge the nodes.
- **Dry run for now** (`dry-run: true`): it only logs what it would evict, until a run's log
  has been checked.

## What it never evicts

- **`stateful-core` pods** (`priorityThreshold`): the shared MariaDB, PostgreSQL, OpenBao and
  valkey-state ([`../priority-classes/README.md`](../priority-classes/README.md)). An evicted
  MariaDB primary forces an unclean failover and a rebuild ([`../mariadb/README.md`](../mariadb/README.md)).
- **Pods with a volume** (`PodsWithPVC`): Prometheus, Loki and the databases stay, no volume
  re-attach. Pods with an emptyDir do move (`PodsWithLocalStorage` disabled) - nearly every pod
  has one.
- **System-critical pods** (CoreDNS, Traefik, metrics-server), DaemonSet pods and pods without a
  controller (CloudNativePG's instances among them) - the descheduler's own defaults.
- **A pod that would not fit elsewhere** (`nodeFit`).
- **Beyond a PodDisruptionBudget**: evictions go through the Eviction API, like kured's drain.

A single-replica app is down from its eviction until its new pod is Ready - seconds to a minute,
the same gap the night's drain gave it. Gatus alerts only after three failed checks, so this
stays quiet.

## Design

- **No CPU limit; memory from the namespace's defaults** (Kyverno's LimitRange, 32Mi / 128Mi)
  instead of the chart's 500m CPU limit and 256Mi: not measured yet, sized by the rule later.
- **`seccompProfile: RuntimeDefault`** in the pod's security context: the restricted Pod Security
  Standard requires it and the chart leaves it out.
- **No priority class** instead of the chart's `system-cluster-critical`: a job that runs once a
  day needs none.
- **No metrics, no alert of its own**: in CronJob mode it serves no metrics; a failed run raises
  kube-prometheus-stack's default `KubeJobFailed`.

## Reading what it did

Its log names every node's usage, which nodes it counted as under- or overutilized, and every
pod it evicted (or would have, in a dry run); each eviction also leaves an Event with reason
`Descheduled` on the pod. In Grafana's Explore with Loki: `{namespace="descheduler"}`.

```sh
kubectl --context d3strukt0r-prod-admin -n descheduler get jobs
kubectl --context d3strukt0r-prod-admin -n descheduler logs job/<job name>
kubectl --context d3strukt0r-prod-admin get events -A --field-selector reason=Descheduled
```

## Running it by hand

After a drain during the day (a resize, a manual reboot) there is no need to wait for the
morning:

```sh
kubectl --context d3strukt0r-prod-admin -n descheduler create job --from=cronjob/descheduler descheduler-manual-1
kubectl --context d3strukt0r-prod-admin -n descheduler logs -f job/descheduler-manual-1
kubectl --context d3strukt0r-prod-admin -n descheduler delete job descheduler-manual-1
```

The last line removes the Job afterwards; the CronJob's own Jobs are cleaned up by its history
limit.

## Removing it

The finalizer removes everything the Application deployed, namespace included. Pods it moved
stay where they are.
