#!/usr/bin/env bash
# Download a free GeoIP City + ASN database into GEOIP_DIR (default ./geoip).
#
# Default source is DB-IP Lite (keyless, CC BY 4.0). Set MAXMIND_LICENSE_KEY to use
# MaxMind GeoLite2 instead. Files are always saved as GeoLite2-City.mmdb and
# GeoLite2-ASN.mmdb, the names the backend expects.
#
# Usage: scripts/fetch-geoip.sh [--force]
#   Existing files are kept unless --force (or FORCE=1) is given.
#
# The download logic lives in the backend (backend/geo), which also uses it to fetch
# the databases on first start and refresh them monthly (GEOIP_AUTO_UPDATE); this is
# only a command-line entry point to it, so it needs Go. Docker users do not need it.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dir="${GEOIP_DIR:-$root/geoip}"
command -v go >/dev/null || { echo "fetch-geoip: go is required (or let the backend download the databases itself: GEOIP_AUTO_UPDATE=true)" >&2; exit 1; }

cd "$root/backend"
GEOIP_CITY_DB="$dir/GeoLite2-City.mmdb" GEOIP_ASN_DB="$dir/GeoLite2-ASN.mmdb" exec go run . fetch-geoip "$@"
