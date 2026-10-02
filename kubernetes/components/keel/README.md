# Keel

Rolls a workload out when a new image is pushed under the floating tag it runs (`:latest`,
`:nginx-latest`, `:1`, ...), without a commit (`kubernetes/clusters/prod/keel.yaml`, the chart
pinned there, values and manifests in this directory, namespace `keel`). How the cluster's
manifests fit together is in [`kubernetes/README.md`](../../README.md).

## Why

An app pinned to a version gets a new one through git: bump the tag, push, Argo CD rolls it out.
An app on a floating tag never changes in git - the tag stays `:latest` while the image behind it
changes - so nothing would roll it out. Keel watches those tags and rolls the workload out when
the digest behind one changes.

## Design

- **Opt-in per workload.** Keel only looks at workloads that carry `keel.sh/policy`; every other
  one stays untouched.
- **Polling, not webhooks**: every 5 minutes (`polling.defaultSchedule`; per workload
  `keel.sh/pollSchedule`). A check is a manifest `HEAD`, which Docker Hub does not count against
  its pull limit. Docker Hub's webhooks were tested (2026-10-02) and left out: they would need a
  public endpoint on Keel (Docker Hub does send the URL's `user:password` as basic auth, so Keel's
  `AUTHENTICATED_WEBHOOKS` would work), but Keel 0.22.4 ignores the digest in the webhook's
  payload and its poller keeps its own last-seen digest - with both on, every push rolls out
  twice; webhook only would lose the safety net of polling. Reported upstream
  (https://github.com/keel-hq/keel/issues/940); worth revisiting once Keel uses the webhook's
  digest.
- **What Keel writes** on a new digest: the pod-template annotation `keel.sh/update-time` (that is
  the rollout), `keel.sh/digest` and `kubernetes.io/change-cause` on the workload, and the image
  in its **short form**. Argo CD ignores the three annotations
  (`resource.customizations.ignoreDifferences.all` in
  [`../argocd/patches/argocd-cm.yaml`](../argocd/patches/argocd-cm.yaml)), never the image.
- **The image without `docker.io/` in git** (`d3strukt0r/app:latest`, not
  `docker.io/d3strukt0r/app:latest`): Keel writes Docker Hub images back as
  `<user>/<repo>:<tag>`, and with the long form in git Argo CD's self-heal would put it back -
  rolling the app out a second time for the same image. Kyverno's `allowed-registries` reads the
  short form as `index.docker.io`.
- **`imagePullPolicy: Always`** on such a workload, or a node keeps running the `:latest` it pulled
  first. With an unchanged digest a pull is only a `HEAD`.
- **Notifications**: every update goes to the alerts' ntfy topic (OpenBao `secret/ntfy`), titled
  "Keel" - through Keel's plain webhook notifier (`WEBHOOK_ENDPOINT` in `external-secrets.yaml`),
  with ntfy's templating turning Keel's JSON into the message. Not Shoutrrr, Keel's own route to
  ntfy: Keel 0.22.4 sets a `level` parameter on every Shoutrrr message, and Shoutrrr's ntfy service
  rejects it (tested 2026-10-02 - every notification failed;
  https://github.com/keel-hq/keel/issues/941). Keel reads the URL only at start,
  and its chart cannot annotate its Deployment for Reloader: after the ntfy token or topic changes
  in OpenBao, restart it once -
  `kubectl --context d3strukt0r-prod-admin -n keel rollout restart deployment keel`.
- **RBAC** is the chart's ClusterRole cut down to the Kubernetes provider: read namespaces,
  nodes and pods, update Deployments, StatefulSets, DaemonSets and CronJobs - **no Secrets**. The
  images are public and polled anonymously. A private image needs a Docker Hub token
  (`dockerRegistry` in the chart: a dockerconfigjson Secret) and Secret access for it.
- No Helm provider (Argo CD only renders charts), no UI, no approvals. Keel's SQLite (approvals,
  audit log) lives on an emptyDir, so the root filesystem stays read-only. No CPU limit; the
  chart's 64Mi/128Mi memory.
- **Chart 1.2.3, Keel 0.22.4**, from `ghcr.io/keel-hq/keel` - the Docker Hub image is no longer
  published. The 0.22 line has needed several fixes in a row (RBAC for nodes in 0.22.2, API
  throttling in 0.22.4) and Keel has essentially one maintainer: read the release notes before an
  upgrade.

## Opting an app in

On the workload's own `metadata` (not the pod template; with a chart, its deployment-annotations
value):

```yaml
metadata:
  annotations:
    keel.sh/policy: force        # follow the tag, whatever version is behind it
    keel.sh/matchTag: "true"     # only this tag, not newer tags of the repository
    keel.sh/trigger: poll
spec:
  template:
    spec:
      containers:
        - image: d3strukt0r/<app>:latest   # without docker.io/
          imagePullPolicy: Always
```

An app pinned to a version needs none of this.

Proven on 2026-10-02 with a throwaway app on `d3strukt0r/keel-test:latest` (removed again): each of
three pushes under the same tag was noticed at the next poll, rolled out exactly once (one new
ReplicaSet, the image string unchanged), Argo CD stayed in sync, and the last one arrived on ntfy.

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin -n keel logs deploy/keel
kubectl --context d3strukt0r-prod-admin -n <namespace> get deploy <name> -o jsonpath='{.metadata.annotations.keel\.sh/digest}'
```

The log names every tracked image, every check that found a new digest, and every update.
