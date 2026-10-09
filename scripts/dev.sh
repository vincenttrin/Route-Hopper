#!/usr/bin/env bash
# Runs the Go backend and the Vite dev server natively, so traceroute sees real
# TTL-exceeded replies (Docker Desktop on macOS and Windows drops them).
# Usage: scripts/dev.sh            (backend :8080, frontend http://localhost:5173)
#        BACKEND_PORT=8099 scripts/dev.sh
# GEOIP_CITY_DB, GEOIP_ASN_DB and the other backend variables pass through.
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
