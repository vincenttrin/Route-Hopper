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
├── backend/               # Go API server (not in this change)
│   ├── main.go
│   ├── handlers/          # HTTP route handlers
│   ├── trace/             # Packet tracing logic (traceroute/mtr execution)
│   ├── geo/               # IP geolocation logic
│   ├── Dockerfile
│   ├── go.mod
│   └── go.sum
├── docker-compose.yml     # Local dev orchestration (not in this change)
├── CLAUDE.md              # This file
└── README.md              # User-facing documentation
```

### Backend API Endpoints
- `POST /api/trace` - Start packet tracing
  - Request: `{ "endpoint": "example.com", "maxHops": 30 }`
  - Response: `{ "hops": [ { "hopNumber": 1, "ip": "1.2.3.4", "hostname": "...", "city": "...", "lat": 40.7, "lng": -74.0, "rtt": 1.23, "org": "..." }, ... ], "destination": { "ip": "...", "lat": ..., "lng": ... } }`
  - `city` and `org` are optional; `org` can be used to override network name derivation

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
- Execute `traceroute -m <maxHops>` or `mtr --report` as subprocess
- Parse output to extract hop information (IP, hostname, RTT)
- Handle different traceroute output formats (Linux vs macOS)
- Gracefully handle cases where hops are unreachable or blocked

### Geolocation (Backend)
- Use MaxMind GeoIP2 Lite free database (or IP2Location)
- Load geolocation database on server startup
- Cache IP -> geo lookups in memory (optional)
- Return lat/lng for each hop's IP address

### Frontend Visualization
- **Line Diagram (Main View):** Transit-map style rendering with hops as stations stacked top-to-bottom. Lanes alternate when the packet switches networks. Hop cards show IP, hostname, city, RTT; click to select and highlight on the map.
- **Geography Map (Side Panel):** Leaflet map with OpenStreetMap tiles showing only located hops. Polylines connect hops, color-coded by network. Destination marked with a yellow dot. Map pans and zooms to selected hop.
- **Network Legend:** Lists all networks crossed, with color swatches. Private IPs shown as "Local network" (grey).
- **Stats:** Distance between located hops (km), latency to destination (RTT of final hop if present, else "-"), count of networks crossed.
- **Example Trace:** First load shows a sample trace across ISP networks from Omaha to Amsterdam, providing clear orientation before the user enters their own endpoint.

### Hop Processing (Frontend)
- **No Reply:** Hops with no IP address (ip = "" or "*") are marked "No reply" and not plotted on the map.
- **Not Located:** Hops with an IP but no geolocation data (missing lat/lng or both zero) appear in the line diagram but not on the map.
- **Local Network:** Private IPs (10.x, 192.168.x, 172.16-31.x, 127.x, 169.254.x) labeled as "Local network" (grey color).
- **Network Name:** Derived from the `org` field if present; otherwise extracted from the last two labels of the hostname (e.g., "cox.net" from "chgil-cr1.cox.net"); defaults to "Unknown network" if no hostname.
- **Destination RTT:** Only shown if the last hop has an RTT value; otherwise shows "-".

### Error Handling
- Invalid endpoints (non-resolvable domains) - backend returns error
- Network unreachable scenarios - backend returns error
- API communication failures - frontend shows error banner
- Traces with no hops - frontend shows error

## Testing

- Backend: unit tests for trace parsing, geolocation lookups (not in this change)
- Frontend: unit tests for trace normalization, network derivation, distance calculation, destination RTT logic (src/lib/trace.test.js)
- Integration: test full trace flow end-to-end with known destinations
- No E2E tests initially (hard to trace real packets reliably); live validation deferred (backend not yet available)

## Deployment

- Containerize both services independently
- Use docker-compose.yml for local development
- For production: push images to registry, deploy via Kubernetes or Docker Swarm
- Lightweight target: single docker-compose on a small VPS or edge device

## Notes

- **Frontend:** Light theme only (deliberately); Vite dev proxy intercepts `/api/*` and forwards to backend; example trace shown on first load; built with React 18, Leaflet 1.9, Vite 5; nginx.conf in container proxies `/api/` to backend service.
- **Backend:** Keep stateless for horizontal scaling; traceroute requires elevated permissions in some environments (CAP_NET_RAW in containers); geolocation accuracy depends on database quality (GeoLite2 is ~95% city-level accurate); consider rate limiting on tracing endpoint (traceroute can be resource-intensive).
- **Tooltip Security:** Frontend builds tooltips as DOM elements using `textContent` (never HTML) to prevent XSS from attacker-controlled reverse DNS hostnames.
- **Browser Compatibility:** Desktop browsers only (no mobile optimization); light theme assumes sRGB display.
