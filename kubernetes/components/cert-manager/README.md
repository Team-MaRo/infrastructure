# cert-manager

Back to [`kubernetes/README.md`](../../README.md).

`kubernetes/components/cert-manager/` deploys cert-manager (pinned release manifest,
v1.21.2; `ServerSideApply=true` is required, its CRDs are too large otherwise) and two
ClusterIssuers (`cluster-issuers.yaml`): `letsencrypt-staging` (untrusted certificates) for
trying things, `letsencrypt` for anything serving traffic. `external-secret.yaml` brings the
two Cloudflare tokens from OpenBao; `service-monitor.yaml` and `rules.yaml` are its metrics and
alerts.

## How it issues

- **DNS-01 over the Cloudflare API**, not HTTP-01: cert-manager proves a name by creating an
  `_acme-challenge` TXT record and deletes it afterwards. No records need preparing, Let's
  Encrypt never has to reach the cluster, and it works for cluster-internal names and
  wildcards. No zone has CAA records, so Let's Encrypt may issue (CAA is a planned
  hardening - it must then also list Cloudflare's own CAs for its edge certificates).
- **Two tokens, one per Cloudflare account** - a token cannot span accounts. Each issuer
  has two solvers: one selecting `arepazo.ch` (the arepazo account's only zone) with the arepazo
  token, and one without a selector - the fallback for every other zone - with the personal
  token. cert-manager uses the most specific match. A zone added to the arepazo account
  must be added to that `dnsZones` list; the personal account needs nothing.
- **Tokens**: `Zone:DNS:Edit` + `Zone:Zone:Read` on "All zones from an account", no expiry
  (an expiring token would silently stop renewals) and **no client IP filter** - nodes may be
  added or replaced, and a filter on today's node IPs would break cert-manager on any new
  one. They are protected by where they live instead, in
  1Password as [`Cloudflare | cert-manager DNS (prod cluster)`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=gb6fdq5kggtselhlpywmmlvake&h=my.1password.com) and
  [`Cloudflare | Arepazo | cert-manager DNS (prod cluster)`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=vnc4tluvl3iy35fxkkavvszkwy&h=my.1password.com), copied into OpenBao `secret/cloudflare-dns` (fields `personal`,
  `arepazo`) with `bao kv put`, and delivered as Secret `cert-manager/cloudflare-api-tokens`
  by an ExternalSecret. ClusterIssuers read their Secrets from cert-manager's own namespace.
- **No email on the ACME accounts**: optional, this repo is public, and Let's Encrypt no
  longer sends expiry mails. cert-manager renews by itself 30 days before expiry.
- **Its pods run restricted** (non-root, seccomp, no capabilities), so `cert-manager` is not
  in `k3s_psa_exempt_namespaces`.

## Metrics and alerts

cert-manager's controller is scraped through a ServiceMonitor (`http-metrics` 9402); the
monitor and the rules carry `SkipDryRunOnMissingResource=true`, since the CRD belongs to
another Application. Alerts: `CertificateNotReady` (1 h), `CertificateExpiringSoon` (under 14
days - renewal starts at 30, critical).

## Requesting a certificate

cert-manager gets certificates from Let's Encrypt, proving each name through a temporary
DNS record in Cloudflare - no DNS preparation, and the name need not be reachable. A
certificate is requested with a `Certificate` next to the workload:

```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: example
  namespace: example
spec:
  secretName: example-tls        # the Secret with tls.crt and tls.key
  dnsNames: [example.d3strukt0r.dev]
  issuerRef:
    kind: ClusterIssuer
    name: letsencrypt
```

```shell
kubectl --context d3strukt0r-prod-admin get certificate -A
kubectl --context d3strukt0r-prod-admin describe challenge -A   # while one hangs
```

## Creating or replacing the Cloudflare tokens

The issuers use one Cloudflare token per account. Creating or replacing them:

1. Cloudflare dashboard → My Profile → API Tokens → Create Token, permissions
   `Zone → DNS → Edit` and `Zone → Zone → Read`, zone resources "All zones from an
   account". Once in the personal login, once in the arepazo login.
2. Store them in 1Password as [`Cloudflare | cert-manager DNS (prod cluster)`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=gb6fdq5kggtselhlpywmmlvake&h=my.1password.com) and
   [`Cloudflare | Arepazo | cert-manager DNS (prod cluster)`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=vnc4tluvl3iy35fxkkavvszkwy&h=my.1password.com) (field `credential`).
3. With the port-forward to OpenBao open and `BAO_TOKEN` set:

   ```shell
   bao kv put secret/cloudflare-dns \
     personal="$(op item get 'Cloudflare | cert-manager DNS (prod cluster)' --account my.1password.com --vault Private --fields credential --reveal)" \
     arepazo="$(op item get 'Cloudflare | Arepazo | cert-manager DNS (prod cluster)' --account my.1password.com --vault Private --fields credential --reveal)"
   ```
