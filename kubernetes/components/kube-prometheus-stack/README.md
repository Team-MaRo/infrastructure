# kube-prometheus-stack

Metrics and alerts (`kubernetes/clusters/prod/kube-prometheus-stack.yaml`, chart pinned, values
and extra manifests in this directory): Prometheus, Alertmanager and Grafana in `monitoring` - an
ordinary namespace, not PSA-exempt. The LimitRange's defaults (32Mi, limit 128Mi) cover what the
values leave out; they set memory for the containers that need more - Prometheus, Grafana and
its sidecars, kube-state-metrics, the operator and the config reloaders - and for Alertmanager,
whose operator would otherwise request 200Mi. node-exporter goes to `kube-system`
(`namespaceOverride`), since it needs the host's network, PIDs and files; there is no
LimitRange there, so it sets its own. The values were sized on 2026-11-02 over
2026-10-23..11-02, the requests from real memory's p95, the limits from the working set's p95
and peak, each next to its value in `values.yaml`: Prometheus 1312Mi / 2720Mi, Grafana 528Mi /
1504Mi and its sidecars 80Mi / 192Mi, kube-state-metrics 160Mi / 384Mi, the operator and
node-exporter 48Mi / 96Mi, Alertmanager and the config reloaders 32Mi / 96Mi - written out,
since the operator would otherwise set its own. How the cluster's
manifests fit together is in [`kubernetes/README.md`](../../README.md).

Each monitoring component gets its own namespace, under the restricted Pod Security
Standard; components that need the host (node-exporter, the log collector) go to
`kube-system` instead. Their UIs are at their own addresses behind the Zitadel login;
port-forwards stay the break-glass way in. The status page and alerting heartbeat is
[Gatus](../gatus/README.md), the logs are [Loki](../loki/README.md) and
[Alloy](../alloy/README.md), the traces [Tempo](../tempo/README.md).

## Remote write and who may reach Prometheus

- **Prometheus accepts remote write** (`prometheusSpec.enableRemoteWriteReceiver`): Tempo's
  metrics generator pushes the metrics it computes from traces - `traces_spanmetrics_*` (rate,
  errors, durations per service and operation) and `traces_service_graph_*` (who calls whom) -
  to `/api/v1/write` instead of being scraped. Remote write takes no login.
- **So `network-policy.yaml` limits port 9090** (user decision 2026-10-18 - everyone reading is
  less dangerous than everyone writing): anything in `monitoring` (Grafana, the operator,
  Prometheus itself), namespace `tempo`, namespace `kubeelasti` (reads for scale-to-zero), the
  pods of mariadb's CronJob `mariadb-primary-off-rebooting-node` (reads `kured_reboot_required`;
  only those, by label - not MariaDB) and Traefik for `prometheus.d3strukt0r.dev`. Traefik runs
  on the host network: towards a pod on another node it arrives from that node's WireGuard
  address, the `.0` of the node's pod range; on the same node kube-router admits the node's own
  traffic anyway (both measured with test pods, 2026-10-18). A pod never has a `.0` address, so
  an ipBlock for it admits only the host.
- **The node allowances maintain themselves**: the Kyverno GeneratingPolicy
  `prometheus-from-nodes` (same file) creates a NetworkPolicy `prometheus-from-<node>` for every
  **ingress node** (`node-role.kubernetes.io/ingress=true`, the label that places Traefik) from
  its `spec.podCIDR`, and deletes it when the node loses the label or goes - NetworkPolicies add
  up, so nothing has to be edited when nodes come, go or change roles. The worker prod-04 has no
  ingress label and gets none. Kyverno ignores Nodes by default; only the Node itself was
  taken off its filter list (`components/kyverno/values.yaml`), not its status updates. If
  Kyverno is down, the existing allowances stay; a node added meanwhile gets its own once
  Kyverno is back. `kubectl port-forward` is not affected by any of it. Anything else that
  should read Prometheus needs an entry in `network-policy.yaml`: its namespace, or - where that
  namespace holds anything else - its namespace and its pods' label together, as for mariadb's
  CronJob, so nothing else there can remote-write.
- **Checking it**:
  `kubectl --context d3strukt0r-prod-admin -n monitoring get networkpolicy` lists `prometheus`
  and one `prometheus-from-<node>` per ingress node.

## Storage and retention

- **Prometheus keeps what fits in 16 GiB** of its 20 GiB volume (`retentionSize: 16GB` -
  Prometheus counts GB as GiB; oldest first), about 23 days at 0.7 GiB a day, and at most 30
  days (`retention`). Memory sizing needs that much: ten days missed the peak that got Argo CD's
  repo server OOM-killed (2026-11-02), so the volume went from 10 to 20 GiB that day. The volume
  runs full on purpose (user decision): 16 GiB is 82 % of the filesystem, and a compaction
  (blocks of up to 3 days merged beside the old ones, about 2 GiB for a few minutes) brings it to
  about 92 %. So the 80 % and 90 % volume alerts leave it out; the 95 % one still covers it and
  means its cleanup failed. A volume can grow but never shrink. Alertmanager and Grafana have no volume:
  silences and anything changed in Grafana's UI are lost on restart - dashboards and datasources
  come from the chart and from ConfigMaps (the sidecar reads every ConfigMap labelled
  `grafana_dashboard: "1"`).

### Growing Prometheus' volume

The operator does not resize an existing volume. Grow the volume before pushing a larger
`retentionSize`, or Prometheus fills the old one:

1. Grow the PVC; the CSI driver enlarges the Hetzner volume and its filesystem while Prometheus
   runs:
   ```shell
   kubectl --context d3strukt0r-prod-admin -n monitoring patch pvc prometheus-kube-prometheus-stack-prometheus-db-prometheus-kube-prometheus-stack-prometheus-0 -p '{"spec":{"resources":{"requests":{"storage":"20Gi"}}}}'
   ```
   `kubectl get pvc` shows the new capacity within a minute or two.
2. Push `storage` and `retentionSize` in `values.yaml`.
3. The operator replaces the StatefulSet itself, since its volume template cannot change in
   place (seen 2026-11-02); Prometheus restarts once for the new `retentionSize`. Check:
   ```shell
   kubectl --context d3strukt0r-prod-admin -n monitoring get statefulset prometheus-kube-prometheus-stack-prometheus -o jsonpath='{.spec.volumeClaimTemplates[0].spec.resources.requests.storage}'
   ```

## Scraping k3s

- **k3s is one process, so three scrapes are disabled** (controller manager, scheduler,
  proxy - there is nothing separate to reach), and with them their alerts and dashboards.
- **k3s serves one shared metrics registry on every port**: the API server's and each
  kubelet's `/metrics` return the same ~64,000 series per node (measured when adopted). The
  API server keeps them, minus histogram buckets that no rule or dashboard of the chart reads
  and minus the kubelet's own families; the kubelet keeps only its own (a keep list). Through
  the API server, the kubelet's volume metrics would also carry `namespace="default"` instead
  of the PVC's, and every volume alert would fire twice. Without that, Prometheus would hold
  roughly 380,000 series, half of them duplicates.
- **etcd is scraped by a `ScrapeConfig`** (`scrape-etcd.yaml`): the private IPs, port 2381, as
  opened by `etcd-expose-metrics` in `k3s_config`, which binds to the node's own address only.
  The chart's own way renders an Endpoints object, which Argo CD does not manage. `kubeEtcd`
  stays enabled with its Service and ServiceMonitor off, because disabling it would drop the
  etcd alerts and dashboard too; the job name `kube-etcd` matches them. **A new server node is
  one more target there.** Each target's `instance` is set to the node name rather than adding a
  `node` label: the chart's etcd alerts group `without (instance)`, and with a `node` label
  every member was a group of its own, so one member down fired `etcdInsufficientMembers`
  (failover test, 2026-09-28).
- **node-exporter listens on 9101**, not its default 9100: Traefik's metrics entrypoint holds
  9100 on the same host network. Both are closed by the Hetzner firewall.
- **The admission webhook's certificate comes from cert-manager**
  (`admissionWebhooks.certManager`), which removes the chart's Helm hook Jobs. The chart
  renders the webhooks without a `caBundle` and cert-manager's cainjector fills it in; since
  git never sets the field, Argo CD sees no difference and needs no `ignoreDifferences` - as
  with cert-manager's own webhook.

## Alerts

- **Alerts go to ntfy.sh, one topic, two priorities.** Alertmanager's webhook cannot set
  ntfy's priority header, so the priority goes into the topic URL, and there is one receiver
  per severity: `critical` at priority 5, everything else at 3, `info` nowhere (it stays in
  Grafana). ntfy's built-in `alertmanager` template formats the message. The URLs and token
  are assembled by the ExternalSecret `alertmanager-ntfy` from OpenBao `secret/ntfy` and read
  through `url_file`/`credentials_file`, so the topic - the secret on the free plan - never
  enters git or the Alertmanager config.
- **The Watchdog heartbeat**: the chart's always-firing `Watchdog` alert is routed to Gatus
  (receiver `watchdog`, the token from OpenBao `secret/gatus` through the ExternalSecret
  `alertmanager-ntfy`). Gatus alerts when it has not heard from it for 5 minutes - Alertmanager
  down, Prometheus down, or its rules not evaluating.
- **Own alerts** (`rules.yaml`): a node cordoned for over 2 hours (a failed upgrade Job or a
  drain that cannot finish), a node waiting for its reboot for over 26 hours (kured blocked by
  the MariaDB primary that could not be moved away, `../kured/README.md`), a server without a successful etcd snapshot upload for 13 hours
  or with a failed one, and a k3s certificate within 30 days of expiry (k3s renews at start
  within 120 days, so this means renewal failed). Their descriptions say what to do. The
  missing-upload alert reads the age of each node's newest S3 snapshot from k3s's
  `ETCDSnapshotFile` records, not from k3s's upload counter: after a k3s restart the counter
  reappears only with the next upload, already at 1, and `increase()` reads that as none - it
  fired falsely after the failover test.
- **The components' own metrics** are scraped too (2026-09-30): Traefik through a PodMonitor
  (`components/traefik/pod-monitor.yaml`, port `metrics` 9100 on the host network),
  cert-manager's controller through a ServiceMonitor (`http-metrics` 9402), Argo CD through one
  ServiceMonitor in `components/argocd-integrations/` (every Argo CD metrics Service has the
  `part-of: argocd` label and a `metrics` port), Kyverno through its chart's
  `serviceMonitor.enabled` per controller. Prometheus takes monitors from every namespace
  without a release label (`*SelectorNilUsesHelmValues: false`). Each monitor or rule gets
  `SkipDryRunOnMissingResource=true` from the shared Component `shared/monitoring-sync-options`,
  since the CRD belongs to another Application (or, here, arrives in the same sync). Alerts: `CertificateNotReady` (1 h), `CertificateRenewalOverdue` (2 days
  past its renewal time), `CertificateExpiringSoon` (under 7 days, critical),
  `ArgoCDApplicationOutOfSync` and `ArgoCDApplicationUnhealthy` (30 min). Their dashboards come
  from grafana.com by revision (`grafana.dashboards` in the values, at the top level next to the
  chart's own), downloaded at every Grafana start with `defaultCurlOptions: -sLf` - the chart's
  default `-k` skips the TLS check.
- **A stuck operator** (`rules.yaml`, 2026-11-02): `ControllerReconcileStuck` when an
  operator's queue has worked on one item for over 15 minutes, for 5 minutes (warning). A stuck
  operator applies no changes and repairs nothing - for mariadb-operator that includes the
  failover - while everything it runs still looks healthy, so nothing else tells. The metric is
  controller-runtime's `workqueue_longest_running_processor_seconds`, from every operator that
  exports it: mariadb-operator, CloudNativePG, External Secrets, trust-manager, Argo CD (and the
  API server). Normal passes take seconds, at most 17 s over 2026-10-23..11-02; mariadb-operator
  hung for about 3.3 hours at a time from 2026-10-29 until its restart on 2026-11-02
  ([`../mariadb-operator/README.md`](../mariadb-operator/README.md) "The operator is stuck").
  Not covered, since they do not export it: Kyverno, cert-manager, the Prometheus operator,
  KubeElasti, Reloader and Keel.
- **Volume alerts at three levels** (`rules.yaml`): 80 % warning, 90 % critical, 95 % critical
  again, each a separate alert so every level notifies once; inhibit rules in the
  Alertmanager config let a higher level silence the lower ones for the same PVC. Prometheus'
  own volume has only the 95 % level: it runs at 82-92 % by design ("Storage and retention"). They replace
  the chart's `KubePersistentVolumeFillingUp` (disabled), which only warned at 15 % free when
  a trend predicted it full within four days.
- **Container memory** (`rules.yaml`, 2026-10-18; the chart has none of these): three alerts for
  every container with a limit; the fix is in its component's README (a higher limit, fewer
  php-fpm workers, a lower `memory_limit`, a smaller cache).
  - `ContainerMemoryNearLimit`: the memory the kernel cannot reclaim stays above 80 % of the
    limit for 5 minutes (warning: raise the limit soon) or above 90 % (critical: the kill is
    near; it silences the warning through the inhibit rule) - 5 minutes, since it should come
    before the kill. The limit counts all of the container's memory; at the limit the
    kernel drops file cache first and kills only when that is not enough - and without swap it
    can free neither anonymous nor shared memory. So the alert measures the working set minus
    the active file cache, which leaves anonymous, shared and kernel memory. The working set
    alone (until 2026-10-25) counted file cache: mariadb's primary sat at 91 % from the cache of
    its writes, at the limit thousands of times without a kill. RSS (2026-10-25..11-02) missed
    shared memory - PHP's OPcache, Postgres' `shared_buffers` - on cgroup v2 it is anonymous
    memory only. Highest over 2026-10-23..11-02: 75 % (mariadb's agent).
  - `ContainerMemoryPressure`: the container waits for memory (PSI) more than 10 % of the time
    for 10 minutes - its limit slows it down before it kills it; longer, since brief bursts of
    reclaim (a backup, an import) are harmless. Highest over those days: 0.25 %.
    Hitting the limit as such is no alert: containers with much file cache do it constantly
    (Postgres 2.2 million times in 10 days, unharmed), since the kernel fills the cache up to it.
  - `ContainerOOMKilled` for a restart that an OOM kill caused. A single kill only restarts the
    container - no crash loop, so nothing else would tell - but every request it was serving
    fails with it (arepazo's php-fpm, twice on 2026-10-04, noticed only by chance).
- **php-fpm of the PHP apps** (`php-fpm.yaml`; the metrics come from each app's
  `pod-monitor.yaml`, job `<namespace>/php-fpm`): `PhpFpmRequestsQueued` (requests waiting for a
  worker), `PhpFpmNoIdleWorker` (all busy) and `PhpFpmStatusDown` (php-fpm's status pool,
  which answers even while every worker is busy, does not answer), each after 5 minutes. A slow
  spell like arepazo's (2026-10-11, every request 6-9 s for 2.5 hours) shows up as the first
  two; what the workers wait on is in the app's slow log. php-fpm does not report its pool
  size (`pm.max_children`), so neither alerts nor dashboard assume one: "no idle worker" is
  "at the limit", whatever the limit. The dashboard "php-fpm" (below) shows both.
- **WordPress' vulnerability check** (`wordpress.yaml`; the metrics come from each site's daily
  check, `kubernetes/charts/wordpress/README.md` "Vulnerability check"): `WordPressVulnerability`
  (warning), `WordPressVulnerabilityCritical` (CVSS 9 and above or on CISA's KEV list),
  `WordPressPluginClosed` and `WordPressVulnerabilityCheckStale`. Grouped by the affected item,
  not the pod, so a restart opens no new alert; a warning and a critical one never fire for the
  same finding, and they have different names so a critical one does not silence the site's other
  warnings (the inhibit rule matches on alertname).

## Grafana

- **Datasources**: the chart's Prometheus and Alertmanager, plus Loki and Tempo
  (`additionalDataSources`), linked both ways. A log line's `TraceId` (Traefik's JSON access
  log) becomes a link to the trace (a derived field on Loki); a trace's spans link to the logs
  around them (±5 minutes, filtered by trace ID), to their service's metrics and to the service
  map drawn from `traces_service_graph_*`. In the values the link is written `$${__value.raw}`,
  since Grafana's provisioning would read `${...}` as an environment variable.
- **Dashboards of our own** are JSON files in `dashboards/`, turned into ConfigMaps labelled
  `grafana_dashboard: "1"` by the `configMapGenerator` in `kustomization.yaml`; Grafana's
  sidecar loads them from every namespace. Today one: "php-fpm" - per app the workers, queued
  requests, requests per second, slow requests and php-fpm's own log with the slow-log
  backtraces. A dashboard changed in Grafana's UI is lost on restart: change the JSON instead
  (Grafana's "Export" gives it).
- **Grafana's admin** comes from OpenBao `secret/grafana` through the ExternalSecret
  `grafana-admin`; the chart would otherwise generate a new random password on every render.
  It is the break-glass login since Grafana moved to `https://grafana.d3strukt0r.dev`.
- **Grafana logs in through Zitadel** (`auth.generic_oauth` in the values, the app in
  `tofu/zitadel/apps_grafana.tf`): a public client, PKCE without a secret, with
  `auth_style: InParams` so the client ID goes into the request body and no Basic header is
  sent. `role_attribute_path` makes `infra-admin` from the `groups` claim `GrafanaAdmin`
  (server admin, which includes Admin of its organisation; `allow_assign_grafana_admin`), and
  `role_attribute_strict` refuses everyone else. Sign-out ends the Zitadel session too
  (`signout_redirect_url`). Only settings that differ from Grafana's defaults are written;
  new users are created at their first login (`allow_sign_up`, a default).

Grafana is restarted by Reloader when Secret `grafana-admin` changes in OpenBao (environment
variables read at start; `grafana.annotations`, which the chart puts on all its Grafana objects -
Reloader only acts on the Deployment; [`../reloader/README.md`](../reloader/README.md)).

## Accessing the UIs

**Grafana** is at `https://grafana.d3strukt0r.dev` - "Sign in with OpenID Connect"; its local `admin`
(password in 1Password [`Grafana | Prod | Admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=25tqaghtajnc3efoop2ve6jfze&h=my.1password.com)) and the port-forward below
stay as break-glass. **Prometheus** is at `https://prometheus.d3strukt0r.dev` and
**Alertmanager** at `https://alertmanager.d3strukt0r.dev`, both behind the Zitadel gate
(`middleware-oauth2-proxy.yaml`, [`../oauth2-proxy/README.md`](../oauth2-proxy/README.md)),
since neither has a login of its own. Prometheus' address is its `externalUrl`, so the
"Source" link in an alert message opens the alert's query there; Alertmanager's is set
explicitly, as the chart would derive `http://`. All three by port-forward:

```shell
kubectl --context d3strukt0r-prod-admin -n monitoring port-forward svc/kube-prometheus-stack-grafana 3000:80
kubectl --context d3strukt0r-prod-admin -n monitoring port-forward svc/kube-prometheus-stack-prometheus 9090:9090
kubectl --context d3strukt0r-prod-admin -n monitoring port-forward svc/kube-prometheus-stack-alertmanager 9093:9093
```

then `http://localhost:3000` (break-glass: user `admin`, password in 1Password [`Grafana | Prod | Admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=25tqaghtajnc3efoop2ve6jfze&h=my.1password.com)),
`http://localhost:9090` (Status → Targets shows every scrape) and `http://localhost:9093`
(firing alerts, silences).

## kube-state-metrics and operator objects

- **kube-state-metrics also reads operator objects** (`customResourceState` in the values): the
  status conditions of `MariaDB` and `PhysicalBackup` become
  `kube_customresource_condition{customresource_kind, name, type, status, reason}`, valued with
  the condition's `lastTransitionTime`, for the MariaDB alerts; a `PointInTimeRecovery`'s
  `lastRecoverableTime` becomes `kube_customresource_last_recoverable_time{name}` (unix seconds,
  kube-state-metrics parses the RFC 3339 string), and `kube_customresource_pitr_ref{pitr}` (an
  info metric, value 1) names the one the MariaDB archives into, so the alert can ignore older,
  frozen ones. The mariadb-operator reports archiving and backups nowhere else. k3s's
  `ETCDSnapshotFile` records become `kube_customresource_etcd_snapshot_created{node, name,
  bucket}` (creation time; `bucket` only on the S3 copies), for the snapshot alert. Another kind
  is one more entry there plus its `list`, `watch` in `rbac.extraRules`; the config can be tried
  locally by running the kube-state-metrics image with `--custom-resource-state-only` against the
  admin kubeconfig.
- **Its series keep a fixed `instance`** (`kube-state-metrics`, a relabeling in the values), not
  the pod's address: otherwise every restart started a second series per object, and for the next
  hour every join on `kube_pod_info` failed as many-to-many - Freelens' node graphs and Disk column
  showed nothing, and rules that join it failed to evaluate (2026-11-02). It runs as one replica,
  so there is one source; during a rollout Prometheus drops the few duplicate samples.

## Before the first sync

Prometheus collects metrics for 30 days, Grafana shows them, and Alertmanager sends alerts to
the phone through the ntfy topic from [Gatus](../gatus/README.md) - `critical` at priority 5,
`warning` at 3, `info` only in Grafana. It needs, before its first sync, `secret/ntfy` and
`secret/gatus` (see "Before the first sync" in [Gatus](../gatus/README.md)) and Grafana's admin
password:

1. Create a password item [`Grafana | Prod | Admin`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=25tqaghtajnc3efoop2ve6jfze&h=my.1password.com) in 1Password (username `admin`).
2. With the port-forward to OpenBao open:

   ```shell
   jq -n --arg p "$(op item get 'Grafana | Prod | Admin' --account my.1password.com --vault Private --fields password --reveal)" \
     'if ($p|length)==0 then error("empty value - 1Password lookup failed") else {"admin-password":$p} end' \
   | BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN="$(op item get 'OpenBao | Prod | Recovery keys & root token' --account my.1password.com --vault Private --fields credential --reveal)" \
     bao kv put secret/grafana -
   ```

   JSON on stdin rather than `key=value`: a generated password may start with `@`, and a
   failed lookup would otherwise store an empty value (see "Putting secret values in" in
   [`tofu/openbao/README.md`](../../../tofu/openbao/README.md)).

After the first sync, the Watchdog on the status page turns green within two minutes, and
Gatus sends a "resolved" message if it had reported it down. While Gatus is unreachable,
Alertmanager reports the failed heartbeats as `AlertmanagerFailedToSendAlerts` after a few
minutes.

## When an alert arrives

Its message says what fired and where. The cluster's own alerts (`rules.yaml`) carry what to do
in their description; the chart's are explained in the
[runbooks](https://runbooks.prometheus-operator.dev/), which each alert links as `runbook_url`.
A known cause being worked on can be silenced in Alertmanager's UI
(`https://alertmanager.d3strukt0r.dev`, Silences → New Silence),
for a fixed time.

**A volume alert** (80 %, 90 %, 95 %) is answered by growing the volume: raise the PVC's
storage request - in the values or manifest that defines it, then push - and Hetzner resizes
it while it stays mounted. A volume can never shrink; going smaller means a new volume and
losing its data.

**Testing the path to the phone** - an alert that resolves itself after five minutes:

```shell
kubectl --context d3strukt0r-prod-admin -n monitoring exec alertmanager-kube-prometheus-stack-alertmanager-0 -c alertmanager -- \
  amtool alert add TestAlert severity=warning --annotation=summary="Test from the README" \
  --end="$(date -u -v+5M +%Y-%m-%dT%H:%M:%SZ)" --alertmanager.url=http://localhost:9093
```
