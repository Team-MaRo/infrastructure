{{/* The site's own address: the first of its hosts. */}}
{{- define "wordpress.host" -}}
{{- required "hosts needs at least one name" (first .Values.hosts) -}}
{{- end -}}

{{/* The database and its user: the site's name, which MariaDB cannot hold with hyphens. */}}
{{- define "wordpress.database" -}}
{{- .Release.Name | replace "-" "_" -}}
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
{{- end -}}
