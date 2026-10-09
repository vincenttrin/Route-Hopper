# traffic-visualizer

Enter a host name, IP, or URL and the app runs a traceroute to it, looks up each hop's
location and network, and draws the route as a line diagram next to a map. The first
load shows an example trace (Omaha to Amsterdam) so you can see the layout before
tracing anything.

- **Frontend:** React, Leaflet (OpenStreetMap tiles), Vite
- **Backend:** Go REST API that shells out to the system `traceroute`
- **Geolocation:** optional MaxMind GeoLite2 (or DB-IP City Lite) `.mmdb` files

## Prerequisites

| For | You need |
| --- | --- |
| Native run | Node.js 18+, Go (version in `backend/go.mod`), `traceroute` on the PATH |
| Docker | Docker with the Compose plugin |

## Quickstart: native

Run the backend and the frontend dev server in two terminals.

```bash
# Terminal 1: API on http://localhost:8080
cd backend
go run .

# Terminal 2: UI on http://localhost:5173 (proxies /api to localhost:8080)
cd frontend
npm install
npm run dev
```

Open http://localhost:5173 and enter an endpoint. Without a GeoIP database the trace
works but no hops are placed on the map; see
[backend/README.md](backend/README.md#geolocation-database) for setting one up.

## Quickstart: Docker Compose

```bash
docker compose up --build
```

The UI is served on http://localhost:3000. nginx in the frontend container proxies
`/api` to the backend, which is not published on the host. To add geolocation, put the
GeoLite2 `.mmdb` files in `./geoip` (or point `GEOIP_DIR` elsewhere).

Docker Desktop on macOS and Windows drops the replies traceroute relies on, so traces
there show only the first hop. Use a Linux host, or run natively. Details are in
[backend/README.md](backend/README.md#docker).

## Ports and environment variables

| Port | Service |
| --- | --- |
| 8080 | Backend API (native run; `PORT` to change) |
| 5173 | Vite dev server |
| 3000 | Frontend in Docker Compose (`PORT` to change) |

| Variable | Default | Used by |
| --- | --- | --- |
| `PORT` | `8080` native, `3000` compose | Backend listen port; compose host port for the UI |
| `GEOIP_CITY_DB`, `GEOIP_ASN_DB` | `GeoLite2-City.mmdb`, `GeoLite2-ASN.mmdb` in the working directory | Backend database paths |
| `GEOIP_DIR` | `./geoip` | Compose: host directory mounted at `/data` |
| `ALLOW_PRIVATE_TARGETS` | `false` | Allow tracing loopback/private addresses (local dev only) |
| `MAX_CONCURRENT_TRACES` | `4` | Backend: traces running at once |
| `RATE_LIMIT_PER_MINUTE` | `20` | Backend: per client address, burst of 5 |

The full list, API reference, and error codes are in [backend/README.md](backend/README.md).

## Tests and linting

```bash
cd backend && go test ./...
cd frontend && npm test && npm run lint
```

## Layout

```
backend/            Go API (main.go, handlers/, trace/, geo/)
frontend/           React app (src/components, src/lib, src/services)
docker-compose.yml  Frontend (nginx) and backend services
CLAUDE.md           Notes for AI coding agents
```
