# CoreDNS

The cluster's DNS server is k3s's own: k3s deploys it from its packaged manifest
(`/var/lib/rancher/k3s/server/manifests/coredns.yaml`) and re-applies that manifest on every start
and upgrade. This component changes only two things about it:

- **Two replicas** (`replicas.yaml`). k3s's manifest sets no replica count, so Kubernetes runs one
  pod - and a drain of its node takes DNS away from the whole cluster. The manifest already spreads
  CoreDNS pods across nodes (`topologySpreadConstraints` on the hostname, `DoNotSchedule`), so two
  replicas land on two nodes.
- **A disruption budget** (`pdb.yaml`, `maxUnavailable: 1`): a drain evicts one pod only while the
  other is ready. Pods that are not ready may always be evicted (`unhealthyPodEvictionPolicy:
  AlwaysAllow`), so two broken pods do not block every drain - kured would wait without end.

Why: on 2026-11-09 kured drained prod-03 at 04:48 and evicted the only CoreDNS pod. Its
replacement on prod-04 waited 3 min 25 s for its image: the peer lookup (Spegel) answered 404, zot
could not fetch from Docker Hub without cluster DNS, and only containerd's last fallback, Docker
Hub from the node, worked. DNS was down until about 04:52, and everything resolving a name in
between failed (`mariadb-flush-binlogs`, for one). With two pods, the other one answers however
long the moved pod takes to start.

## How the replica count gets in

`replicas.yaml` is a **partial** Deployment: name, namespace, an annotation and `spec.replicas`.
The Application (`clusters/prod/coredns.yaml`) applies it server-side (`ServerSideApply=true`) and
compares on the server (`ServerSideDiff=true`), so Argo CD owns `spec.replicas`, its sync-options
annotation and its tracking annotation, and k3s keeps every other field. Since k3s's applied
state has no `replicas`, its re-applies leave the count alone - as long as k3s's manifest does
not start setting one. If a k3s upgrade ever adds it, k3s and Argo CD take turns: one pod is
removed at every k3s start and added back by self-heal; then the count has to move into a k3s
manifest of our own instead.

The trap: Argo CD treats the Deployment as its own. Without `Prune=false,Delete=false` on it,
removing `replicas.yaml` or the Application would delete CoreDNS, and so DNS. The Application has
no finalizer for the same reason.

If k3s ever recreates the Deployment (a manifest change it cannot apply in place), it comes back
with one replica; self-heal sets two again. A sync in the moment the Deployment is gone fails (a
partial object cannot be created), so the Application retries (`retry`, about 17 minutes).

## Runbook

Check that both pods run, on two nodes, and that the budget allows one disruption:

```shell
kubectl --context d3strukt0r-prod-admin -n kube-system get deploy coredns
kubectl --context d3strukt0r-prod-admin -n kube-system get pods -l k8s-app=kube-dns -o wide
kubectl --context d3strukt0r-prod-admin -n kube-system get pdb coredns
```

To stop managing the count (removing `replicas.yaml`): the Deployment keeps two replicas and
Argo CD's annotations, and the Application stays OutOfSync ("requires pruning", which
`Prune=false` refuses) until the annotations are gone:

```shell
kubectl --context d3strukt0r-prod-admin -n kube-system annotate deploy coredns argocd.argoproj.io/tracking-id- argocd.argoproj.io/sync-options-
```

With fewer than two schedulable nodes, the second pod stays Pending (the spread allows only one
per node) - expected on a single-node cluster, not on `prod`.
