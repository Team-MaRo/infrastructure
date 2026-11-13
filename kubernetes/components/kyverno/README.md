# Kyverno

Back to [`kubernetes/README.md`](../../README.md).

`kubernetes/clusters/prod/kyverno.yaml` deploys the `kyverno` Helm chart (patch range 3.9.x =
Kyverno v1.19) with `kubernetes/components/kyverno/` as values and extra resources - the
same two-source pattern as OpenBao, plus a third source for the directory's manifests. It holds
two policies: `default-resources` (`default-resources.yaml`) gives every namespace but a few
system ones a LimitRange, so a container without `resources` of its own still has requests and
limits; `allowed-registries` (`allowed-registries.yaml`) refuses images from unknown
registries. `rbac-limitranges.yaml` lets Kyverno create LimitRanges.

## Running Kyverno under Argo CD

- **Argo CD settings Kyverno needs**, from Kyverno's own notes: `ServerSideApply=true`
  (huge CRDs), `ServerSideDiff=true,IncludeMutationWebhook=true` on the Application,
  `config.preserve: false` (the default marks Kyverno's ConfigMap `helm.sh/resource-policy:
  keep`, which would orphan it when the Application is removed; Kyverno's notes still
  describe an older post-delete hook, which the 3.9 charts no longer render),
  and `ignoreAggregatedRoles: true` in `argocd-cm`, since aggregated ClusterRoles never
  match git. Argo CD's own defaults already exclude Kyverno's report kinds.
- Kyverno is scraped through its chart's `serviceMonitor.enabled` per controller.
- **Memory per controller** (`values.yaml`), over 2026-10-23..11-02, the request from real memory's
  p95, the limit from the working set's p95 and peak: admission 96Mi / 224Mi (real p95 89 MiB; 97,
  101 MiB), background 96Mi / 224Mi (86; 110, 120 MiB), cleanup 48Mi / 160Mi (45; 68, 72 MiB),
  reports 96Mi / 224Mi (89; 105, 112 MiB). The admission controller's is under
  `admissionController.container`, the others' directly under the controller.
- **Nodes are not filtered out** (`config.resourceFiltersExclude: ["[Node,*,*]"]`, 2026-10-18):
  Kyverno's default list ignores Nodes, but kube-prometheus-stack's `network-policy.yaml`
  generates a NetworkPolicy per node from them (Traefik's access to Prometheus). Only the Node
  itself was taken off the list; its subresources, above all the status every node reports
  every few seconds, stay ignored, so Kyverno only sees a node being added, changed or removed.
- **Other components' GeneratingPolicies** bring their own aggregated ClusterRole for what they
  generate (as `rbac-limitranges.yaml` does here): kube-prometheus-stack's for NetworkPolicies.

## Default resources

- **Defaults, never a max.** Requests CPU `50m`, memory `32Mi`, ephemeral-storage `50Mi`;
  limits memory `128Mi`, ephemeral-storage `1Gi`. An app that needs more sets `resources`
  on its containers, which always wins. The ephemeral-storage limit keeps a container from
  filling a node's 40 GB disk, which holds etcd too.
- **No CPU limit, on purpose.** A CPU limit throttles a container even while the node is
  idle; requests already share CPU fairly under contention.
- **The default request is 32Mi** (64Mi until 2026-11-02): most containers use less, and a
  request no one uses still holds back a node's memory from the scheduler.
- **A container sets its memory itself where the defaults do not fit** - more than 32 MiB of real
  memory (p95) or a limit above 128Mi, the latter with a request of 32Mi written out - and
  everywhere in `kube-system` and `system-upgrade`, which have no LimitRange; through a patch or
  Helm value in its component. Where the defaults fit, nothing is set, so there is nothing to keep
  in step. Without a request the scheduler reserves nothing for it (Argo CD's application controller
  uses 1.2-1.6 GB), and without a limit a leak can take the node's memory. Only what k3s itself
  deploys (CoreDNS, metrics-server, its Helm install Jobs) is left as k3s sets it.
- **How the values are sized** (user decision, 2026-11-02), from Prometheus over a window of
  days, not hours (2026-10-23..11-02 for the first ones). The request is the p95 of real memory,
  `container_memory_working_set_bytes - container_memory_total_active_file_bytes` - anonymous,
  shared and kernel memory, what the kernel cannot reclaim without swap (PHP's OPcache,
  Postgres' `shared_buffers` included) - rounded up to 16 MiB. The limit is twice the working
  set's p95 or the highest working set ever seen x 1.1 rounded up to 32 MiB, whichever is
  higher, so the file cache keeps its room up to the limit. Grafana's dashboard "Memory sizing"
  computes both for every container
  ([`../kube-prometheus-stack/README.md`](../kube-prometheus-stack/README.md)). A comment next to
  the value gives the real-memory p95, the working set's p95 and peak, and the window they come
  from. A
  container that holds data by design sizes its limit from that design instead, the derivation
  in the comment (Traefik: up to 404 MiB of upload bodies for the upload scan, plus one being
  joined). An OOM kill means raising that limit - `kubectl --context d3strukt0r-prod-admin get pod`
  shows the restarts, `kubectl --context d3strukt0r-prod-admin describe pod` the `OOMKilled`
  reason.
- **Requests from real memory, not the working set** (user decision, 2026-11-02): file cache
  only fills memory that is free, and the kernel hands it back when a process needs it, so
  nothing has to be reserved for it. The real-memory p95s sum to 10.1 GiB, the working set's to
  12.6 GiB; MariaDB alone went from 935 to 604 MiB. The caveat: the kubelet ranks pods for
  eviction by the working set, cache included, against their request, so a cache-heavy pod
  counts as above its request first - but only when a node really runs out of memory.
- **p95, not p99** (user decision, 2026-11-02): each container's p99 reserves its own rare peak,
  and they never peak together. The p95s summed (12.2 GiB) match the most the pods ever used at
  once (12.9 GiB), while p99 x 1.1 booked about 15 GiB. A container above its request is neither
  slowed nor killed; it only goes first if a node runs out of memory.
- **Only four namespaces are left out** (the policy's `matchConditions`): `kube-system`,
  whose pods k3s manages; `system-upgrade`, whose k3s upgrade Jobs set no memory and must not
  be killed halfway through replacing the binary; and the pod-less `kube-public` and
  `kube-node-lease`. Infrastructure is covered like any app - a component that needs more
  than the defaults sets its own values, which always win. `kyverno` gets none regardless:
  Kyverno's own resource filter (`[*/*,kyverno,*]` in its ConfigMap) ignores its namespace,
  and its chart sets every container's resources itself. **This list and
  `k3s_psa_exempt_namespaces`** (see "Pod Security" in [`AGENTS.md`](../../../AGENTS.md))
  **are independent**: one is about default resources, the other about privileges.
- **`GeneratingPolicy`, not `ClusterPolicy`**, which is deprecated since Kyverno's CEL-based
  policy types. `generateExisting` covers namespaces created before the policy;
  `synchronize` keeps the LimitRange in step with the policy and removes it with its
  namespace - hand edits to it are reverted. **The policy matches `UPDATE` too, not only
  `CREATE`**: with `synchronize`, Kyverno re-checks it on every change to a namespace (an Argo CD
  sync of its `namespace.yaml`, a label) and deletes the LimitRange when the change no longer
  matches. With `CREATE` alone that happened on every such change; the hourly `generateExisting`
  scan brought it back, but pods started in between ran without any requests or limits.
- **Kyverno needs RBAC for what it generates.** Its background controller's own ClusterRole
  covers only a few kinds; `rbac-limitranges.yaml` adds LimitRanges through the aggregation
  label `rbac.kyverno.io/aggregate-to-background-controller`. Generating another kind means
  another such role.
- The policy was tested offline with the Kyverno CLI (`kyverno apply <policy> --resource
  <namespaces>`), which is the quickest way to try a change before pushing.

### Checking the defaults

Kyverno gives every namespace a LimitRange `default-resources`. A container that sets no
`resources` gets requests of 50m CPU, 32Mi memory and 50Mi ephemeral storage, and limits of
128Mi memory and 1Gi ephemeral storage - no CPU limit. Setting `resources` on a container
overrides any of them; there is no maximum.

```shell
kubectl --context d3strukt0r-prod-admin get limitrange -A
kubectl --context d3strukt0r-prod-admin get generatingpolicy default-resources
```

Only `kube-system`, `system-upgrade`, `kube-public` and `kube-node-lease` are left out
(`components/kyverno/default-resources.yaml`); there, every container sets its memory
itself. `kyverno` gets none either - Kyverno ignores its own namespace - but its chart sets
all resources. A new infrastructure component needs no entry here - only its own
`resources` where the defaults do not fit, and a place in `k3s_psa_exempt_namespaces` in
Ansible if it needs more than the restricted standard allows.

## Allowed registries

Pod Security cannot say where an image comes from, so that is Kyverno's:
`kubernetes/components/kyverno/allowed-registries.yaml`, a `ValidatingPolicy` in `Deny`
mode. In every namespace, infrastructure included, every container, init
container and ephemeral container must come from `docker.io`, `ghcr.io`, `quay.io`,
`registry.k8s.io`, `public.ecr.aws`, `us-docker.pkg.dev` or our own `registry.d3strukt0r.dev`
([`../zot/README.md`](../zot/README.md)) - whole hosts, not single organisations (user
decision; `us-docker.pkg.dev` is Google's Artifact Registry, where Fairwinds publishes
Goldilocks and nothing else of ours comes from); anything else is refused when the Deployment - or whichever controller - is
applied. Kyverno itself is set to pull from `ghcr.io` (`global.image.registry` in its
values) instead of its default `reg.kyverno.io`.

- **`kube-system` and `kyverno` are never checked**: Kyverno's own webhook configuration
  (its `config.webhooks` default) keeps them out, so a broken Kyverno cannot block CoreDNS,
  the CSI driver or its own restart. Their images happen to comply anyway.
- **`failurePolicy: Ignore`**: with Kyverno down, pods start unchecked instead of not at all.
  `Fail` would also stop Argo CD from restarting - the tool that repairs Kyverno - and need
  Kyverno's webhook configuration deleted by hand. Images only come from git, so the check
  guards against mistakes, not against someone timing a Kyverno outage.
- **A chart update that moves its images to another registry is refused** at sync, and Argo
  CD shows the error: either the chart gets a registry override, as Kyverno did, or the
  registry is added to the list.
- **Docker Hub is `index.docker.io` in the list**, not `docker.io`: Kyverno's CEL image
  library reports that registry for `nginx`, `rancher/x` and `docker.io/x` alike.
- **Hosts match exactly** (`docker.io.evil.com` is refused), and a reference the library
  cannot parse is refused too - `evil.example.com/x`, with a host but a single path
  segment, is one.
- **autogen** covers Deployments, StatefulSets, DaemonSets, Jobs and CronJobs, so a bad
  image is refused when the controller is applied, not later as a pod that never appears.
- Adding a registry is one entry in the `allowed` variable in
  `components/kyverno/allowed-registries.yaml`. Test a change offline first:
  `kyverno apply <policy> --resource <pods and deployments>`.

## Routes only through Cloudflare

Every proxied route puts the Middleware `cloudflare-only` first, so only Cloudflare reaches it
([`../../charts/cloudflare-only/README.md`](../../charts/cloudflare-only/README.md)). A new route
that forgets it would be reachable around Cloudflare by anyone who knows a node's address, which
the DNS-only names publish. `components/kyverno/cloudflare-only-routes.yaml`, a
`ValidatingPolicy` in `Deny` mode, refuses it when it is applied:

- **An Ingress** must name `<namespace>_cloudflare-only@kubernetescrd` first in
  `traefik.ingress.kubernetes.io/router.middlewares`; **an IngressRoute** must name
  `cloudflare-only` first in every route's `middlewares`. First, so nothing else (a login gate, a
  rate limit) runs for a request from outside Cloudflare.
- **A route for a DNS-only name** (the registry, later Forgejo) carries the label
  `d3strukt0r.dev/dns-only: "true"` and is not checked. The label is the visible decision that
  this route is meant to be reachable directly.
- **`kube-system` and `kyverno` are not checked**, as for the registries - the Traefik
  dashboard's IngressRoute in `kube-system` carries the middleware all the same. TCP routes
  (`IngressRouteTCP`) are not checked either: they carry no HTTP middlewares.
- **`failurePolicy: Ignore`**, for the same reason as the registries: routes only come from git.
- Tested offline with the CLI of the running version (2026-10-28: the middleware second, missing
  or from another namespace refused; labelled skipped; all 23 live routes passed):
  `docker run --rm -v "$PWD:/w" -w /w ghcr.io/kyverno/kyverno-cli:v1.19.1 apply <policy> --resource <routes>`.
