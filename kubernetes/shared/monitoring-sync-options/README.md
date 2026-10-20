# monitoring-sync-options

A Kustomize Component: every ServiceMonitor, PodMonitor, PrometheusRule and ScrapeConfig in a
kustomization that lists it gets the annotation
`argocd.argoproj.io/sync-options: SkipDryRunOnMissingResource=true`.

Their CRDs come with kube-prometheus-stack - another Application, or, for kube-prometheus-stack's
own rules and scrape configs, the same sync. Argo CD dry-runs every object before a sync; while
the CRD does not exist yet (at a bootstrap, or a rebuilt cluster) that dry run fails and takes
the whole sync with it. With the option it skips the dry run for that object, which then applies
once the CRD is there.

## Using it

In the component's `kustomization.yaml`:

```yaml
components:
  - ../../shared/monitoring-sync-options
```

and the monitors and rules there leave the annotation out. Its other annotations are kept (the
patch merges).

## Where it does not reach

Kustomize applies it, so not the WordPress chart's PodMonitor (Helm,
`charts/wordpress/templates`, which carries the annotation itself) and not monitors that a chart
renders from its values (Kyverno, Zitadel, the operators) - those only matter at a bootstrap.

## Checking it

```shell
kubectl kustomize kubernetes/components/<app> | yq 'select(.apiVersion == "monitoring.coreos.com/v1") | .kind + "/" + .metadata.name + " " + .metadata.annotations."argocd.argoproj.io/sync-options"'
```
