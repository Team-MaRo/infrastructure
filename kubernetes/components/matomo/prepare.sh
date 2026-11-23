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
EOF
# The importer's encryption_key, only while a Google Analytics import runs (README "Google
# Analytics history"); without it the plugin would generate one into this file, lost at the next
# start together with the credentials it encrypted.
if [ -n "${MATOMO_GA_IMPORTER_KEY:-}" ]; then
  printf '\n[GoogleAnalyticsImporter]\nencryption_key = "%s"\n' "$MATOMO_GA_IMPORTER_KEY" >> config/config.ini.php
fi

# Installs the newest stable version of a Marketplace plugin within a range (README "Upgrades"),
# read as npm does: ~X.Y.Z and ~X.Y take patches, ~X minors; ^X.Y.Z minors and patches, below
# 1.0.0 only what the first non-zero part allows (^0.2.3 patches).
plugin_install() {
  version=$(curl -fsSL --retry 3 "https://plugins.matomo.org/api/2.0/plugins/$1/info" | php -r '
    if (!preg_match("/^([~^])(\d+)(?:\.(\d+))?(?:\.(\d+))?$/", $argv[1], $m)) exit(2);
    [$op, $parts] = [$m[1], array_map("intval", array_slice(array_filter($m, "strlen"), 2))];
    $min = implode(".", array_pad($parts, 3, 0));
    // The part that may not change: ~ keeps the minor when given, ^ the first non-zero one.
    $keep = $op === "~" ? min(count($parts), 2) - 1 : count($parts) - 1;
    if ($op === "^") {
      foreach ($parts as $i => $part) {
        if ($part !== 0) { $keep = $i; break; }
      }
    }
    $upper = array_slice($parts, 0, $keep + 1);
    $upper[$keep]++;
    $below = implode(".", array_pad($upper, 3, 0));
    $best = null;
    foreach (json_decode(stream_get_contents(STDIN), true)["versions"] ?? [] as $v) {
      $n = $v["name"];
      if (preg_match("/^\d+\.\d+\.\d+$/", $n) && version_compare($n, $min, ">=")
        && version_compare($n, $below, "<") && ($best === null || version_compare($n, $best, ">"))) {
        $best = $n;
      }
    }
    if ($best === null) exit(1);
    echo $best;
  ' "$2") || { echo "$1: no version in $2 on the Marketplace (a range is ~ or ^ and X, X.Y or X.Y.Z)"; exit 1; }
  echo "$1 $version ($2)"
  curl -fsSL --retry 3 -o "/tmp/$1.zip" "https://plugins.matomo.org/api/2.0/plugins/$1/download/$version"
  rm -rf "plugins/$1"
  php -r '$zip = new ZipArchive(); if ($zip->open($argv[1]) !== true || !$zip->extractTo("plugins")) exit(1);' "/tmp/$1.zip"
}

# GoogleAnalyticsImporter (README "Google Analytics history"), in the archive pod as well: the
# imported days have reports but no raw visits, and only the loaded plugin stops Matomo from
# rebuilding such a day from nothing if it is ever invalidated. 6.x needs Matomo 6.
plugin_install GoogleAnalyticsImporter '^5.2.4'

# Archiving (which also sends the scheduled email reports) needs neither the login nor locations,
# and the web pod updates the database alone - and activates the plugins only after that update.
if [ "${1:-}" = archive ]; then
  php console plugin:activate GoogleAnalyticsImporter --no-interaction
  exit 0
fi

# RebelOIDC, the Zitadel login (README "Login through Zitadel"); patches only, since a minor can
# change its security checks and settings (5.2).
plugin_install RebelOIDC '~5.2.0'

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
