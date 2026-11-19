# RabbitMQ

Back to [`kubernetes/README.md`](../../README.md).

The cluster's shared AMQP broker, for Symfony Messenger and any later app that queues work:
one RabbitMQ node, a vhost and user per app. Deployed by the Application `rabbitmq`
(`kubernetes/clusters/prod/rabbitmq.yaml`) into the namespace `rabbitmq`, run by the operators
in [`../rabbitmq-operators/`](../rabbitmq-operators/README.md). Decided 2026-10-06 over Messenger's
Redis or Doctrine transports and over LavinMQ (lighter, but no operator, and its image runs as
root); built 2026-11-19, before its first consumer, so it runs and is measured first.

## Design

- **One node** (`rabbitmq-cluster.yaml`), RabbitMQ **4.4.1** - community support for 4.4 runs to
  2027-07-31. An exact tag: the operator owns the StatefulSet, so Keel must not change its image,
  and the operator's own default lags. Classic queues: quorum queues add nothing on one node.
- **A 10 GiB Hetzner Volume** (`hcloud-volumes`, `Retain`), PriorityClass `stateful-core`. No
  backups: messages are work in transit, and the definitions (vhosts, users, permissions) live in
  Git as the apps' topology objects - a lost volume costs the messages waiting at that moment.
- **Memory 224Mi / 576Mi** (measured 2026-11-19 in a local kind cluster: working set 206-216 MiB,
  cgroup peak 445 MiB with file cache), the memory watermark at 384 MiB - above it RabbitMQ blocks
  publishers rather than being OOM-killed - and 512 MB free disk before it blocks publishers:
  stopping while there is room left keeps its own writes and paging memory to disk working (the
  limit at least the memory watermark, as RabbitMQ advises); a full disk would crash it instead.
  Consumers keep working through either alarm, and it clears by itself. Re-sized by the rule
  once prod has a week of data.
- **Restricted Pod Security** through `override.statefulSet`: the pod gets `runAsNonRoot` and the
  seccomp profile, both containers no privilege escalation and no capabilities - the operator
  leaves them out. Tested under an enforcing namespace in kind.
- **No Application finalizer**: it holds a volume.

### TLS only

Every listener is TLS (`disableNonTLSListeners`): AMQP on 5671, the UI and its API on 15671, the
metrics on 15691; nothing listens in plain text (`rabbitmq-diagnostics listeners`).

- **Certificates** (`tls.yaml`): a self-signed Issuer makes the CA `rabbitmq-ca` (5 years), the CA
  signs `rabbitmq-tls` for `rabbitmq`, `rabbitmq.rabbitmq.svc` and its `.cluster.local` form
  (cert-manager's 90 days, renewed 30 days before). The CA's key never leaves this namespace.
- **The CA reaches whoever verifies** through trust-manager's Bundle `rabbitmq-ca` (key `ca.crt`,
  [`../trust-manager/README.md`](../trust-manager/README.md)) in every namespace labelled
  `trust.d3strukt0r.dev/rabbitmq-ca: "true"`: the apps, this namespace (Traefik's
  ServersTransport and the PodMonitor read it here), and `rabbitmq-system` (the topology operator
  talks HTTPS to the API). `ca-reader.yaml` lets trust-manager's copy read the CA Secret - here,
  since this Application creates the namespace.
- **A renewed server certificate is picked up without a restart**: tested 2026-11-19 in kind by
  having cert-manager issue a new one - within two minutes new connections on 5671, 15671 and
  15691 got the new serial (Kubernetes updates the mounted Secret, then Erlang's certificate
  cache expires), restart count 0. Existing connections keep the certificate they started with,
  which is fine: it stays valid for 30 more days after a renewal.

### The management UI

`https://rabbitmq.d3strukt0r.dev` (`ingress.yaml`, proxied by Cloudflare, behind the
`cloudflare-only` chain), with RabbitMQ's own Zitadel login - no oauth2-proxy in front.

- **Zitadel**: the app `RabbitMQ` in the project Infrastructure (`tofu/zitadel/apps_rabbitmq.tf`) -
  a public browser client (PKCE, no secret) whose access token is a JWT. Its client ID
  (`400334089078440102`) is both `management.oauth_client_id` and RabbitMQ's
  `resource_server_id`, so only tokens issued for this app count.
- **Rights**: the groups webhook puts the user's project roles into the token's `groups` list;
  `additional_scopes_key = groups` reads it, and the alias maps `infra-admin` onto RabbitMQ's
  administrator tag and full rights on every vhost (`scope_prefix = rabbitmq.`). Without the role
  Zitadel refuses the login already (role check on the project).
- **The login button** is RabbitMQ's fixed text; it cannot be set to "OpenID Connect" in this
  single-provider setup, like OpenBao's.
- **Apps never use OAuth**: `auth_backends` tries the internal users (the apps' passwords) first,
  then OAuth tokens.
- **Traefik talks HTTPS to the UI** and verifies it with our CA: the Service carries the
  annotations (set through the RabbitmqCluster's `spec.service`), the ServersTransport `rabbitmq`
  in `ingress.yaml` names the CA and the certificate's name.
- **Break-glass**: the operator's generated admin user, Secret `rabbitmq-default-user`, which the
  topology operator also uses. The UI's own password login is off; the API still accepts it:

  ```shell
  kubectl --context d3strukt0r-prod-admin -n rabbitmq port-forward svc/rabbitmq 15671:15671
  kubectl --context d3strukt0r-prod-admin -n rabbitmq get secret rabbitmq-default-user -o jsonpath='{.data.password}' | base64 -d
  ```

### Network

`network-policy.yaml` admits AMQPS only from namespaces labelled
`rabbitmq.d3strukt0r.dev/client: "true"`, the API from `rabbitmq-system` (the topology operator),
the metrics from `monitoring`, and the UI from Traefik on the ingress nodes - one NetworkPolicy per
node, generated by Kyverno like Prometheus' (`kube-prometheus-stack/network-policy.yaml` explains
why).

## Monitoring

`pod-monitor.yaml` scrapes `/metrics` (memory, alarms, connections) and `/metrics/detailed` with only
the families `queue_coarse_metrics` and `queue_consumer_count` (per queue), over TLS. Alerts
(`rules.yaml`, exact figures; queues named `*failed*` - Messenger's failure transport, meant to
hold messages - are left out):

| Alert | When |
|---|---|
| `RabbitMQDown` | not scraped for 5 min |
| `RabbitMQAlarm` | the memory or disk alarm blocks publishers for 5 min |
| `RabbitMQQueueWithoutConsumer` | messages ready and no consumer for 15 min |
| `RabbitMQQueueBacklog` | never empty within an hour and fuller than an hour ago |
| `RabbitMQMessagesUnacked` | unacknowledged messages for an hour - a hung worker |

The container's memory is kube-prometheus-stack's `ContainerMemoryNearLimit`.

## Adding an app

Like a database for Postgres or MariaDB, the app's own component brings its objects, with
`namespace: rabbitmq`:

1. A password in OpenBao (`bao kv put secret/<app>-rabbitmq` with `username` and `password`).
2. An ExternalSecret `<app>-rabbitmq-user` in `rabbitmq` with those two keys and the label
   `rabbitmq.com/topology-operator: "true"` (the topology operator reads only labelled Secrets).
3. `Vhost` (`spec.name: <app>`), `User` (`importCredentialsSecret: <app>-rabbitmq-user`) and
   `Permission` (`vhost: <app>`, `user: <app>`, `.*` for configure, write and read) - each with
   `rabbitmqClusterReference: {name: rabbitmq, namespace: rabbitmq}`.
4. In the app's namespace: the labels `rabbitmq.d3strukt0r.dev/client: "true"` and
   `trust.d3strukt0r.dev/rabbitmq-ca: "true"`, its own ExternalSecret for the DSN
   `amqps://<user>:<password>@rabbitmq.rabbitmq.svc:5671/<app>`, and the ConfigMap `rabbitmq-ca`
   mounted as the CA to verify against (Messenger: `cacert` in the DSN's options).

Deleting those objects removes the vhost with its queues and the user.

## Checking it

```shell
kubectl --context d3strukt0r-prod-admin -n rabbitmq get rabbitmqcluster,pods,pvc
kubectl --context d3strukt0r-prod-admin -n rabbitmq get vhosts,users,permissions -A
kubectl --context d3strukt0r-prod-admin -n rabbitmq exec rabbitmq-server-0 -c rabbitmq -- rabbitmq-diagnostics status
kubectl --context d3strukt0r-prod-admin -n rabbitmq exec rabbitmq-server-0 -c rabbitmq -- rabbitmq-diagnostics listeners
kubectl --context d3strukt0r-prod-admin -n rabbitmq exec rabbitmq-server-0 -c rabbitmq -- rabbitmqctl list_queues -p <vhost> name messages_ready messages_unacknowledged consumers
```

## Upgrading

Raise `image` in `rabbitmq-cluster.yaml` after reading the release notes of every version in
between. Within a series (4.4.x) the operator restarts the pod; for the next series, first enable
all feature flags on the running node (`rabbitmqctl enable_feature_flag all`) - RabbitMQ never
downgrades, and free patches exist only for the newest series, so a series upgrade comes about
every six months.
