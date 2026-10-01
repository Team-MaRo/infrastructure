# Kyverno

Back to [`kubernetes/README.md`](../../README.md).

`kubernetes/clusters/prod/kyverno.yaml` deploys the `kyverno` Helm chart (pinned, 3.9.1 =
Kyverno v1.19.1) with `kubernetes/components/kyverno/` as values and extra resources - the
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
  describe an older post-delete hook, which chart 3.9.1 no longer renders),
  and `ignoreAggregatedRoles: true` in `argocd-cm`, since aggregated ClusterRoles never
  match git. Argo CD's own defaults already exclude Kyverno's report kinds.
- Kyverno is scraped through its chart's `serviceMonitor.enabled` per controller.

## Default resources

- **Defaults, never a max.** Requests CPU `50m`, memory `64Mi`, ephemeral-storage `50Mi`;
  limits memory `128Mi`, ephemeral-storage `1Gi`. An app that needs more sets `resources`
  on its containers, which always wins. The ephemeral-storage limit keeps a container from
  filling a node's 40 GB disk, which holds etcd too.
- **No CPU limit, on purpose.** A CPU limit throttles a container even while the node is
  idle; requests already share CPU fairly under contention.
- **A container sets its memory itself where the defaults do not fit** - more than 64Mi
  used or a limit above 128Mi - and everywhere in `kube-system` and `system-upgrade`, which
  have no LimitRange; through a patch or Helm value in its component. Where the defaults fit,
  nothing is set, so there is nothing to keep in step. Without a request the scheduler reserves nothing for it (Argo CD's application controller uses
  1.2-1.6 GB), and without a limit a leak can take the node's memory. The values were set on
  2026-09-28 from Prometheus: the request about the usual use, the limit about twice the
  highest seen, 128Mi at the least. They rest on a few hours of data, so an OOM kill means
  raising that limit - `kubectl get pod` shows the restarts, `kubectl describe pod` the
  `OOMKilled` reason. Only what k3s itself deploys (CoreDNS, metrics-server, its Helm install
  Jobs) is left as k3s sets it.
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
  namespace - hand edits to it are reverted.
- **Kyverno needs RBAC for what it generates.** Its background controller's own ClusterRole
  covers only a few kinds; `rbac-limitranges.yaml` adds LimitRanges through the aggregation
  label `rbac.kyverno.io/aggregate-to-background-controller`. Generating another kind means
  another such role.
- The policy was tested offline with the Kyverno CLI (`kyverno apply <policy> --resource
  <namespaces>`), which is the quickest way to try a change before pushing.

### Checking the defaults

Kyverno gives every namespace a LimitRange `default-resources`. A container that sets no
`resources` gets requests of 50m CPU, 64Mi memory and 50Mi ephemeral storage, and limits of
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
`registry.k8s.io` or `public.ecr.aws` - whole hosts, not single organisations (user
decision); anything else is refused when the Deployment - or whichever controller - is
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
