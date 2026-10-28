# zitadel

What is inside Zitadel - everything the Helm values in `kubernetes/components/zitadel/` do not
cover: the instance's default login and domain policies, token lifetimes and the organisation
`D3strukt0r`'s domains, adopted in `imports.tf`; the groups webhook's action; and the project
`Infrastructure` with its role and the apps of every admin UI and of kubectl; and a project
per WordPress site ("WordPress sites"); and the project "Matomo" ("Matomo"). It reaches Zitadel at
`https://auth.d3strukt0r.dev` directly, no port-forward. How Zitadel itself runs on the cluster is
in [`kubernetes/components/zitadel/README.md`](../../kubernetes/components/zitadel/README.md).

Shared rules, credentials and the state backend are in [`../README.md`](../README.md).

## What the module configures

- **What is inside Zitadel is `tofu/zitadel`** (provider `zitadel/zitadel`, at least 3.8.7 - the
  first release that imports the default login policy): the instance's login and domain
  policies and the organisation's domains, adopted with import blocks after they had been set
  in the console, and the instance's token lifetimes (ID and access tokens 1 hour instead of
  12, since Kubernetes cannot revoke a token before it expires; refresh tokens at Zitadel's
  defaults). The lifetimes are instance-wide, so the admin UIs' sessions also renew hourly,
  usually unnoticed through a silent re-login. The project `Infrastructure` holds the admin
  UIs' and kubectl's applications (Argo CD, Grafana, pgAdmin, OpenBao, oauth2-proxy, zot -
  oauth2-proxy and zot need a client secret, regenerated in the console after creation and kept
  in 1Password and OpenBao only) and the role `infra-admin` (role keys carry their project's prefix, since the `groups` claim mixes all
  projects' roles); role assertion puts roles into tokens and role check makes Zitadel itself
  refuse users without a role. A project and its roles carry display names for the console and
  the login page ("Robines Portfolio", "Administrator"); only the role keys are technical, since
  apps and the `groups` claim read those. Role assignments are here too - the personal user is
  looked up by username. It logs in as the machine user `iam-admin` with the
  key its setup job wrote (`zitadel_jwt_profile` in the gitignored tfvars; the key expires
  2029-01-01). Human users stay in the console - their passwords and second factors are theirs.
  Kubernetes runs one `components/zitadel` however many organisations or instances exist; they
  are data in Zitadel, so per-tenant files belong in `tofu/zitadel`.
- **Every project with an app sets `project_role_check = true`.** Since Arepazo lets strangers
  register, a Zitadel account no longer means someone known; the role check is what keeps them
  out of every app but the shop - Zitadel refuses the login before the app sees a token. An app
  in a project without it admits every account of the instance.
- **The `groups` claim comes from a webhook** (`components/zitadel/groups-webhook.yaml`, targets
  and executions in `tofu/zitadel/groups_claim.tf`). Zitadel sends roles only as a nested map,
  `urn:zitadel:iam:org:project:roles`, which Argo CD, OpenBao and oauth2-proxy cannot read; an
  Actions v2 target (`REST_CALL`) on the functions `preuserinfo` and `preaccesstoken` calls a
  short Python script (stock `python` image, the script in a ConfigMap - the repo builds no
  images) that answers the role keys as a list. Actions v1, a script inside Zitadel, would need
  no service, but is deprecated and goes with Zitadel v5 (user decision). Roles only arrive for
  projects with role assertion on. `interrupt_on_error = false`: with the webhook down, tokens
  lack `groups`, Zitadel logins still work and the UIs refuse (fail closed). The request
  signature is not checked - the answer only matters to Zitadel, and a NetworkPolicy admits
  only Zitadel's pods. Zitadel only calls the webhook because its deny list leaves out the
  webhook Service's address, and it checks a target against that list when the target is
  created - see "Zitadel refuses to call private addresses" in
  [`kubernetes/components/zitadel/README.md`](../../kubernetes/components/zitadel/README.md).
- **The provider speaks native gRPC**, which Cloudflare's proxy refuses with `403` unless the
  zone's gRPC switch (dashboard → Network → gRPC) is on - turned on for `d3strukt0r.dev` on
  2026-09-30. No provider manages that switch, so it is set by hand; the symptom of it being
  off is `server closed the stream without sending trailers`. It opens nothing new: the same
  API is public over REST and gRPC-web.
- **Email through mailomat** (`email.tf`, since 2026-10-18; before, Zitadel had no email
  provider and sent nothing). Invitations, email verification, password resets and OTP codes
  leave from `d3strukt0r Login <auth@d3strukt0r.dev>` through mailomat's SMTP user
  `zitadel@mailomat.d3strukt0r.dev` (`smtp.mailomat.cloud:587`): mailomat signs as
  `mailomat.d3strukt0r.dev`, which `d3strukt0r.dev`'s relaxed DMARC counts for the domain
  ([`../cloudflare/README.md`](../cloudflare/README.md), "Sending through mailomat"); replies
  go to addy. `tls = true` is right for 587: Zitadel tries TLS from the first byte and falls
  back to STARTTLS when the server answers in plain text (its `channels/smtp/channel.go`);
  `false` would never encrypt. `zitadel_email_provider_smtp`, not the deprecated
  `zitadel_smtp_config`. **The password is write-only** (`smtp_password`, an ephemeral variable
  from the gitignored tfvars, 1Password
  [`Mailomat | Prod | zitadel`](https://start.1password.com/open/i?a=RWQYBTIV4BG3RD74KLKHPJVTXU&v=rgb7ahgkjpry4bld5uyx5ya5au&i=7vpv7gubslfi4qfcvhqaod42zy&h=my.1password.com),
  an E-Mail item): the state keeps only `password_hash`, so a plan shows when it changed.
  Zitadel stores it encrypted in its own database; nothing on the cluster needs it. To rotate:
  "Eigenes Passwort setzen" in mailomat with a new 1Password value, the new line into the tfvars
  (the command in `terraform.tfvars.example`, which escapes what HCL reads specially), apply.
  The mailomat plan's 200 mails a month are shared with the websites.

## WordPress sites

Each WordPress site is a project of its own (`project_<site>.tf`), so who may log in to it is
decided per site; the site logs in through the WordPress chart's OIDC support
([`../../kubernetes/charts/wordpress/README.md`](../../kubernetes/charts/wordpress/README.md),
"Login through Zitadel").

- **Role check and project check on**: Zitadel refuses anyone without a role in the project, or
  whose organisation neither owns nor was granted it, before WordPress is reached. Role
  assertion puts the roles into userinfo, where WordPress reads them.
- **A role per WordPress role**, `<site>:<wordpress-role>` (`robines-portfolio:administrator`,
  `robines-portfolio:editor`): the site sets that role at every login and creates the WordPress
  user at the first. Changing someone's role is changing their grant here (or in the console
  for people of another organisation); it applies at their next login.
- **A second app for Claude**, `<site>-claude`, where a site lets people edit it through Claude
  (charts/wordpress README "Content through Claude"): native, public (`NONE`, PKCE - the
  connector holds no secret), authorization code and refresh token, callbacks claude.ai's and
  Claude Code's fixed `http://localhost:33418/callback`. In the site's project, so the role
  check applies; the site checks its tokens with the first app's credentials (introspection).
  Its client ID is an output (`<site>_claude_client_id`) and goes into the site's
  `mcp.clientId`.
- **A confidential app** (`OIDC_AUTH_METHOD_TYPE_BASIC`): the plugin has no PKCE. Its secret is
  regenerated after creation and kept in 1Password and OpenBao only, like oauth2-proxy's.
- The personal user gets `<site>:administrator`; other people of the organisation get a grant
  by username once their user exists (human users are created in the console).
- **A site with its own people** (`project_arepazo.tf`): the shop arepazo's staff and customers
  are in the organisation `Arepazo`, which its owner Eduardo runs. The project stays in ours and
  is granted to Arepazo with all its roles (`zitadel_project_grant`), so Arepazo's owner gives
  his people roles in the console (Granted projects) and none of them needs anything here.
  - **Customers register themselves**: Arepazo's own login policy allows it - and requires no
    second factor and never prompts for one, so a shopper needs no authenticator app. A policy
    applies to the whole organisation, so staff are not forced either and should set one up
    themselves. The site's "Create an account" button sends the shopper there (charts/wordpress
    README "Registration in Zitadel").
  - **Their first role**: the organisation's metadata `registration-roles`
    (`{"project": ..., "roles": ["arepazo:customer"]}`) tells the registration webhook what every
    new member gets; `registration.tf` has Zitadel call it when a user is created. Its machine
    user `registration` is an `ORG_USER_MANAGER` of Arepazo and nothing else; its token is made
    in the console ("Adding a WordPress site", step 5), never here.

### Adding a WordPress site

1. `project_<site>.tf` after `project_robines_portfolio.tf`: project, roles, app (redirect URI
   `https://<host>/wp-admin/admin-ajax.php?action=openid-connect-authorize`, post-logout URI
   `https://<host>/wp-login.php?loggedout=true`), grants. Plan, apply.
2. Regenerate the app's secret - the one from creation is in the tofu state: console → Projects
   → `<site>` → the app → Regenerate Secret. It is shown once; copy it and run step 3 straight
   away, which takes it from the clipboard.
3. Into 1Password, then OpenBao (logged in with `bao login -method=oidc -no-print` against
   `BAO_ADDR=https://openbao.d3strukt0r.dev`):

   ```shell
   op item create --account my.1password.com --vault Private --category password \
     --title 'Zitadel | Prod | <site>' "password=$(pbpaste)" \
     "client-id[text]=$(cd ~/Projects/infrastructure/tofu/zitadel && tofu output -raw <site>_client_id)" >/dev/null
   jq -n --arg c "$(op item get 'Zitadel | Prod | <site>' --account my.1password.com --vault Private --fields password --reveal)" \
     'if ($c|length)==0 then error("empty value - 1Password lookup failed") else {"client-secret":$c} end' \
   | bao kv put secret/<site>-oidc -
   ```
4. The site's values: `oidc.clientId` and the `secrets` block for `OIDC_CLIENT_SECRET` (chart
   README).
5. Only for a site whose people register themselves, as arepazo: its organisation, login
   policy, `registration-roles` and machine user as in `project_arepazo.tf`, and the site's
   `oidc.registerOrganisation` (`tofu output -raw <site>_org_id`). The registration webhook has
   one token, of the machine user in Arepazo; a second such organisation gives that user
   `ORG_USER_MANAGER` there too (`zitadel_org_member`) instead of a user of its own. The token
   was made once - console → Organization (`Arepazo`) → Users → Service Users → `registration`
   → Personal Access Tokens → New, no expiry; shown once, copied, then from the clipboard:

   ```shell
   op item create --account my.1password.com --vault Private --category password \
     --title 'Zitadel | Prod | registration token' "password=$(pbpaste)" >/dev/null
   jq -n --arg t "$(op item get 'Zitadel | Prod | registration token' --account my.1password.com --vault Private --fields password --reveal)" \
     'if ($t|length)==0 then error("empty value - 1Password lookup failed") else {"token":$t} end' \
   | bao kv put secret/zitadel-registration -
   ```

   The ExternalSecret `zitadel-registration` (kubernetes/components/zitadel) brings it to the
   webhook, which reads it at every call.

## Matomo

Matomo, the web analytics of every site (`project_matomo.tf`, kubernetes/components/matomo), logs
in through Zitadel with its RebelOIDC plugin: the project "Matomo" and its app `matomo`. Tried
first on a local Matomo (2026-10-22/23): users are created at their first login, their site
rights follow these roles at every login, and Zitadel refuses anyone without a role.

- **A role is a right on a Matomo site**: `matomo:<siteID>:<view|write|admin>`. The groups
  webhook turns these keys into the claim `matomo-permission-path`
  (`/matomo/<siteID>/<access>`), which Matomo's RebelOIDC plugin ("fine-grained permissions")
  reads at every login and makes the user's site rights exactly match - a role removed here is a
  right gone at the next login, and rights given inside Matomo vanish at the next login too. Role
  check on: without any Matomo role Zitadel refuses the login, so the plugin's own role gate
  stays off. A site's roles are added here when the site is added to Matomo.
- **The admin of all of Matomo** holds `matomo:superuser` here and Matomo's superuser flag
  there. The claim can only name single sites, so the role only gets its holder past the role
  check (the webhook makes no site right of it); the flag is set once in Matomo (Administration →
  Benutzer → Superuser-Zugriff) and covers every site, future ones too. Taking the role away
  blocks the login, not the flag - remove that in Matomo as well.
- **The access token is a JWT**: RebelOIDC reads the claims by decoding the access and ID
  tokens it receives from the token endpoint.
- **The client secret** is regenerated in the console after creation, like the WordPress sites'
  (the one Zitadel returns at creation is in the state), and entered in Matomo's plugin settings.
- **Logging out** ends the Zitadel session too and returns to `https://matomo.d3strukt0r.dev/`
  (`post_logout_redirect_uris`, Matomo's `login_logout_url`); without that address Zitadel ends
  on a page our login does not serve.

## Running it

`zitadel/terraform.tfvars` holds `zitadel_jwt_profile`: the whole JSON of the machine user
`iam-admin`'s key, pasted between the heredoc markers from

```sh
op document get 'Zitadel | Prod | iam-admin key' --account my.1password.com --vault Private | jq .
```

and `smtp_password`, the mailomat password (`email.tf`), as a line this prints escaped:

```sh
op item get 'Mailomat | Prod | zitadel' --account my.1password.com --vault Private --fields smtp_password --reveal \
  | python3 -c 'import json,sys; print("smtp_password = " + json.dumps(sys.stdin.read().strip()).replace("${", "$${").replace("%{", "%%{"))'
```

```sh
cd tofu/zitadel
tofu init
tofu plan
```

Adopting something set by hand: an `import` block, then `tofu plan -generate-config-out=generated.tf`
writes its live values; move them into
the module, delete `generated.tf`, and plan again until only the import remains. Human users are
not managed here.
