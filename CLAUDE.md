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
1. User inputs endpoint URL/IP in React frontend
2. Frontend sends request to Go backend API
3. Backend executes traceroute/mtr command and captures hop results
4. Backend enriches hop data with geolocation (IP -> coordinates)
5. Backend returns structured hop data as JSON
6. Frontend plots hops on Leaflet map, draws path, shows hop details

### Directory Structure
```
traffic-visualizer/
├── frontend/              # React SPA
│   ├── src/
│   │   ├── components/    # React components (Map, HopList, Input)
│   │   ├── services/      # API client
│   │   ├── pages/         # Main pages
│   │   └── App.jsx
│   ├── Dockerfile
│   ├── package.json
│   └── vite.config.js
├── backend/               # Go API server
│   ├── main.go
│   ├── handlers/          # HTTP route handlers
│   ├── trace/             # Packet tracing logic (traceroute/mtr execution)
│   ├── geo/               # IP geolocation logic
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
  - Response: `{ "hops": [ { "hopNumber": 1, "ip": "1.2.3.4", "hostname": "...", "lat": 40.7, "lng": -74.0, "rtt": 1.23 }, ... ], "destination": { "ip": "...", "lat": ..., "lng": ... } }`

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

# Run tests (if present)
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

### Frontend Map Visualization
- Use Leaflet with OpenStreetMap tiles
- Plot hops as markers with hop number
- Draw polyline connecting hops to show packet path
- Color-code markers (e.g., green for first hop, red for destination)
- Show hop details (IP, hostname, RTT) in popup/sidebar

### Error Handling
- Invalid endpoints (non-resolvable domains)
- Network unreachable scenarios
- Hops with no geolocation data
- Backend/frontend communication failures

## Testing

- Backend: unit tests for trace parsing, geolocation lookups
- Integration: test full trace flow end-to-end with known destinations
- Frontend: component tests for map rendering, user input
- No E2E tests initially (hard to trace real packets reliably)

## Deployment

- Containerize both services independently
- Use docker-compose.yml for local development
- For production: push images to registry, deploy via Kubernetes or Docker Swarm
- Lightweight target: single docker-compose on a small VPS or edge device

## Notes

- Keep backend stateless for horizontal scaling
- Traceroute requires elevated permissions in some environments (CAP_NET_RAW in containers)
- Geolocation accuracy depends on database quality (GeoLite2 is ~95% city-level accurate)
- Consider rate limiting on tracing endpoint (traceroute can be resource-intensive)
