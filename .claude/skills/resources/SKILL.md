---
name: resources
description: Report and evaluate resource usage of the prod cluster - per node (memory, what runs outside pods, requests vs allocatable, CPU, disk, whether a drain fits) and per container (request and limit against the repo's sizing rule, peaks, VPA's opinion, CPU, restarts, OOM risk) - as tables plus an evaluation with concrete recommendations. Use it whenever the user asks about memory, CPU, disk or capacity ("how's the cluster on memory", "is prod-02 full", "what uses the most", "can we remove prod-04", "is X sized right", "why was Y OOM-killed"), before resizing or removing a node, before changing a container's memory, or when Grafana's "Memory sizing" or Goldilocks prompt a question.
argument-hint: "[namespace|node] [--window 14d]"
---

# Resources

The cluster's sizing rule is in AGENTS.md ("Default resources come from Kyverno's LimitRange"):
a container's memory **request** is the p95 of its real memory (working set minus active file
cache) rounded up to 16 MiB; its **limit** is its highest *expected* working set ÷ 0.8 (x 1.25) - so that peak sits at 80 %, where `ContainerMemoryNearLimit` warns -, rounded
up to 32 MiB - a peak the software genuinely has, never a leak or a bug, which the limit is there
to reveal. No CPU limits anywhere. Grafana's "Memory
sizing" dashboard shows the same rule over a chosen window; Goldilocks shows VPA's opinion. This
skill puts both into a table here and adds an evaluation.

## Run

```sh
.claude/skills/resources/scripts/resources.py [namespace|node] [--window 14d]
```

Read-only, about 6 s for the whole cluster (its window queries sample every 30 min - every 5 min loaded Prometheus by a gigabyte). A namespace (`monitoring`) narrows the container
table to it; a node (`prod-02`) narrows both tables to that node and the containers on it. The
container figures come from the recording rules in
`kubernetes/components/kube-prometheus-stack/memory-sizing.yaml`, recorded since 2026-11-13 - the
header says how many days exist; until the window is covered, p95 and max are from fewer days, so
say so in the evaluation and prefer caution over lowering anything.

## The tables

**Nodes** (MiB):

| Column | Meaning |
|---|---|
| USED NOW / USED MAX | MemTotal - MemAvailable now, and its highest over the window - what the kernel cannot hand out |
| PODS WS | the pods' working sets on the node |
| OUTSIDE PODS | USED - PODS WS: k3s, etcd, containerd, the OS. About 3 GB on a server (prod-01..03, etcd), under 1 GB on the worker. Not split per process (those series are not scraped) |
| REQUESTS/ALLOCATABLE | the scheduler's view: what pods asked for, of what the kubelet offers. `kubectl top nodes` shows memory % of allocatable too, so it reads higher than USED |
| CPU NOW/P95 | cores busy now and at p95, of the node's cores |
| DRAIN FITS | whether the other nodes' free allocatable takes this node's requests - what a kured reboot or a drain needs |

**Containers** (MiB; one row per workload and container, replicas folded by max):

| Column | Meaning |
|---|---|
| REQUEST, LIMIT | set now |
| REAL P95, WS P95, WS MAX | over the window, from the 30 s samples |
| PEAK | the peak the limit is built on: `k` the kernel's own (`container_memory_max_usage_bytes`, cgroup memory.peak), which no scrape misses - short spikes run 10-30 % above WS MAX; `s` the sampled WS MAX where the kernel's peak sits at the limit, which is file cache filling the container (Prometheus, Loki, Tempo, Postgres) and says nothing about the need |
| RULE REQ/LIM | what the sizing rule gives - the limit from PEAK as if that peak were expected; the evaluation decides whether it is |
| VPA | VPA's target (Goldilocks' source) - a second opinion with its own formula (`kubernetes/components/vertical-pod-autoscaler/README.md`); `-` where it has none (operator-owned pods) |
| CPU P95/MAX | cores |
| RESTARTS | in the window |
| FLAG | `oom-killed` (last termination), `over-limit-risk` (PEAK above 80 % of the limit - beyond it the near-limit alert fires), `under-requested` (request below the rule), `over-requested` (request above twice the rule), `ok` |

## The evaluation (what to add on top)

Answer the user's question first, then in this order, short:

1. **Can the cluster lose a node?** Any `DRAIN FITS: NO` is the headline - a reboot night or a
   failure would leave pods Pending. Then nodes whose USED MAX comes near their total (memory
   pressure evicts pods before it OOM-kills the node).
2. **Containers at risk of being OOM-killed**: `oom-killed` and `over-limit-risk`. First decide
   whether WS MAX is an **expected** peak - something the software genuinely does (a backup, a
   heavy query or trace lookup, wp-cli at a start; say what it is) - or a sign of trouble: real
   memory climbing for days without coming back down (a leak), a jump after an upgrade, a spike
   with a known bug behind it. Only an expected peak becomes the limit (÷ 0.8); trouble is
   reported for investigation and the limit stays, because revealing it is the limit's job. Look
   at the container over time (Grafana "Memory sizing", one workload; or a range query on
   `workload_container:memory_real:max_bytes`) before calling it either. A container that holds
   data by design (Traefik with upload-scan bodies) is sized from that design, not the rule - its
   README says so.
3. **Under-requested**: the scheduler thinks the node has room it lacks; the rule's request.
4. **Over-requested**: memory reserved but unused; the rule's request. Lower only with a full
   window of data, never from a few days.
5. CPU only when something is busy (p95 well above its request, or a node's CPU above ~70 %).

Recommendations are concrete values and the file they go in (`components/<app>/values.yaml`, a
manifest's `resources:`), with the derivation as the comment the repo writes next to them
("Over <dates>: real p95 X, the request; working set peak Z (what it was) ÷ 0.8, the limit"). Nothing is
changed without the user's go; a change follows the component's README and the infrastructure
rules (plan, render, commit on the user's word).
