#!/bin/sh
# Compares the upload-scan plugin with PHP (README.md): Traefik with the plugin from this checkout,
# a stand-in clamd and PHP in Docker, then the crafted and the random uploads. Needs Docker and
# Python 3. SEEDS and COUNT choose the random bodies.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
plugin=$(dirname "$here")
prefix=upload-scan-compare
port=${PORT:-18081}

cleanup() {
  docker rm -f "$prefix-clamd" "$prefix-php" "$prefix-traefik" >/dev/null 2>&1 || true
  docker network rm "$prefix" >/dev/null 2>&1 || true
}
trap cleanup EXIT
cleanup

docker network create "$prefix" >/dev/null
docker run -d --name "$prefix-clamd" --network "$prefix" --network-alias clamd \
  -v "$here/fakeclamd.py:/fakeclamd.py:ro" python:3-alpine python3 -I /fakeclamd.py >/dev/null
# 50 uploads per request instead of PHP's 20, so the 25-file cases store every file.
docker run -d --name "$prefix-php" --network "$prefix" --network-alias php \
  -v "$here/index.php:/app/index.php:ro" php:8.3-cli \
  php -d display_errors=0 -d max_file_uploads=50 -S 0.0.0.0:8000 /app/index.php >/dev/null
# The Traefik version k3s ships (kubectl --context d3strukt0r-prod-admin -n kube-system get ds
# traefik); keep them in step.
docker run -d --name "$prefix-traefik" --network "$prefix" -p "127.0.0.1:$port:80" \
  -v "$here/traefik.yml:/etc/traefik/traefik.yml:ro" -v "$here/dynamic.yml:/etc/traefik/dynamic.yml:ro" \
  -v "$plugin:/plugins-local/src/github.com/team-maro/uploadscan:ro" traefik:v3.7.15 >/dev/null

tries=0
until docker logs "$prefix-traefik" 2>&1 | grep -q 'Plugins loaded'; do
  tries=$((tries + 1))
  if [ "$tries" -gt 30 ]; then
    echo "Traefik did not load the plugin:" >&2
    docker logs "$prefix-traefik" >&2
    exit 1
  fi
  sleep 1
done

failed=0
PORT=$port PREFIX=$prefix python3 -I "$here/crafted.py" || failed=1
for seed in ${SEEDS:-7 23 101 211}; do
  PORT=$port PREFIX=$prefix SEED=$seed COUNT=${COUNT:-400} MISSES=${MISSES:-} \
    python3 -I "$here/random_bodies.py" || failed=1
done
exit $failed
