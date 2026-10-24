#!/bin/sh
# Assembles Matomo in the empty html volume at every start, for the web pod (no argument) and the
# archive job (`archive`): the code from the image, the configuration and the Google Analytics
# importer, and for the web pod also
# the RebelOIDC plugin, the GeoIP database and the database update. Nothing on the volume survives
# the pod; Matomo's state is in its database (README).
set -eu

# The image's tar sets the permissions of the directory it copies into, which only its owner may:
# so www-data creates html itself rather than copying into the root-owned volume.
mkdir -p html
cd html
/entrypoint.sh true

# Only an installed Matomo starts: with an empty database it would show error pages, and the
# installer is off (common.config.ini.php).
status=0
php -r '
  $db = mysqli_init();
  $db->ssl_set(null, null, "/etc/mariadb-ca/ca.crt", null, null);
  $db->real_connect("mariadb-primary.mariadb.svc", "matomo", getenv("MATOMO_DB_PASSWORD"), "matomo", 3306, null, MYSQLI_CLIENT_SSL);
  exit($db->query("SHOW TABLES LIKE \"matomo_option\"")->num_rows === 1 ? 0 : 3);
' || status=$?
case $status in
  0) ;;
  3) echo "Matomo is not installed in its database yet - README \"Installing\""; exit 1 ;;
  *) echo "Matomo's database cannot be reached"; exit 1 ;;
esac

cp /config/common.config.ini.php config/
cat > config/config.ini.php <<EOF
; <?php exit; ?> DO NOT REMOVE THIS LINE
[database]
password = "$MATOMO_DB_PASSWORD"

[General]
salt = "$MATOMO_SALT"

[mail]
password = "$MATOMO_SMTP_PASSWORD"

[GoogleAnalyticsImporter]
encryption_key = "$MATOMO_GA_IMPORTER_KEY"
EOF

# GoogleAnalyticsImporter (README "Google Analytics history"), in the archive pod as well: the
# imported days have reports but no raw visits, and only the loaded plugin stops Matomo from
# rebuilding such a day from nothing if it is ever invalidated. Its OAuth credentials in the
# database are encrypted with the key above, which it would otherwise generate into this file.
gaimporter=5.2.4
curl -fsSL --retry 3 -o /tmp/gaimporter.zip \
  "https://plugins.matomo.org/api/2.0/plugins/GoogleAnalyticsImporter/download/$gaimporter"
rm -rf plugins/GoogleAnalyticsImporter
php -r '$zip = new ZipArchive(); if ($zip->open("/tmp/gaimporter.zip") !== true || !$zip->extractTo("plugins")) exit(1);'

# Archiving (which also sends the scheduled email reports) needs neither the login nor locations,
# and the web pod updates the database alone - and activates the plugins only after that update.
if [ "${1:-}" = archive ]; then
  php console plugin:activate GoogleAnalyticsImporter --no-interaction
  exit 0
fi

# RebelOIDC, the Zitadel login (README "Login through Zitadel"), with one line patched: it decodes
# the tokens as plain base64 instead of base64url, so a login fails whenever a token holds - or _
# (upstream issue #47). If a new version no longer has that line, the start fails here, so the
# patch is reviewed rather than silently skipped.
rebeloidc=5.1.6
curl -fsSL --retry 3 "https://github.com/Digitalist-Open-Cloud/Matomo-Plugin-RebelOIDC/archive/refs/tags/$rebeloidc.tar.gz" | tar -xz -C /tmp
rm -rf plugins/RebelOIDC
mv "/tmp/Matomo-Plugin-RebelOIDC-$rebeloidc" plugins/RebelOIDC
sed -i 's|base64_decode($parts\[1\])|base64_decode(strtr($parts[1], "-_", "+/"))|' plugins/RebelOIDC/Controller.php
grep -q 'base64_decode(strtr($parts\[1\], "-_", "+/"))' plugins/RebelOIDC/Controller.php \
  || { echo "RebelOIDC $rebeloidc: the base64url patch did not apply - check upstream issue #47"; exit 1; }

# The visitors' locations (README "GeoIP"), from the bucket the geoip CronJob fills; if the bucket
# fails, this month's file straight from DB-IP, as the CronJob fetches it. Without either Matomo
# still runs and guesses the country from the browser's language.
if ! curl -fsS --retry 3 --aws-sigv4 "aws:amz:nbg1:s3" --user "$S3_ACCESS_KEY_ID:$S3_SECRET_ACCESS_KEY" \
  -H "x-amz-content-sha256: UNSIGNED-PAYLOAD" -o misc/DBIP-City-Lite.mmdb \
  https://d3strukt0r-prod-matomo.nbg1.your-objectstorage.com/geoip/DBIP-City-Lite.mmdb; then
  echo "WARNING: no GeoIP database in the bucket - fetching it from DB-IP"
  rm -f misc/DBIP-City-Lite.mmdb
  if ! { curl -fsSL --retry 3 -o misc/DBIP-City-Lite.mmdb.gz \
    "https://download.db-ip.com/free/dbip-city-lite-$(date -u +%Y-%m).mmdb.gz" \
    && gunzip misc/DBIP-City-Lite.mmdb.gz; }; then
    rm -f misc/DBIP-City-Lite.mmdb misc/DBIP-City-Lite.mmdb.gz
    echo "WARNING: no GeoIP database from DB-IP either - Matomo runs without visitor locations"
  fi
fi

# A new Matomo from the image updates its tables before it serves.
php console core:update --yes --no-interaction
# Activation is recorded in config.ini.php, which was just written afresh.
php console plugin:activate RebelOIDC GoogleAnalyticsImporter --no-interaction
