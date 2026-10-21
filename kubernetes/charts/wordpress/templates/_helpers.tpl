{{/* The site's own address: the first of its hosts. */}}
{{- define "wordpress.host" -}}
{{- required "hosts needs at least one name" (first .Values.hosts) -}}
{{- end -}}

{{/* The database and its user: the site's name, which MariaDB cannot hold with hyphens. */}}
{{- define "wordpress.database" -}}
{{- .Release.Name | replace "-" "_" -}}
{{- end -}}

{{/*
An ExternalSecret's data from a list of blocks {key, env: {VARIABLE: property}} (values secrets).
*/}}
{{- define "wordpress.secretData" -}}
{{- range . }}
{{- $key := required "every secrets block needs a key" .key }}
{{- range $name, $property := required (printf "the secrets block %s needs env" $key) .env }}
- secretKey: {{ $name }}
  remoteRef:
    key: {{ $key }}
    property: {{ $property }}
{{- end }}
{{- end }}
{{- end -}}

{{/*
The remoteRef of one variable from values secrets, for the objects that need the same password
outside the pod (the MariaDB user, the Valkey user). Fails when no block provides it.
*/}}
{{- define "wordpress.secretRef" -}}
{{- $name := .name }}
{{- $ref := dict }}
{{- range .secrets }}
{{- if hasKey (default dict .env) $name }}
{{- $ref = dict "key" .key "property" (index .env $name) }}
{{- end }}
{{- end }}
{{- if not $ref }}
{{- fail (printf "no secrets block provides %s" $name) }}
{{- end -}}
key: {{ $ref.key }}
property: {{ $ref.property }}
{{- end -}}

{{- define "wordpress.bucket" -}}
d3strukt0r-prod-{{ .Release.Name }}
{{- end -}}

{{/*
The environment of every container that loads WordPress: wp-config.php reads every WORDPRESS_*
variable at runtime, so each one carries the same set.
*/}}
{{- define "wordpress.env" -}}
# wp-cli keeps its cache under HOME; the images' own home is not writable for 33.
- name: HOME
  value: /tmp
- name: WORDPRESS_DB_HOST
  value: mariadb-primary.mariadb.svc:3306
- name: WORDPRESS_DB_USER
  value: {{ include "wordpress.database" . }}
- name: WORDPRESS_DB_NAME
  value: {{ include "wordpress.database" . }}
- name: WORDPRESS_CONFIG_EXTRA
  value: |
    define( 'S3_UPLOADS_BUCKET', '{{ include "wordpress.bucket" . }}' );
    define( 'S3_UPLOADS_REGION', 'nbg1' );
    define( 'S3_UPLOADS_KEY', getenv( 'S3_KEY' ) );
    define( 'S3_UPLOADS_SECRET', getenv( 'S3_SECRET' ) );
    // Never public: browsers get uploads through the site and its uploads proxy.
    define( 'S3_UPLOADS_OBJECT_ACL', 'private' );
    define( 'S3_UPLOADS_BUCKET_URL', 'https://{{ include "wordpress.host" . }}/wp-content' );
    // The site's own address, whatever the database says.
    define( 'WP_HOME', 'https://{{ include "wordpress.host" . }}' );
    define( 'WP_SITEURL', 'https://{{ include "wordpress.host" . }}' );
    // TLS alone is not checked: PHP verifies MariaDB's certificate (against
    // files/php.ini's openssl.cafile) only with VERIFY_SERVER_CERT - tested 2026-10-03.
    define( 'MYSQL_CLIENT_FLAGS', MYSQLI_CLIENT_SSL | MYSQLI_CLIENT_SSL_VERIFY_SERVER_CERT );
    // Plugins and themes come from the values, WordPress itself from the image.
    define( 'DISALLOW_FILE_MODS', true );
    define( 'DISALLOW_FILE_EDIT', true );
    define( 'AUTOMATIC_UPDATER_DISABLED', true );
    // Scheduled tasks run from the CronJob wp-cron, not on page loads (cronjob.yaml).
    define( 'DISABLE_WP_CRON', true );
    // Mail leaves through mailomat (README): WP Mail SMTP takes its connection from
    // these constants instead of its database settings. Encryption and auth are set
    // because the plugin's defaults for SMTP are none and off. The sender (From) stays
    // in wp-admin.
    define( 'WPMS_ON', true );
    define( 'WPMS_MAILER', 'smtp' );
    define( 'WPMS_SMTP_HOST', 'smtp.mailomat.cloud' );
    define( 'WPMS_SMTP_PORT', 587 );
    define( 'WPMS_SSL', 'tls' );
    define( 'WPMS_SMTP_AUTH', true );
    define( 'WPMS_SMTP_USER', getenv( 'SMTP_USERNAME' ) );
    define( 'WPMS_SMTP_PASS', getenv( 'SMTP_PASSWORD' ) );
    {{- with .Values.objectCache.database }}
    // The object cache in valkey-cache, the site's own database there (values objectCache),
    // through Predis - the image has no phpredis. Graceful: while Valkey cannot be reached the
    // site runs from its database, slower, instead of showing an error page; what changes in
    // that time misses the cache (components/valkey-cache, "After a cache outage"). The init
    // container installs the drop-in, so the plugin must not replace it.
    define( 'WP_REDIS_CLIENT', 'predis' );
    define( 'WP_REDIS_HOST', 'valkey-cache.valkey-cache.svc' );
    define( 'WP_REDIS_PASSWORD', [ '{{ $.Release.Name }}', getenv( 'VALKEY_PASSWORD' ) ] );
    define( 'WP_REDIS_DATABASE', {{ . }} );
    define( 'WP_REDIS_GRACEFUL', true );
    define( 'WP_REDIS_DISABLE_DROPIN_AUTOUPDATE', true );
    {{- end }}
    {{- if .Values.pageCache.enabled }}
    // The page cache (values pageCache, README "Page cache"): Proxy Cache Purge sends its PURGE
    // to Varnish in this pod.
    define( 'VHP_VARNISH_IP', '127.0.0.1:8080' );
    {{- end }}
    {{- with .Values.oidc.clientId }}
    // Login through Zitadel (values oidc, README "Login through Zitadel"): OpenID Connect Generic
    // Client with the button beside the normal form, which stays for the break-glass admin.
    // Users are recognised by Zitadel's sub only, never linked by email - another
    // organisation's admin could give a user anyone's address; an existing account is linked
    // once by hand. Roles, the email check and PKCE are mu-plugins/oidc.php.
    define( 'OIDC_CLIENT_ID', '{{ . }}' );
    define( 'OIDC_CLIENT_SECRET', getenv( 'OIDC_CLIENT_SECRET' ) );
    define( 'OIDC_CLIENT_SCOPE', 'openid profile email' );
    define( 'OIDC_ENDPOINT_LOGIN_URL', 'https://auth.d3strukt0r.dev/oauth/v2/authorize' );
    define( 'OIDC_ENDPOINT_TOKEN_URL', 'https://auth.d3strukt0r.dev/oauth/v2/token' );
    define( 'OIDC_ENDPOINT_USERINFO_URL', 'https://auth.d3strukt0r.dev/oidc/v1/userinfo' );
    define( 'OIDC_ENDPOINT_LOGOUT_URL', 'https://auth.d3strukt0r.dev/oidc/v1/end_session' );
    // Without it the plugin does not check the ID token's signature.
    define( 'OIDC_ENDPOINT_JWKS_URL', 'https://auth.d3strukt0r.dev/oauth/v2/keys' );
    define( 'OIDC_LOGIN_TYPE', 'button' );
    define( 'OIDC_LINK_EXISTING_USERS', 0 );
    define( 'OIDC_CREATE_IF_DOES_NOT_EXIST', 1 );
    {{- end }}
{{- end -}}
