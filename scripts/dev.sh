#!/usr/bin/env bash
# Runs the Go backend and the Vite dev server natively, so traceroute sees real
# TTL-exceeded replies (Docker Desktop on macOS and Windows drops them).
# Usage: scripts/dev.sh            (backend :8080, frontend http://localhost:5173)
#        BACKEND_PORT=8099 scripts/dev.sh
# The GeoIP databases are read from $GEOIP_DIR (default ./geoip, where
# scripts/fetch-geoip.sh puts them); GEOIP_CITY_DB, GEOIP_ASN_DB and the other
# backend variables pass through.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
port="${BACKEND_PORT:-8080}"

command -v traceroute >/dev/null || { echo "traceroute not found on PATH" >&2; exit 1; }
command -v go >/dev/null || { echo "go not found on PATH" >&2; exit 1; }
command -v npm >/dev/null || { echo "npm not found on PATH" >&2; exit 1; }
if nc -z localhost "$port" 2>/dev/null; then
  echo "port $port is already in use; pick another with BACKEND_PORT=<port>" >&2
  exit 1
fi

# The backend runs from backend/, so without explicit paths it would look for the
# databases there instead of where fetch-geoip.sh wrote them, and plot nothing.
geo_dir="${GEOIP_DIR:-$root/geoip}"
export GEOIP_CITY_DB="${GEOIP_CITY_DB:-$geo_dir/GeoLite2-City.mmdb}"
export GEOIP_ASN_DB="${GEOIP_ASN_DB:-$geo_dir/GeoLite2-ASN.mmdb}"
[ -f "$GEOIP_CITY_DB" ] || echo "no GeoIP database at $GEOIP_CITY_DB: traces will show no map. Run scripts/fetch-geoip.sh, then restart." >&2

[ -d "$root/frontend/node_modules" ] || (cd "$root/frontend" && npm install)

pids=()
cleanup() {
  trap - EXIT INT TERM
  for p in "${pids[@]}"; do pkill -TERM -P "$p" 2>/dev/null || true; kill "$p" 2>/dev/null || true; done
}
trap cleanup EXIT INT TERM

(cd "$root/backend" && PORT="$port" exec go run .) &
pids+=($!)
(cd "$root/frontend" && BACKEND_URL="http://localhost:$port" exec npm run dev) &
pids+=($!)

# Stop everything as soon as either process exits (macOS ships bash 3.2, no wait -n).
while kill -0 "${pids[0]}" 2>/dev/null && kill -0 "${pids[1]}" 2>/dev/null; do sleep 1; done
