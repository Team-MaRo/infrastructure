# vertical-pod-autoscaler

The VPA recommender: Kubernetes' own estimate of the memory and CPU each container should request
(`kubernetes/clusters/prod/vertical-pod-autoscaler.yaml`, the chart pinned there, values in this
directory, namespace `vertical-pod-autoscaler`). From the Kubernetes autoscaler project. It is a
second opinion next to our own sizing rule (Grafana's dashboard "Memory sizing",
[`../kube-prometheus-stack/README.md`](../kube-prometheus-stack/README.md)); nothing acts on what
it says. How the cluster's manifests fit together is in [`kubernetes/README.md`](../../README.md).

## Design

- **Recommender only.** VPA has three parts; the updater (evicts pods to apply new requests) and
  the admission controller (rewrites pods as they start) are switched off, so no pod is ever
  changed. Requests stay in git and Argo CD's self-heal would undo anything else.
- **It recommends for VPA objects only**, one per workload, with `updateMode: Off`. Goldilocks
  creates them. **It tracks every container** (no `--memory-saver`), so a workload that gets a VPA
  later already has its history.
- **Its formula is not ours**: target = p90 of each day's peak over about 8 days (a decaying
  histogram, half-life 24 h) + 15 %; lower bound p50, upper bound p95, both with the 15 %. Our rule
  is the p95 of real memory (working set minus active file cache); VPA reads the working set,
  file cache included, so its numbers run higher for containers that read many files.
- **Lower floors**: upstream rounds every recommendation up to 250 MB and 25m per pod, which
  would put each small container at the floor. Set to 16 MB and 10m.
- **History in checkpoints, not Prometheus.** The recommender keeps its history in
  `VerticalPodAutoscalerCheckpoint` objects and needs about 8 days for steady numbers. Prometheus
  as the history source would start full, but it ignores the history of every VPA whose
  workload's selector has a label key with `.` or `/` (`app.kubernetes.io/name`, so nearly all
  charts) - kubernetes/autoscaler#10476, open.
- **One replica, no PDB**: numbers no one acts on automatically need no second copy, and a PDB
  over a single replica would block every node drain.
- **Restricted Pod Security** is added in the values: the chart sets only `runAsNonRoot` and the
  UID.
- **Logs at `-v=0`**: errors, warnings and plain info. From `1` on (the chart sets `4`) it logs
  ~23 lines a minute removing empty aggregates (`V(1)` in 1.8.1), about 33,000 a day into Loki.
- **Memory 64Mi / 256Mi, not measured yet** (upstream requests 500Mi for large clusters); size it
  by the rule after a week.
- **No finalizer**: the chart brings the CRDs `verticalpodautoscalers` and
  `verticalpodautoscalercheckpoints`, and deleting them would delete every VPA object. The chart
  also marks them `helm.sh/resource-policy: keep`.

## Open bugs that bear on the numbers

- kubernetes/autoscaler#10674: memory percentiles can come out too low after a histogram bucket
  empties.
- kubernetes/autoscaler#10211: aggregates holding only memory samples are garbage-collected, so
  short-lived pods (CronJobs) lose their history - Goldilocks leaves Jobs and CronJobs out.
- kubernetes/autoscaler#9891: a checkpoint is dropped when its workload cannot be resolved for a
  moment, and the memory recommendation starts over.

## Reading a recommendation

```sh
kubectl --context d3strukt0r-prod-admin get vpa -A
kubectl --context d3strukt0r-prod-admin -n <namespace> describe vpa <name>
```

`Status -> Recommendation` per container: `Target` (the suggested request), `Lower Bound` and
`Upper Bound`, `Uncapped Target` (before any bounds set on the VPA object). Goldilocks shows the
same numbers per namespace.
