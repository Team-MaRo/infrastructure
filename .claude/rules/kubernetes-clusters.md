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
  that brings CRDs, holds a volume, or is foundation (root, private, argocd, hcloud-csi, traefik, coredns),
  and never on `priority-classes` (a pod naming a missing class is refused). See
  "Pruning and the finalizer" in `kubernetes/components/argocd/README.md`.
- A chart gets a patch range (`targetRevision: "~X.Y.Z"`) and follows its patch releases by
  itself; raising the range (a minor or major) is a deliberate change after reading the changelog
  and rendering it. Versions update themselves wherever the tool can: an upstream with a Helm
  chart gets the chart in the Application (values from `components/<app>/` as a second source, its
  manifests as a third), a pinned upstream manifest only without a chart or for a reason written
  next to it (Argo CD's bootstrap); an image we set runs on a `major.minor`/`major` tag with Keel
  where one exists, an exact tag only where floating is unsafe (database operators, Traefik's
  plugin) - see AGENTS.md. A chart of our
  own (`kubernetes/charts/`) comes from `master`, like its values (Argo CD refuses a `$values`
  source at another revision of the same repository); a chart change reaches every site at once
  (`kubernetes/charts/wordpress/README.md`).
- `ServerSideApply=true` where CRDs are large; `ServerSideDiff=true` where an operator
  defaults fields in its objects (otherwise the app stays OutOfSync).
- Prefer the Application's own `namespace.yaml` in the component over `CreateNamespace` for
  apps with the finalizer.
- A new component also needs its line in the component table of `kubernetes/README.md` and in
  the index of `AGENTS.md`.
