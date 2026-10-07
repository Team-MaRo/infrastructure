---
paths:
  - "kubernetes/components/**"
---

# Kubernetes components

- Read the `README.md` of the component you touch first, and update it in the same change -
  design, settings that differ from upstream and runbooks live there.
- Pushing to `master` deploys, pruning included. Never apply these manifests by hand while
  Argo CD runs.
- Every container passes the restricted Pod Security Standard: `runAsNonRoot`,
  `allowPrivilegeEscalation: false`, `capabilities.drop: [ALL]`, `seccompProfile:
  RuntimeDefault`, no host namespaces, host paths or privileged mode. A namespace that needs
  more goes onto `k3s_psa_exempt_namespaces` in `ansible/roles/k3s/defaults/main.yml`.
- Images only from `docker.io`, `ghcr.io`, `quay.io`, `registry.k8s.io`, `public.ecr.aws`;
  write them fully qualified where an operator deploys them.
- No CPU limits. Set memory only where Kyverno's defaults (64Mi request, 128Mi limit) do not
  fit, and always in `kube-system` and `system-upgrade`.
- An app whose Application carries the finalizer brings its own `namespace.yaml` (never in
  `kube-system`); one that brings CRDs or volumes gets no finalizer - see "Pruning and the
  finalizer" in `kubernetes/components/argocd/README.md`.
- A ServiceMonitor, PodMonitor or PrometheusRule outside kube-prometheus-stack carries the
  sync option `SkipDryRunOnMissingResource=true`.
- Secret values never go into git: they live in OpenBao (`bao kv put`) and arrive through an
  ExternalSecret against the `ClusterSecretStore` `openbao`, with `refreshInterval: 5m`.
- A 1Password item is referenced by its title and its link (in comments: the title, the link
  on the line below) - see "Conventions" in `AGENTS.md`.
- An app's Postgres `DatabaseRole`/`Database` lives in the app's component with
  `namespace: postgres`; its MariaDB `User`/`Grant`/`Database` with `namespace: mariadb` and
  `cleanupPolicy: Skip`. Its namespace gets the label `trust.d3strukt0r.dev/postgres-ca` (or
  `mariadb-ca`) and mounts the ConfigMap of that name for verify-full - no CA copy of its own.
- An app that reads a Secret or ConfigMap only at start gets
  `secret.reloader.stakater.com/reload: <name>` (or `configmap.`) on its workload, and its
  namespace goes into `reloader.namespaces` in `kubernetes/components/reloader/values.yaml`.
- An app on a floating tag gets `keel.sh/policy: force`, `keel.sh/matchTag: "true"`,
  `keel.sh/trigger: poll` on its workload, `imagePullPolicy: Always`, and its Docker Hub image
  without `docker.io/` (`kubernetes/components/keel/README.md`).
- Every `kubectl` in docs or commands carries `--context d3strukt0r-prod-admin`.
