# Route Hopper (traffic-visualizer)

Enter a host name, IP, or URL and a bunny follows the route a packet takes to it. The
app runs a traceroute, looks up each hop's location and network, and draws the route as
a line diagram next to a map, with the bunny hopping from hop to hop on both. On the map
the route is drawn as arcs and the bunny bounces along them (the arcs are only for show;
distances are measured along straight great-circle legs). The first load shows an example
trace (Chicago to Amsterdam) so you can see the layout before tracing anything.

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

The **score** is relative: the ratio of the smaller of your guess and the actual distance
to the larger. A guess within 10 km of the actual distance (over or under, always measured in
km whatever unit you type in) scores a full 100.

```
score = 100                                            for |guess - actual| <= 10 km
score = round(100 * min(guess, actual) / max(guess, actual))   otherwise
```

For example, a guess of 10000 km for an actual 13146 km scores 76, and so does a guess of
13146 km for an actual 10000 km. Over- and undershooting are scored symmetrically, and the
score always stays between 0 and 100. The functions are in `frontend/src/lib/game.js` with tests.

A round is not scored, and says why, when:

- fewer than two hops have a location, or the located hops are all in the same spot
  (under 1 km of route): there is no distance to guess, so the panel says so instead of
  asking;
- the trace is cancelled, or the connection to the backend is lost before it finishes.

## Sharing a trip

When a round is finished (scored, or too short to guess) a **Share trip** button appears next
to **Play again**. It shares a short, friendly summary: the destination, how many hops, the
total distance, the networks crossed, the countries the route passed through, and your guess
and score if you played. For example:

```
🐰 I followed a packet to example.com (Amsterdam, Netherlands)!
It hopped 9 times and travelled about 7,200 km across 4 networks, passing through United States, Ireland, United Kingdom, and Netherlands.
I guessed 8,000 km and scored 90/100. Can you beat my bunny score? 🥕
```

The browser's share sheet is used where there is one (Web Share API, so phones and recent
desktop browsers); otherwise the text is copied to the clipboard, and if even that is blocked it
is shown for you to copy by hand. A link to the app is added unless it runs on `localhost`.
The summary is built in `frontend/src/lib/share.js`. It only uses the destination you typed
(host name only, never a path or query) and coarse facts, never hop addresses or host names.

## Privacy: the start of the route is hidden

The trace starts from wherever the backend runs, and the first hops of a traceroute describe
that network: the LAN gateway, then the ISP's routers around it, whose addresses, host names
(they usually carry a city code) and locations would say roughly where the server or you are.
So the **backend** cuts them out before anything reaches the browser. They are replaced by one
**hidden start** hop, shown as "Hidden start / Private", with no address, name, location or
latency, and the remaining hops are renumbered from 2, so even the number of hidden hops is
not revealed. The distance, the map and the share text only use the hops that remain.

What counts as the start: every hop before the first public address (private, loopback and
carrier-grade NAT addresses, silent hops), the first public hop, and the following hops that
belong to the same network as the first public hop (same network owner, or same host name
domain) or are located within 100 km of it. The destination is never hidden. Details and
limits are in [backend/README.md](backend/README.md#privacy-the-start-of-the-route-is-hidden).
The first visible hop is where the trace leaves that network, so its country and
metro area still show; and with no GeoIP database nothing says which hops belong to the
start, so only the first public hop is hidden.

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

The UI is served on http://localhost:5173. nginx in the frontend container proxies
`/api` to the backend, which is not published on the host.

**Location database:** nothing to set up. On first start the backend downloads the free
DB-IP Lite City and ASN databases into the `geoip` volume (about 140 MB, 10 to 30 seconds)
and starts placing hops on the map as soon as they are in. The volume keeps them across
restarts. They are refreshed by themselves: the backend checks daily and downloads new files
once the ones it has are 30 days old (the publishers release monthly), validates them, and
swaps them in without a restart. A failed download, say when offline, is logged and retried
(hourly while there is no database, daily otherwise); the server stays healthy and keeps
the databases it has, and without any it still traces, just without a map. The page footer
shows which database is loaded and when it was last updated. To use MaxMind GeoLite2 instead,
set `MAXMIND_LICENSE_KEY` (a free key). To keep the files in a folder of your own,
set `GEOIP_DIR=./geoip`; on Linux that folder must be writable by uid 10001
(`chown 10001 geoip`). Set `GEOIP_AUTO_UPDATE=false` to turn the downloads off and manage the
files yourself. Details are in [backend/README.md](backend/README.md#geolocation-database).

Docker Desktop on macOS and Windows drops the replies traceroute relies on, so traces
there show only the first hop, and the UI warns about it. Use a Linux host, or run natively. Details are in
[backend/README.md](backend/README.md#docker).

## Ports and environment variables

| Port | Service |
| --- | --- |
| 8080 | Backend API (native run; `PORT` to change) |
| 5173 | Vite dev server (native run) or frontend in Docker Compose (`PORT` to change); the two cannot run at the same time on one host |

| Variable | Default | Used by |
| --- | --- | --- |
| `PORT` | `8080` native, `5173` compose | Backend listen port; compose host port for the UI |
| `GEOIP_CITY_DB`, `GEOIP_ASN_DB` | `GeoLite2-City.mmdb`, `GeoLite2-ASN.mmdb` in the working directory | Backend database paths |
| `GEOIP_AUTO_UPDATE` | `true` in Docker and `scripts/dev.sh`, else `false` | Backend: download the GeoIP databases when missing and refresh them monthly |
| `MAXMIND_LICENSE_KEY` | unset | Backend: use MaxMind GeoLite2 instead of keyless DB-IP Lite |
| `GEOIP_DIR` | the `geoip` named volume | Compose: volume or host directory mounted at `/data` |
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
backend/            Go API (main.go, handlers/, trace/, geo/); geo/ also downloads and refreshes the databases
frontend/           React app (src/components, src/lib, src/services)
docker-compose.yml  Frontend (nginx) and backend services
CLAUDE.md           Notes for AI coding agents
```
