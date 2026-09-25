#!/usr/bin/env bash
set -euo pipefail

# Checks the actual runtime image, using the repository's hello-world plugin.
# All data and ports are temporary; no provider credentials are needed.
image=${1:?Usage: test-dynamic-image.sh IMAGE PLUGIN_SO}
plugin=${2:?Usage: test-dynamic-image.sh IMAGE PLUGIN_SO}
plugin="$(cd "$(dirname "$plugin")" && pwd)/$(basename "$plugin")"
test -f "$plugin"
scratch=$(mktemp -d)
container=
cleanup() {
  result=$?
  if [[ -n "$container" ]]; then
    if [[ $result != 0 ]]; then docker logs "$container" >&2 || true; fi
    docker rm -fv "$container" >/dev/null || true
  fi
  rm -rf "$scratch"
  exit "$result"
}
trap cleanup EXIT

cat > "$scratch/config.json" <<'JSON'
{"plugins":[{"name":"hello-world","enabled":true,"path":"/test/hello-world.so","config":{}}]}
JSON
chmod 755 "$scratch"
chmod 644 "$scratch/config.json"
container=$(docker run -d -p 127.0.0.1::8080 \
  --mount "type=bind,src=$plugin,dst=/test/hello-world.so,readonly" \
  --mount "type=bind,src=$scratch/config.json,dst=/app/data/config.json,readonly" \
  "$image")
port=$(docker port "$container" 8080/tcp | sed 's/.*://')
url="http://127.0.0.1:$port"
curl --fail --silent --show-error --retry 30 --retry-delay 2 --retry-all-errors \
  --max-time 5 --retry-max-time 120 "$url/health" > /dev/null
curl --fail --silent --show-error "$url/api/plugins/loaded" | \
  jq -e '.plugins | index("hello-world") != null' > /dev/null
curl --fail --silent --show-error "$url/" > "$scratch/index.html"
# Check a real frontend asset, not just a placeholder returning HTTP 200.
asset=$(sed -nE 's/.*<script[^>]*src="([^"]+\.js)".*/\1/p' "$scratch/index.html" | head -1)
[[ "$asset" == /* && "$asset" != //* ]]
curl --fail --silent --show-error "$url$asset" > "$scratch/app.js"
test -s "$scratch/app.js"
echo 'PASS: gateway health, embedded frontend asset, dynamic hello-world plugin'
