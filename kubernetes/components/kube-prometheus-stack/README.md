# kube-prometheus-stack

Metrics and alerts (`kubernetes/clusters/prod/kube-prometheus-stack.yaml`, chart pinned, values
and extra manifests in this directory): Prometheus, Alertmanager and Grafana in `monitoring` - an
ordinary namespace, not PSA-exempt: the LimitRange's defaults fit most of its containers, so the
values set memory only for Prometheus and Grafana, which need more, and for Alertmanager, whose
operator would otherwise request 200Mi. node-exporter goes to `kube-system`
(`namespaceOverride`), since it needs the host's network, PIDs and files; there is no
LimitRange there, so it sets its own. How the cluster's manifests fit together is in
[`kubernetes/README.md`](../../README.md).

Each monitoring component gets its own namespace, under the restricted Pod Security
Standard; components that need the host (node-exporter, the log collector) go to
`kube-system` instead. Their UIs are at their own addresses behind the Zitadel login;
port-forwards stay the break-glass way in. The status page and alerting heartbeat is
[Gatus](../gatus/README.md), the logs are [Loki](../loki/README.md) and
[Alloy](../alloy/README.md).

## Storage and retention

- **Prometheus keeps what fits in 8 GB** of its 10 GB volume (`retentionSize`, oldest first),
  roughly three weeks at ~350 MB a day, capped at 30 days. It started on 30 GB and was
  recreated at 10 GB: a volume can grow but never shrink, so the smaller start is the
  cheaper mistake. Alertmanager and Grafana have no volume: silences and anything
  changed in Grafana's UI are lost on restart - dashboards and datasources come from the chart
  and from ConfigMaps (the sidecar reads every ConfigMap labelled `grafana_dashboard: "1"`).

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
  drain that cannot finish), a server without a successful etcd snapshot upload for 13 hours
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
  without a release label (`*SelectorNilUsesHelmValues: false`). Each monitor or rule outside
  kube-prometheus-stack carries `SkipDryRunOnMissingResource=true`, since the CRD belongs to
  another Application. Alerts: `CertificateNotReady` (1 h), `CertificateExpiringSoon` (under 14
  days - renewal starts at 30, critical), `ArgoCDApplicationOutOfSync` and
  `ArgoCDApplicationUnhealthy` (30 min). Their dashboards come from grafana.com by revision
  (`grafana.dashboards` in the values, folder "Components"), downloaded at every Grafana start
  with `defaultCurlOptions: -sLf` - the chart's default `-k` skips the TLS check.
- **Volume alerts at three levels** (`rules.yaml`): 80 % warning, 90 % critical, 95 % critical
  again, each a separate alert so every level notifies once; inhibit rules in the
  Alertmanager config let a higher level silence the lower ones for the same PVC. They replace
  the chart's `KubePersistentVolumeFillingUp` (disabled), which only warned at 15 % free when
  a trend predicted it full within four days.

## Grafana

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

**Grafana** is at `https://grafana.d3strukt0r.dev` - "Sign in with Zitadel"; its local `admin`
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

- **kube-state-metrics also reads operator objects** (`customResourceState` in the values):
  the status conditions of `MariaDB` and `PhysicalBackup` become
  `kube_customresource_condition{customresource_kind, name, type, status, reason}`, valued with
  the condition's `lastTransitionTime`, for the MariaDB alerts. The mariadb-operator reports
  archiving and backups nowhere else. k3s's `ETCDSnapshotFile` records become
  `kube_customresource_etcd_snapshot_created{node, name, bucket}` (creation time; `bucket` only
  on the S3 copies), for the snapshot alert. Another kind is one more entry there plus its
  `list`, `watch` in `rbac.extraRules`; the config can be tried locally by running the
  kube-state-metrics image with `--custom-resource-state-only` against the admin kubeconfig.

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
