# KubeElasti

Scale to zero: rarely used apps can sleep at zero replicas and wake on the first request
(`kubernetes/clusters/prod/kubeelasti.yaml`, chart `elasti` pinned, values in this directory,
namespace `kubeelasti`). How the cluster's manifests fit together is in
[`kubernetes/README.md`](../../README.md).

Chosen over Sablier because Sablier on Kubernetes can only answer the first request with an
HTML waiting page, which breaks API clients; KubeElasti **holds** the request until the app is
ready and answers with the real response (user decision, 2026-09-30). KEDA's HTTP add-on (beta,
a proxy permanently in front of every app, ~200-350 MiB) and Knative (far heavier) were rejected
too.

## Design

- **How it works**: an `ElastiService` per app names its Service, its Deployment, a Prometheus
  query and a `cooldownPeriod`. When the query stays below its threshold that long, the operator
  scales the Deployment to 0 and adds an EndpointSlice that points the app's Service at its
  resolver. The public Service itself is never changed; a private copy (`elasti-<svc>-pvt-*`)
  reaches the pods. The first request makes the resolver scale the app back up, and it holds the
  request until a pod is Ready (up to `reqTimeout`, 120 s here, then 408); once the app is awake
  the resolver steps out of the path.
- **Proven 2026-09-30** with a throwaway whoami app: asleep after 275 s idle, the first request
  through Traefik held and answered with the app's JSON after 3.1 s, the next one in 0.13 s;
  Argo CD stayed Synced throughout. Operator ~24 MiB, resolver ~12 MiB; restricted Pod Security
  as the chart ships it; the chart's CPU limits are removed (`null`) as everywhere.
- **Traefik needs two things per app.** The Service annotation
  `traefik.ingress.kubernetes.io/service.nativelb: "true"`: Traefik normally sends to pod IPs
  and skips the resolver's EndpointSlice, which has no ready condition (KubeElasti#285, open) -
  with it, Traefik sends to the ClusterIP and kube-proxy accepts the slice. The cost is coarser
  balancing (per connection, no sticky sessions), irrelevant for one replica. And a `headers`
  Middleware on the app's Ingress setting `X-Envoy-Decorator-Operation:
  <svc>.<ns>.svc.cluster.local`: the resolver finds the app to wake from that header and would
  misread the public Host.
- **The scale-down query** is Traefik's request counter for the app's service,
  `sum(rate(traefik_service_requests_total{service="<ns>-<svc>-<port name>@kubernetes"}[2m])) or
  vector(0)`, threshold `0.01` (under one request per 100 s). It needs Traefik's PodMonitor.
- **Health checks must not wake an app**: `probeResponse` in the ElastiService answers a matching
  request (here `GET /health` with header `X-Health-Probe: gatus`) from the resolver while the app
  sleeps. Gatus checks such an app inside the cluster (`http://<svc>.<ns>.svc/health` with that
  header), bypassing Traefik, so its checks neither wake the app nor count toward the query.
  New or changed rules take up to 5 minutes to reach the resolver.
- **GitOps**: the app's Deployment has **no `replicas`** in git, so Argo CD's self-heal leaves the
  operator's scaling alone. The kubeelasti Application has **no finalizer**: the chart carries
  the ElastiService CRD, and removing it would delete every ElastiService.
- **Known weak spots, accepted**: pre-1.0, one vendor (CNCF Sandbox since 2026-01), no commit for
  weeks at the time of adoption. In 0.1.30, deleting an ElastiService while its app sleeps leaves
  the app at zero (fixed in 0.1.31-rc2): wake an app before removing its ElastiService alone.
  HTTP only (no TCP or TLS passthrough); only a Service's first port is proxied while asleep.

## Letting an app sleep

An app that is rarely used can sleep at zero replicas and wake on its first request, which
KubeElasti holds until the app is ready (a few seconds - 3 s for a small test app). What an app
needs, all in its own component:

1. **No `replicas` in its Deployment** - KubeElasti scales it, and one replica when awake.
2. **Its Service** carries `traefik.ingress.kubernetes.io/service.nativelb: "true"`.
3. **A Middleware** telling KubeElasti which app a request is for, referenced from its Ingress
   (`traefik.ingress.kubernetes.io/router.middlewares: <ns>-kubeelasti-target@kubernetescrd`):

   ```yaml
   apiVersion: traefik.io/v1alpha1
   kind: Middleware
   metadata:
     name: kubeelasti-target
   spec:
     headers:
       customRequestHeaders:
         X-Envoy-Decorator-Operation: <svc>.<ns>.svc.cluster.local
   ```

4. **An ElastiService** (`argocd.argoproj.io/sync-options: SkipDryRunOnMissingResource=true`, the
   CRD is the kubeelasti Application's):

   ```yaml
   apiVersion: elasti.truefoundry.com/v1alpha1
   kind: ElastiService
   metadata:
     name: <app>
   spec:
     service: <svc>
     minTargetReplicas: 1
     cooldownPeriod: 900          # seconds without requests before it sleeps
     scaleTargetRef:
       apiVersion: apps/v1
       kind: Deployment
       name: <deployment>
     triggers:
       - type: prometheus
         metadata:
           query: sum(rate(traefik_service_requests_total{service="<ns>-<svc>-<port name>@kubernetes"}[2m])) or vector(0)
           threshold: "0.01"
     probeResponse:
       - method: GET
         path:
           type: Exact
           value: /health
         headers:
           - name: X-Health-Probe
             value: gatus
         response:
           status: 200
           body: '{"status":"asleep"}'
   ```

5. **Its Gatus check** goes to the Service inside the cluster, with the probe header - through
   the public address it would wake the app every minute and keep it awake:

   ```yaml
   - name: <App>
     group: Apps
     url: http://<svc>.<ns>.svc/health
     headers:
       X-Health-Probe: gatus
     conditions:
       - "[STATUS] == 200"
   ```

Whether it sleeps:

```shell
kubectl --context d3strukt0r-prod-admin -n <ns> get elastiservice <app> -o jsonpath='{.status.mode}{"\n"}'   # proxy = asleep, serve = awake
kubectl --context d3strukt0r-prod-admin -n <ns> get deployment <deployment>
```

**Removing it again**: wake the app first (one request) before deleting only the ElastiService -
KubeElasti 0.1.30 leaves a sleeping app at zero otherwise. Removing the whole app does not need
that.
