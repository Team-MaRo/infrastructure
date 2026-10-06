---
paths:
  - "kubernetes/clusters/**"
---

# Kubernetes cluster directories

- Read `kubernetes/README.md` first; it says how clusters and components fit together. A
  subdirectory with its own `README.md` (such as `etcd-snapshots/`) is updated in the same
  change.
- `clusters/<name>/` decides what runs where; how it runs belongs in `components/<app>/`. A
  cluster that needs something different gets a Kustomize overlay here, not a copy.
- Pushing to `master` deploys, pruning included: deleting an Application file here deletes
  what it deployed if it carries `resources-finalizer.argocd.argoproj.io`.
- The finalizer only on apps whose data lives elsewhere or does not matter - never on an app
  that brings CRDs, holds a volume, or is foundation (root, private, argocd, hcloud-csi, traefik),
  and never on `priority-classes` (a pod naming a missing class is refused). See
  "Pruning and the finalizer" in `kubernetes/components/argocd/README.md`.
- Pin every chart and manifest version. A Helm-only upstream: chart pinned in the Application,
  values from `components/<app>/` as a second source, its manifests as a third.
- `ServerSideApply=true` where CRDs are large; `ServerSideDiff=true` where an operator
  defaults fields in its objects (otherwise the app stays OutOfSync).
- Prefer the Application's own `namespace.yaml` in the component over `CreateNamespace` for
  apps with the finalizer.
- A new component also needs its line in the component table of `kubernetes/README.md` and in
  the index of `AGENTS.md`.
