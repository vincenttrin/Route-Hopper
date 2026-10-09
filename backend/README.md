# traffic-visualizer backend

Go API that runs the system `traceroute`, enriches each hop with reverse DNS and
geolocation, and returns JSON for the frontend map.

## Run

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
  "destination": { "ip": "1.0.0.1", "city": "Sydney", "lat": -33.86, "lng": 151.2 }
}
```

- A hop that never answered has an empty `ip`; a hop without geolocation (private
  addresses, unknown ranges) has `lat` and `lng` of 0.
- `rtt` is the mean of the answered probes in milliseconds.
- If the 60 second limit is hit, the hops collected so far are returned.
- Errors are plain text: 400 invalid input, 403 non-public target, 422 unresolvable
  host, 429 rate limited, 503 too many concurrent traces, 504 nothing traced in time.

`GET /healthz` returns `{"status":"ok"}`.

## Geolocation database

Download GeoLite2 City (and optionally ASN, for the `org` field) from MaxMind
(free account and license key required) and point the server at the files:

```bash
GEOIP_CITY_DB=/path/GeoLite2-City.mmdb GEOIP_ASN_DB=/path/GeoLite2-ASN.mmdb go run .
```

Defaults are `GeoLite2-City.mmdb` and `GeoLite2-ASN.mmdb` in the working directory.
Without a City database the server still runs, with every `lat`/`lng` at 0. DB-IP
"City Lite" files also work for coordinates. In Docker, mount the files at `/data`.

## Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `PORT` | `8080` | Listen port |
| `GEOIP_CITY_DB` / `GEOIP_ASN_DB` | see above | Database paths |
| `ALLOW_PRIVATE_TARGETS` | `false` | Allow tracing loopback/private addresses (local dev only) |
| `MAX_CONCURRENT_TRACES` | `4` | Traces running at once |
| `RATE_LIMIT_PER_MINUTE` | `20` | Per client address, burst of 5 |

The rate limiter keys on the connection's remote address, so behind a reverse proxy
it limits the proxy as a whole. Terminate limits at the proxy or add trusted
forwarded-header handling before exposing this directly.

## Docker

```bash
docker build -t traffic-visualizer-backend .
docker run --rm -p 8080:8080 -v /path/to/dbs:/data traffic-visualizer-backend
```

The image runs as a non-root user. The system traceroute uses UDP probes and does not
need extra capabilities in the tested setups; if your runtime blocks it, add
`--cap-add=NET_RAW`.
