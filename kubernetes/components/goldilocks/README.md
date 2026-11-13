# goldilocks

Goldilocks shows the VPA recommender's view of each container's requests and limits at
`https://goldilocks.d3strukt0r.dev`, behind the Zitadel gate (`kubernetes/clusters/prod/goldilocks.yaml`,
the chart pinned there, values and manifests in this directory, namespace `goldilocks`). From
Fairwinds. It is one of three views on memory sizing: our own rule in Grafana's dashboard "Memory
sizing" ([`../kube-prometheus-stack/README.md`](../kube-prometheus-stack/README.md)), and VPA's
numbers, which Goldilocks displays and the recommender computes
([`../vertical-pod-autoscaler/README.md`](../vertical-pod-autoscaler/README.md)). How the cluster's
manifests fit together is in [`kubernetes/README.md`](../../README.md).

## Reading it

Per namespace and workload, two suggestions for each container, both from VPA's formula (p90 of
daily peaks + 15 %), not from our rule:

| Goldilocks | Request | Limit |
|---|---|---|
| Guaranteed | VPA's target | VPA's target |
| Burstable | VPA's lower bound | VPA's upper bound |

Next to them it shows what is set now - **as written in the workload**, so a container that lives
on the namespace's defaults (Kyverno's LimitRange adds 32Mi / 128Mi when the pod is created)
shows "Not Set" here; the Grafana dashboard reads the running pods and shows the real values. The
bounds are wide while the recommender's history is short (about 8 days to settle), and it reads the working set, file cache included, so cache-heavy
containers come out higher than our rule. Our values still come from the rule; Goldilocks is the
second opinion.

## Design

- **Two parts from one image**: the controller creates a VPA object with `updateMode: Off` for
  every Deployment, StatefulSet and DaemonSet in every namespace (`on-by-default`, no namespace
  label needed); the dashboard reads them, and needs `on-by-default` too, or its namespace list
  stays empty.
- **The controller logs at `-v0`**: at `2` it reports every namespace's VPA count and every pod
  added or deleted, ~23,000 lines a day. Errors stay, among them bug #893 below. Jobs and CronJobs get none
  (`ignore-controller-kind`): the recommender loses short-lived pods' history
  (kubernetes/autoscaler#10211), and the Grafana dashboard covers them.
- **Nothing changes a pod**: `Off` makes VPA only recommend, and the VPA updater and admission
  controller are not installed.
- **The VPA objects are not in git.** The controller owns them; Argo CD does not track them, and
  they hold no settings of ours.
- **No login of its own**, so the Ingress (from the chart) puts `cloudflare-only` and the
  oauth2-proxy gate in front, like phpMyAdmin; the role `infra-admin` passes.
- **Cost estimates off** (`enable-cost: false`): otherwise the page's JavaScript calls
  `insights.fairwinds.com`.
- **The image is only on `us-docker.pkg.dev`** (Google's Artifact Registry), allowed for it in
  Kyverno and pulled through zot ([`../kyverno/README.md`](../kyverno/README.md)); pinned by the
  chart, so `IfNotPresent` instead of the chart's `Always`.
- **No Argo Rollouts rights** (`enableArgoproj: false`): Rollouts is not installed.
- **Memory on the namespace's defaults** (32Mi / 128Mi from Kyverno's LimitRange) instead of the
  chart's 256Mi request: not measured yet, sized by the rule after a week.
- **No Gatus check**: behind the gate the page answers with a redirect; pods not running already
  alert through the chart's default rules.

## Known bugs

- FairwindsOps/goldilocks#897: every pod event makes the controller write every VPA in that
  namespace again, even unchanged - watch the API server's load if it ever matters.
- FairwindsOps/goldilocks#893: for pods owned by a custom resource (the databases' operators) it
  logs "Error retrieving parent object" and falls back to the StatefulSet - log noise only.

## Removing it

The finalizer removes everything the Application deployed, but not the VPA objects the controller
created. Afterwards:

```sh
kubectl --context d3strukt0r-prod-admin get vpa -A -l source=goldilocks
kubectl --context d3strukt0r-prod-admin delete vpa -A -l source=goldilocks
```
