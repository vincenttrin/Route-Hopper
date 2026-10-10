# traffic-visualizer backend

Go API that runs the system `traceroute`, enriches each hop with reverse DNS and
geolocation, and returns JSON for the frontend map.

## Run

On macOS (and anywhere real traces matter during development) run the backend and the
Vite dev server natively. From the repo root:

```bash
scripts/dev.sh           # backend :8080, frontend http://localhost:5173
BACKEND_PORT=8099 scripts/dev.sh   # if 8080 is taken
```

The script installs the frontend dependencies on first run and stops both processes
on Ctrl-C. It points the backend at the databases in `./geoip` (or `$GEOIP_DIR`) and has
the backend download them there on first start and keep them fresh (`GEOIP_AUTO_UPDATE`,
see [Geolocation database](#geolocation-database)). Other backend
variables such as `GEOIP_CITY_DB` pass through. The system `traceroute` that ships with
macOS works unprivileged, so no sudo is needed.

To run only the backend:

```bash
go run .                 # http://localhost:8080
go test -race ./...
```

Requires `traceroute` on the PATH (`traceroute6` for IPv6 targets).

## API

`POST /api/trace` with `{ "endpoint": "example.com", "maxHops": 30 }`. The endpoint
may be a host name, IP, or URL. `maxHops` is optional (default 30, max 64).

```json
{
  "hops": [
    { "hopNumber": 1, "ip": "192.168.1.1", "hostname": "", "lat": 0, "lng": 0, "rtt": 3.1 },
    { "hopNumber": 2, "ip": "", "hostname": "", "lat": 0, "lng": 0, "rtt": 0 },
    { "hopNumber": 3, "ip": "1.0.0.1", "hostname": "one.one.one.one", "city": "Sydney",
      "country": "AU", "org": "Cloudflare", "lat": -33.86, "lng": 151.2, "rtt": 25.8 }
  ],
  "destination": { "ip": "1.0.0.1", "city": "Sydney", "lat": -33.86, "lng": 151.2 },
  "reached": true
}
```

- `reached` is false when the destination itself never answered a probe, so the last
  hops are `ip: ""`. That is not an error: many hosts and firewalls drop traceroute
  probes. The trace first uses the default UDP probes; if the destination does not
  answer, it runs once more with ICMP echo (`traceroute -I`), which hosts such as
  facebook.com answer, and keeps that result if it got through. ICMP needs
  `CAP_NET_RAW` on Linux (macOS allows it unprivileged); without it the UDP result stands.
- `warning` (optional string) is present when the trace is probably incomplete or
  cannot be drawn: at least three hops were traced and none past the first replied (see
  [Docker](#docker)), or public hops replied but none could be geolocated (no GeoIP
  database, see [Geolocation database](#geolocation-database)). The frontend shows it
  above the results.

- A hop that never answered has an empty `ip`; a hop without geolocation (private
  addresses, unknown ranges) has `lat` and `lng` of 0.
- `rtt` is the mean of the answered probes in milliseconds.
- The UDP pass is limited to 60 seconds and the ICMP fallback pass gets its own 15 seconds, so a trace takes at most about 75 seconds. If a limit is hit, the hops collected so far are returned.
- Errors are plain text: 400 invalid input, 403 non-public target, 422 unresolvable
  host, 429 rate limited, 503 too many concurrent traces, 504 nothing traced in time.

### Streaming: `POST /api/trace/stream`

Same request, but the response is [NDJSON](https://github.com/ndjson/ndjson-spec)
(`application/x-ndjson`): one JSON object per line, written and flushed as the trace
progresses, so a client can draw each hop the moment traceroute prints it. It is NDJSON
rather than server-sent events because the request needs a POST body, which `EventSource`
cannot send; `fetch` reads the stream directly. Every line has a `type`:

| `type` | Fields | When |
| --- | --- | --- |
| `start` | `destination` | First line, once the target is resolved and a trace slot is taken |
| `hop` | `hop` (same shape as in `hops` above, already named and located) | Each hop, in order, as it is parsed |
| `phase` | `phase: "icmp"` | The destination did not answer UDP probes and the ICMP retry begins |
| `reset` | | The ICMP retry reached the destination: discard the hops so far; its hops follow |
| `done` | `destination`, `reached`, `warning` (optional) | Last line of a finished trace |
| `error` | `error`, `status` | The trace failed after streaming began; `status` is the HTTP status the plain endpoint would have used |

Requests that are rejected before streaming begins (400, 403, 422, 429, 503) are
ordinary plain-text HTTP errors, exactly as for `POST /api/trace`. Both endpoints share
the rate limit and the concurrent trace cap, and keep the same time limits. During the
ICMP retry the ICMP hops are held back and sent after `reset` only if they got further
than the UDP hops. Closing the connection stops the traceroute process. Behind a proxy,
disable response buffering for this path (the response sets `X-Accel-Buffering: no`; the
bundled `frontend/nginx.conf` also turns `proxy_buffering` off).

### `GET /healthz` and `GET /api/geoip`

`GET /healthz` returns `{"status":"ok","geoip":"ready"}`. The server is healthy without a
location database (it may still be downloading, or the download failed), in which case
`geoip` is `"missing"`: traces work, hops just have no coordinates.

`GET /api/geoip` (reachable through the frontend proxy) says which database is loaded, for the
"database last updated" note in the UI:

```json
{ "available": true, "provider": "DB-IP Lite", "updated": "2026-10-01" }
```

`updated` is the build date of the City database (UTC). Without a database it is just
`{ "available": false }`. It changes by itself when the databases are refreshed.

## Geolocation database

The backend can fetch and refresh its own databases, so there is nothing to set up. With
`GEOIP_AUTO_UPDATE=true` (the default in the Docker image and in `scripts/dev.sh`; `false` when
you run the binary yourself):

- **First start.** If the files at `GEOIP_CITY_DB` and `GEOIP_ASN_DB` are missing, the server
  starts at once without locations and downloads them in the background (about 140 MB; 10
  to 30 seconds). The databases are hot-swapped in when they are ready, with no restart.
  The log says `geoip: downloading ...` then `geoip: installed ...`.
- **Refresh.** A check runs daily; once the City file is 30 days old (DB-IP publishes
  monthly, early in the month, falling back to the previous month's file until then), both
  databases are downloaded, opened and checked (they must be the expected City and ASN
  types), and only then renamed into place and swapped in atomically while requests are being
  served. If the new build is not newer than the one in use it is dropped.
- **Failure.** Any failure (offline, HTTP error, corrupt or wrong download) is logged and the
  databases in use stay as they are; the files on disk are untouched. The retry comes hourly
  while there is no database at all, and daily otherwise. The server stays healthy throughout
  and `/healthz` reports `geoip: missing` until the first database is in.
- **Source.** Keyless DB-IP Lite (CC BY 4.0) by default; with `MAXMIND_LICENSE_KEY` set (a free
  key), MaxMind GeoLite2. The key never appears in logs. Files are always saved as
  `GeoLite2-City.mmdb` and `GeoLite2-ASN.mmdb`, the names the paths default to.

The same download code is a command, for native use or one-off refreshes. From the repo root:

```bash
scripts/fetch-geoip.sh           # writes ./geoip (or $GEOIP_DIR); --force refreshes
```

which runs `go run . fetch-geoip [--force]` in `backend/` (so it needs Go). Existing files are
kept unless `--force` (or `FORCE=1`); it exits non-zero with a message on any failure.
`MAXMIND_LICENSE_KEY=... scripts/fetch-geoip.sh --force` switches to MaxMind. The files are
git-ignored. Code: `geo/source.go` (downloads), `geo/update.go` (the update and the schedule),
`geo/swap.go` (the hot swap).

Point a native server at the files:

```bash
GEOIP_CITY_DB=../geoip/GeoLite2-City.mmdb GEOIP_ASN_DB=../geoip/GeoLite2-ASN.mmdb GEOIP_AUTO_UPDATE=true go run .
```

`scripts/dev.sh` sets these for you. Run directly, the defaults are `GeoLite2-City.mmdb`
and `GeoLite2-ASN.mmdb` in the working directory, so from `backend/` they are not found
unless you set the variables as above. Without a City database the server still runs, with every
`lat`/`lng` at 0. A database that exists but cannot be opened is treated the same way (logged,
and replaced by the next refresh).

**Attribution:** the DB-IP Lite databases are licensed
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/) and require attribution. The
map credits "IP geolocation by DB-IP" and the page footer repeats it with the database's
last-updated date; keep that credit if you redistribute the app.

## Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `PORT` | `8080` | Listen port |
| `GEOIP_CITY_DB` / `GEOIP_ASN_DB` | see above | Database paths |
| `GEOIP_AUTO_UPDATE` | `false` (`true` in the Docker image) | Download missing databases and refresh them monthly |
| `MAXMIND_LICENSE_KEY` | unset | Use MaxMind GeoLite2 instead of DB-IP Lite |
| `ALLOW_PRIVATE_TARGETS` | `false` | Allow tracing loopback/private addresses (local dev only) |
| `MAX_CONCURRENT_TRACES` | `4` | Traces running at once |
| `RATE_LIMIT_PER_MINUTE` | `20` | Per client address, burst of 5 |

The rate limiter keys on the connection's remote address, so behind a reverse proxy
it limits the proxy as a whole. Terminate limits at the proxy or add trusted
forwarded-header handling before exposing this directly.

## Docker

From the repo root, `docker compose up --build` serves the frontend on
http://localhost:5173 (`PORT` to change it) and proxies `/api` to the backend, which is
not published on the host (the Vite dev server uses the same port, so stop it first). The backend downloads the location databases itself on first
start (see [Geolocation database](#geolocation-database)).

The databases live in the `geoip` named volume mounted at `/data`, so they survive restarts
and `docker compose up --build`; `docker compose down -v` deletes them (they download again).
To use a host folder instead, set `GEOIP_DIR=./geoip`: the backend runs as uid 10001, so on
Linux the folder must be writable by it (`chown 10001 geoip`), or it cannot save its downloads
(it logs the error and keeps running without locations). Pass `MAXMIND_LICENSE_KEY` in the
environment to use GeoLite2, or `GEOIP_AUTO_UPDATE=false` to manage the files yourself.

Docker Desktop on macOS and Windows runs containers behind a NAT that drops the
"TTL exceeded" replies traceroute depends on, so traces from there show the container's
gateway as hop 1 and then no replies. The backend detects this (no reply from any hop
past the first) and returns a `warning`, which the UI shows as a banner and the server
logs. Use a Linux host for real traces, or run the backend natively with
`scripts/dev.sh` (see [Run](#run)). A firewall that drops all TTL-exceeded replies
triggers the same warning.

Backend image on its own:

```bash
docker build -t traffic-visualizer-backend .
docker run --rm -p 8080:8080 -v geoip:/data traffic-visualizer-backend
```

The image runs as a non-root user. The system traceroute uses UDP probes and does not
need extra capabilities in the tested setups; if your runtime blocks it, add
`--cap-add=NET_RAW`.
