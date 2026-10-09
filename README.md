# Route Hopper (traffic-visualizer)

Enter a host name, IP, or URL and a bunny follows the route a packet takes to it. The
app runs a traceroute, looks up each hop's location and network, and draws the route as
a line diagram next to a map, with the bunny hopping from hop to hop on both. On the map
the route is drawn as arcs and the bunny bounces along them (the arcs are only for show;
distances are measured along straight great-circle legs). The first load shows an example
trace (Omaha to Amsterdam) so you can see the layout before tracing anything.

Hops are discovered one at a time and shown at a steady pace: at least 1.2 seconds
apart, however fast the backend finds them, so the hopping is easy to watch. The pace is
a frontend queue (`PACE_MS` in `frontend/src/lib/pace.js`); the real traceroute is not
slowed down. A trace in progress can be cancelled at any moment, and it stops where the
bunny is. With `prefers-reduced-motion` set, the bunny moves without the jump animation
and rows appear without sliding in; the pace stays the same.

## The game

Watch the bunny follow the route first, then guess how far the packet travelled.

1. Enter an endpoint and press **Hop!**. The bunny hops along the route; the real distance
   stays hidden (the distance stat shows `?`).
2. When the bunny gets home, the game panel asks for a guess in km or miles (toggle beside
   the box). Empty, non-numeric, zero, or absurdly large (over 500,000 km) guesses are
   rejected with a message and can be corrected.
3. Press **Guess**. The panel shows your guess, the actual distance, and a score from 0 to
   100, with a message from the bunny. **Play again** clears the round. The best score of
   the session is kept in the page (nothing is stored on the server).

The **actual distance** is the sum of the great-circle distances between consecutive hops
that have a location, in order. Hops without a location (private addresses, no reply, no
database entry) are skipped.

The **score** depends on the relative error `e = |guess - actual| / actual`:

```
score = round(100 * (1 - e) ^ 2)     for e < 1
score = 0                            for e >= 1
```

An exact guess scores 100, 5% off scores 90, 10% off 81, 25% off 56, 50% off 25, and a
guess off by the whole distance or more (such as double the actual distance) scores 0.
Over- and undershooting by the same distance score the same. The functions are in
`frontend/src/lib/game.js` with tests.

A round is not scored, and says why, when:

- fewer than two hops have a location, or the located hops are all in the same spot
  (under 1 km of route): there is no distance to guess, so the panel says so instead of
  asking;
- the trace is cancelled, or the connection to the backend is lost before it finishes.

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

`scripts/dev.sh` starts both in one command (see [backend/README.md](backend/README.md#run)).

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
there show only the first hop, and the UI warns about it. Use a Linux host, or run natively. Details are in
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
