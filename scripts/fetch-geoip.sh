#!/usr/bin/env bash
# Download a free GeoIP City + ASN database into GEOIP_DIR (default ./geoip).
#
# Default source is DB-IP Lite (keyless, CC BY 4.0). Set MAXMIND_LICENSE_KEY to use
# MaxMind GeoLite2 instead. Files are always saved as GeoLite2-City.mmdb and
# GeoLite2-ASN.mmdb, the names the backend and docker-compose expect.
#
# Usage: scripts/fetch-geoip.sh [--force]
#   Existing files are kept unless --force (or FORCE=1) is given.
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
dir="${GEOIP_DIR:-$root/geoip}"
force="${FORCE:-0}"
[ "${1:-}" = "--force" ] && force=1

fail() { echo "fetch-geoip: $*" >&2; exit 1; }
command -v curl >/dev/null || fail "curl is required"

if [ "$force" != 1 ] && [ -s "$dir/GeoLite2-City.mmdb" ] && [ -s "$dir/GeoLite2-ASN.mmdb" ]; then
  echo "GeoIP databases already present in $dir (use --force to refresh)"
  exit 0
fi

mkdir -p "$dir"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# download <url> <out>; fails quietly on HTTP errors so callers can fall back.
download() { curl -fsSL --retry 2 --connect-timeout 15 --max-time 600 -o "$2" "$1"; }

# install <file> <name>: refuse empty or non-mmdb files, then move into place atomically.
install_db() {
  [ -s "$1" ] || fail "downloaded $2 is empty"
  LC_ALL=C grep -qa 'MaxMind.com' "$1" || fail "downloaded $2 is not an mmdb file"
  mv "$1" "$dir/$2.part" && mv "$dir/$2.part" "$dir/$2"
  echo "installed $dir/$2"
}

fetch_dbip() {
  command -v gzip >/dev/null || fail "gzip is required"
  local edition out month
  for edition in city:GeoLite2-City asn:GeoLite2-ASN; do
    out="$tmp/${edition#*:}.mmdb"
    # DB-IP publishes monthly; early in a month the new file may not exist yet.
    for month in "$(date -u +%Y-%m)" "$(date -u -d 'last month' +%Y-%m 2>/dev/null || date -u -v-1m +%Y-%m)"; do
      if download "https://download.db-ip.com/free/dbip-${edition%%:*}-lite-$month.mmdb.gz" "$tmp/db.gz"; then
        gzip -dc "$tmp/db.gz" > "$out" || fail "could not decompress DB-IP ${edition%%:*} database"
        install_db "$out" "${edition#*:}.mmdb"
        continue 2
      fi
    done
    fail "could not download DB-IP ${edition%%:*} lite database (network down, or DB-IP changed its URLs)"
  done
  echo "Source: DB-IP Lite (https://db-ip.com), licensed CC BY 4.0. Attribution is shown in the app."
}

fetch_maxmind() {
  command -v tar >/dev/null || fail "tar is required"
  local name
  for name in GeoLite2-City GeoLite2-ASN; do
    download "https://download.maxmind.com/app/geoip_download?edition_id=$name&license_key=$MAXMIND_LICENSE_KEY&suffix=tar.gz" "$tmp/$name.tgz" \
      || fail "MaxMind download of $name failed (check MAXMIND_LICENSE_KEY)"
    mkdir "$tmp/$name" && tar -xzf "$tmp/$name.tgz" -C "$tmp/$name" --strip-components=1 \
      || fail "could not extract $name archive"
    install_db "$tmp/$name/$name.mmdb" "$name.mmdb"
  done
  echo "Source: MaxMind GeoLite2. Review its license terms and attribution requirements."
}

if [ -n "${MAXMIND_LICENSE_KEY:-}" ]; then fetch_maxmind; else fetch_dbip; fi
