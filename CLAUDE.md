# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**traffic-visualizer** is a network packet journey analyzer with a React web interface and Go backend. Users input an endpoint, and the application traces the packet route, documents each hop, and visualizes the path on an interactive map.

**Tech Stack:**
- **Frontend:** React + Leaflet (OpenStreetMap) + Vite
- **Backend:** Go + net/http for REST API
- **Containerization:** Docker + Docker Compose
- **Packet Tracing:** System tools (traceroute/mtr)
- **Geolocation:** MaxMind GeoIP2 Lite (free database) or similar IP geolocation service

## Architecture

### High-Level Flow
1. User inputs endpoint URL/IP in React frontend (or views example trace)
2. Frontend sends request to Go backend API
3. Backend executes traceroute/mtr command and captures hop results
4. Backend enriches hop data with geolocation (IP -> coordinates) and network org
5. Backend returns structured hop data as JSON
6. Frontend visualizes: line diagram (transit-map style) on left, geolocation map on right, network legend and stats in sidebar

### Directory Structure
```
traffic-visualizer/
├── frontend/              # React SPA
│   ├── src/
│   │   ├── components/    # React components (LineDiagram, HopMap, TraceInput)
│   │   ├── services/      # API client and sample data
│   │   ├── lib/           # Trace parsing and geo utilities
│   │   ├── App.jsx        # Main app component
│   │   ├── main.jsx       # Entry point
│   │   └── styles.css     # Light-theme styles
│   ├── Dockerfile         # Multi-stage build with nginx
│   ├── nginx.conf         # Proxy config for /api and SPA routing
│   ├── package.json
│   ├── vite.config.js
│   └── index.html
├── backend/               # Go API server
│   ├── main.go
│   ├── handlers/          # HTTP route handlers
│   ├── trace/             # Packet tracing logic (traceroute execution)
│   ├── geo/               # IP geolocation logic
│   ├── README.md          # Backend documentation
│   ├── Dockerfile
│   ├── go.mod
│   └── go.sum
├── docker-compose.yml     # Local dev orchestration
├── CLAUDE.md              # This file
└── README.md              # User-facing documentation
```

### Backend API Endpoints
- `POST /api/trace` - Start packet tracing
  - Request: `{ "endpoint": "example.com", "maxHops": 30 }`
  - Response: `{ "hops": [ { "hopNumber": 1, "ip": "1.2.3.4", "hostname": "...", "city": "...", "country": "...", "lat": 40.7, "lng": -74.0, "rtt": 1.23, "org": "..." }, ... ], "destination": { "ip": "...", "city": "...", "lat": ..., "lng": ... } }`
  - `city`, `country`, and `org` are optional and only present for publicly routable IPs with geolocation data
  - See backend/README.md for full API documentation, error codes, and configuration

## Development Setup

### Prerequisites
- Node.js 18+ (frontend development)
- Go 1.20+ (backend development)
- Docker & Docker Compose (containerization)
- traceroute/mtr installed locally (system tools)

### Initial Setup
```bash
# Clone and navigate
cd /Users/tamyboi/Projects/traffic-visualizer

# Install frontend dependencies
cd frontend && npm install && cd ..

# Download GeoIP database (if using MaxMind GeoLite2)
# Instructions in backend/README.md
```

## Common Development Commands

### Frontend
```bash
# Dev server (http://localhost:5173)
cd frontend && npm run dev

# Build for production
cd frontend && npm run build

# Run linter
cd frontend && npm run lint

# Run tests
cd frontend && npm test
```

### Backend
```bash
# Run local backend server (http://localhost:8080)
cd backend && go run main.go

# Build binary
cd backend && go build -o traffic-visualizer-api

# Run tests
cd backend && go test ./...

# Run tests with coverage
cd backend && go test -cover ./...

# Format code
cd backend && go fmt ./...

# Lint
cd backend && golangci-lint run ./...
```

### Docker & Compose
```bash
# Build and run full stack locally
docker-compose up

# Rebuild images
docker-compose up --build

# Build individual images
docker build -t traffic-visualizer-frontend ./frontend
docker build -t traffic-visualizer-backend ./backend

# Stop and remove containers
docker-compose down
```

## Key Implementation Details

### Packet Tracing (Backend)
- Execute `traceroute` (or `traceroute6` for IPv6) with flags: `-n` (no DNS during trace), `-q 2` (2 probes per hop), `-w 1` (1 second timeout), `-m <maxHops>`
- Parse output to extract hop information (IP, hostname, RTT); handles both Linux and macOS formats
- Enrich hops with concurrent reverse DNS lookups (separate from traceroute, with 2s timeout per lookup)
- Return partial results if the 60 second UDP limit or 15 second ICMP fallback limit is hit; gracefully handle unreachable hops (empty IP, zero coordinates)

### Geolocation (Backend)
- Load MaxMind GeoIP2 City and ASN .mmdb files at startup (optional; DB-IP "City Lite" also compatible)
- Server runs without databases, returning lat/lng of 0 for all IPs
- Lookup coordinates and ASN (for `org` field) for each hop's public IP
- Private addresses (RFC 1918, loopback, link-local) return lat/lng of 0 regardless of database

### Frontend Visualization
- **Line Diagram (Main View):** Transit-map style rendering with hops as stations stacked top-to-bottom. Lanes alternate when the packet switches networks. Hop cards show IP, hostname, city, RTT; click to select and highlight on the map.
- **Geography Map (Side Panel):** Leaflet map with OpenStreetMap tiles showing only located hops. Polylines (drawn as arcs) connect hops, color-coded by network. Destination marked with an orange dot. Map pans and zooms to selected hop.
- **Network Legend:** Lists all networks crossed, with color swatches. Private IPs shown as "Local network" (grey).
- **Stats:** Distance between located hops (km or miles, shown as "?" until the player has guessed), latency to destination (RTT of the final hop if the destination replied, else "-"), count of networks crossed.
- **Example Trace:** First load shows a sample trace across ISP networks from Chicago to Amsterdam, providing clear orientation before the user enters their own endpoint.

### Hop Processing (Frontend)
- **No Reply:** Hops with no IP address (ip = "" or "*") are marked "No reply" and not plotted on the map.
- **Not Located:** Hops with an IP but no geolocation data (missing lat/lng or both zero) appear in the line diagram but not on the map.
- **Local Network:** Private IPs (10.x, 192.168.x, 172.16-31.x, 127.x, 169.254.x) labeled as "Local network" (grey color).
- **Network Name:** Derived from the `org` field if present; otherwise extracted from the last two labels of the hostname (e.g., "cox.net" from "chgil-cr1.cox.net"); defaults to "Unknown network" if no hostname.
- **Destination RTT:** Only shown if the destination itself replied (`reached`) and the last hop has an RTT value; otherwise shows "-". When it never replies, trailing no-reply hops fold into one row and the map marks the destination as unconfirmed.

### Error Handling
- Invalid endpoints (non-resolvable domains) - backend returns error
- Network unreachable scenarios - backend returns error
- API communication failures - frontend shows error banner
- Traces with no hops - frontend shows error

## Testing

- Backend: unit tests for trace parsing (macOS and Linux formats), endpoint validation, resolver, rate limiting, handler with fakes (backend/*_test.go)
- Frontend: unit tests for trace normalization, network derivation, distance calculation, destination RTT logic (src/lib/trace.test.js)
- Integration: E2E test run against real traceroute and GeoIP database (manual validation)
- No automated E2E tests yet (hard to trace real packets reliably); live validation requires real traceroute and network access

## Deployment

- Containerize both services independently
- Use docker-compose.yml for local development
- For production: push images to registry, deploy via Kubernetes or Docker Swarm
- Lightweight target: single docker-compose on a small VPS or edge device

## Notes

- **Frontend:** Light theme only (deliberately); Vite dev proxy intercepts `/api/*` and forwards to backend; example trace shown on first load; built with React 18, Leaflet 1.9, Vite 5; nginx.conf in container proxies `/api/` to backend service.
- **Backend:** Stateless by design for horizontal scaling. Rate limiting per remote address (default 20/min, burst 5); MAX_CONCURRENT_TRACES cap (default 4) returns 503 if saturated; both keyed on connection source (behind proxy, limits the proxy). Non-public targets (RFC 1918, loopback, link-local) rejected with 403 unless ALLOW_PRIVATE_TARGETS=true, preventing self-targeting. Traceroute requires elevated permissions in some environments (CAP_NET_RAW in containers for ICMP; UDP probes generally work). Geolocation accuracy ~95% city-level (GeoLite2) depending on database. Errors returned as plain text (client displays body directly).
- **Tooltip Security:** Frontend builds tooltips as DOM elements using `textContent` (never HTML) to prevent XSS from attacker-controlled reverse DNS hostnames.
- **Browser Compatibility:** Responsive layout from phone to ultra-wide widths (map fills the space beside the hop list on wide screens, stacks above it on narrow ones); light theme assumes sRGB display.
