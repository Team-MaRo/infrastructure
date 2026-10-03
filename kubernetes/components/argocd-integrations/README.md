# Argo CD integrations

Back to [`kubernetes/README.md`](../../README.md).

What Argo CD needs from other components' CRDs - cert-manager, External Secrets, the
Prometheus operator - deployed by its own Application, `argocd-integrations`
(`kubernetes/clusters/prod/argocd-integrations.yaml`, with the finalizer), into the `argocd`
namespace. Argo CD itself is [`../argocd/`](../argocd/README.md).

## Why it is separate

**Nothing in `components/argocd/` may need another component's CRDs**: the bootstrap applies
it to a fresh cluster before cert-manager, External Secrets or the Prometheus operator exist.
Argo CD's certificate, its webhook secret and its ServiceMonitors are therefore
`components/argocd-integrations/`, an Application of their own (with the finalizer). The
certificate sat in `components/argocd/` until 2026-09-30, which would have broken a rebuild.

## What it holds

- `certificate.yaml` - the certificate `argocd-tls` for `argocd.d3strukt0r.dev`, from the
  ClusterIssuer `letsencrypt`.
- `external-secrets.yaml` - the GitHub webhook secret, from OpenBao `secret/argocd-webhook`
  (1Password [`GitHub | infrastructure | Argo CD webhook`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=gknq7z4job2lgkcpzszvucaxay&h=my.1password.com)).
  `argocd-secret` carries only the reference `$argocd-webhook:github-secret`, and the Secret it
  names needs the label `app.kubernetes.io/part-of: argocd`. How the webhook works is "How
  quickly a push arrives" in [`../argocd/README.md`](../argocd/README.md).
- `repository-infrastructure-private.yaml` - read access to `Team-MaRo/infrastructure-private`,
  which the Application `private` (`clusters/prod/private.yaml`) syncs: a fine-grained token of
  the bot account `MaRose-Bot` (resource owner Team-MaRo, only that repository, "Contents:
  Read-only"), from OpenBao `secret/argocd-repo-infrastructure-private` (field `token`; 1Password
  [`GitHub | MaRose-Bot | ArgoCD Infrastructure Private`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=xvqlbbv22gfv3grsa35sindt2e&h=my.1password.com)).
  The label `argocd.argoproj.io/secret-type: repository` makes Argo CD use it. **It expires every
  31.12 and is renewed on 01.01** (Team-MaRo allows at most a year); missed, every Application
  from that repository goes to sync status `Unknown` and `ArgoCDApplicationOutOfSync` alerts -
  what runs keeps running, only new commits stop arriving. Renewing: on GitHub as `MaRose-Bot`,
  Settings → Developer settings → Fine-grained tokens, the settings above, expiry 31.12; an org
  owner approves it (Team-MaRo → Settings → Personal access tokens → Pending requests); into the
  1Password item, then logged in to OpenBao:

  ```shell
  jq -n --arg t "$(op item get 'GitHub | MaRose-Bot | ArgoCD Infrastructure Private' --account my.1password.com --vault Private --fields token --reveal)" \
    'if ($t|length)==0 then error("empty token - 1Password lookup failed") else {"token":$t} end' \
  | bao kv put secret/argocd-repo-infrastructure-private -
  kubectl --context d3strukt0r-prod-admin -n argocd annotate externalsecret repository-infrastructure-private force-sync=$(date +%s) --overwrite
  ```

  then revoke the old token.
- `service-monitor.yaml` - Argo CD is scraped through one ServiceMonitor (every Argo CD
  metrics Service has the `part-of: argocd` label and a `metrics` port).
- `rules.yaml` - the alerts `ArgoCDApplicationOutOfSync` and `ArgoCDApplicationUnhealthy`
  (30 min).

Each monitor or rule outside kube-prometheus-stack carries
`SkipDryRunOnMissingResource=true`, since the CRD belongs to another Application.
