# oauth2-proxy

The login gate: UIs without a Zitadel login of their own - the Traefik dashboard, phpMyAdmin,
Prometheus and Alertmanager - sit behind oauth2-proxy (`kubernetes/clusters/prod/oauth2-proxy.yaml`, chart pinned, values and extra
objects in this directory, namespace `oauth2-proxy`). Traefik asks it about every request (a
`forwardAuth` Middleware); it never proxies a request itself (`upstreams: static://202`). How the
cluster's manifests fit together is in [`kubernetes/README.md`](../../README.md).

## Design

- **Not logged in**: it answers with the redirect to Zitadel, Traefik hands that to the
  browser, and after the login Zitadel returns to `https://oauth2-proxy.d3strukt0r.dev/oauth2/callback`,
  which sets the cookie and sends the browser back (`reverse-proxy` reads the original address
  from Traefik's headers; `whitelist-domain` allows returning anywhere under the domain).
  **Logged in with `infra-admin`** (the `groups` claim, `allowed-group`): 202, and Traefik
  passes the request on. Logged in without it: refused.
- **One login for all guarded UIs**: the cookie is for `.d3strukt0r.dev`, so browsers send it
  to every name under the domain - all of them on the cluster since the old server went
  (2026-10-04). It is encrypted with the cookie secret.
- **The one client with a secret** (`tofu/zitadel/apps_oauth2_proxy.tf`, auth method BASIC):
  oauth2-proxy refuses to run without one. The secret Zitadel returned at creation is in the
  tofu state, so it was regenerated in the console at once; the live one and the cookie secret
  are in 1Password [`Zitadel | Prod | oauth2-proxy`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=7ckbf72eceri3t2s3cy7c5du2y&h=my.1password.com) and OpenBao `secret/oauth2-proxy`, delivered
  by the ExternalSecret `oauth2-proxy` (the client ID, not secret, is written into its
  template).
- **The Middleware exists once per guarded namespace** (`kube-system`, `phpmyadmin`,
  `monitoring`) - Traefik only takes middleware from the router's own namespace
  (`allowCrossNamespace` stays off);
  each points at `http://oauth2-proxy.oauth2-proxy.svc`, drops the client's `X-Forwarded-*`
  headers (`trustForwardHeader: false`) and takes at most 1 MiB as answer
  (`maxResponseBodySize`). A new guarded UI gets a copy in its namespace, the annotation
  `traefik.ingress.kubernetes.io/router.middlewares: <namespace>_oauth2-proxy@kubernetescrd` on
  its Ingress (`_`: Traefik's safe naming, [`../traefik/README.md`](../traefik/README.md)), and
  its hostname under the domain.
- The chart meets the restricted Pod Security Standard as it is; the image is on `quay.io`;
  one replica - with it down, the guarded UIs are unreachable (fail closed) but everything
  else keeps running.
- **Restarted by Reloader** when Secret `oauth2-proxy` changes in OpenBao (environment variables
  read at start; `deploymentAnnotations`, [`../reloader/README.md`](../reloader/README.md)).

## Before the first sync

Its Zitadel app comes from `tofu/zitadel` (`apps_oauth2_proxy.tf`); its secrets go into OpenBao:

1. Right after `tofu apply` created the app, regenerate its secret - the one from creation is in
   the tofu state: Zitadel console → Projects → Infrastructure → oauth2-proxy → Regenerate
   Secret. It is shown once.
2. A 1Password item with that secret and a cookie secret (exactly 32 random bytes - oauth2-proxy
   accepts only 16, 24 or 32):

   ```shell
   op item create --account my.1password.com --vault Private --category password \
     --title 'Zitadel | Prod | oauth2-proxy' \
     "password=<the regenerated client secret>" \
     "cookie-secret[password]=$(openssl rand -base64 32 | tr -- '+/' '-_')" >/dev/null
   ```
3. As JSON on stdin, with the empty check (port-forward and token as in
   [OpenBao](../openbao/README.md)):

   ```shell
   jq -n --arg c "$(op item get 'Zitadel | Prod | oauth2-proxy' --account my.1password.com --vault Private --fields password --reveal)" \
         --arg k "$(op item get 'Zitadel | Prod | oauth2-proxy' --account my.1password.com --vault Private --fields cookie-secret --reveal)" \
     'if ($c|length)==0 or ($k|length)==0 then error("empty value - 1Password lookup failed") else {"client-secret":$c,"cookie-secret":$k} end' \
   | BAO_ADDR=http://127.0.0.1:8200 BAO_TOKEN="$(op item get 'OpenBao | Prod | Recovery keys & root token' --account my.1password.com --vault Private --fields credential --reveal)" \
     bao kv put secret/oauth2-proxy -
   ```

A new client ID (a recreated app) goes into `external-secrets.yaml`.
