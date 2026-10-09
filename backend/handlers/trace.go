// Package handlers contains the HTTP API.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/netip"

	"github.com/tamtrinh/traffic-visualizer/backend/geo"
	"github.com/tamtrinh/traffic-visualizer/backend/trace"
)

// Tracer traces the route to an address.
type Tracer interface {
	Trace(ctx context.Context, ip netip.Addr, maxHops int) ([]trace.Hop, error)
}

// API serves the trace endpoint.
type API struct {
	Tracer   Tracer
	Resolver trace.Resolver
	Geo      geo.Locator
	// AllowPrivate permits tracing loopback and private addresses.
	AllowPrivate bool
	// slots bounds concurrent traces, which are expensive.
	slots chan struct{}
}

// NewAPI returns an API that runs at most maxConcurrent traces at once.
func NewAPI(t Tracer, r trace.Resolver, g geo.Locator, maxConcurrent int) *API {
	return &API{Tracer: t, Resolver: r, Geo: g, slots: make(chan struct{}, maxConcurrent)}
}

type traceRequest struct {
	Endpoint string `json:"endpoint"`
	MaxHops  int    `json:"maxHops"`
}

type hopJSON struct {
	HopNumber int     `json:"hopNumber"`
	IP        string  `json:"ip"`
	Hostname  string  `json:"hostname"`
	City      string  `json:"city,omitempty"`
	Country   string  `json:"country,omitempty"`
	Org       string  `json:"org,omitempty"`
	Lat       float64 `json:"lat"`
	Lng       float64 `json:"lng"`
	RTT       float64 `json:"rtt"`
}

type destinationJSON struct {
	IP   string  `json:"ip"`
	City string  `json:"city,omitempty"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

type traceResponse struct {
	Hops        []hopJSON       `json:"hops"`
	Destination destinationJSON `json:"destination"`
	// Warning explains why the trace may be incomplete. Optional.
	Warning string `json:"warning,omitempty"`
}

// noRepliesWarning is shown when only the first hop answered. It is the
// signature of Docker Desktop on macOS and Windows, but a firewall that drops
// all TTL-exceeded replies looks the same, so it says "may".
const noRepliesWarning = "No hop past the first one replied, so this trace is incomplete. " +
	"If the backend runs in Docker Desktop (macOS or Windows), its network drops the replies " +
	"traceroute needs: run the backend natively (see backend/README.md) or on a Linux host."

// Trace handles POST /api/trace.
func (a *API) Trace(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var req traceRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		http.Error(w, "request body must be JSON like {\"endpoint\":\"example.com\"}", http.StatusBadRequest)
		return
	}
	if req.MaxHops < 0 || req.MaxHops > trace.MaxHops {
		http.Error(w, "maxHops must be between 1 and 64", http.StatusBadRequest)
		return
	}

	host, err := trace.ParseEndpoint(req.Endpoint)
	if err != nil {
		http.Error(w, "enter a valid host name or IP address", http.StatusBadRequest)
		return
	}
	ip, err := trace.Resolve(r.Context(), a.Resolver, host, a.AllowPrivate)
	if err != nil {
		writeTraceError(w, err)
		return
	}

	select {
	case a.slots <- struct{}{}:
		defer func() { <-a.slots }()
	default:
		http.Error(w, "too many traces are running, try again shortly", http.StatusServiceUnavailable)
		return
	}

	hops, err := a.Tracer.Trace(r.Context(), ip, req.MaxHops)
	if err != nil {
		writeTraceError(w, err)
		return
	}
	resp := traceResponse{Hops: make([]hopJSON, 0, len(hops))}
	for _, h := range hops {
		hj := hopJSON{HopNumber: h.Number, Hostname: h.Hostname, RTT: h.RTT}
		if h.IP.IsValid() {
			hj.IP = h.IP.String()
			if loc, ok := a.locate(h.IP); ok {
				hj.City, hj.Country, hj.Org = loc.City, loc.Country, loc.Org
				hj.Lat, hj.Lng = loc.Lat, loc.Lng
			}
		}
		resp.Hops = append(resp.Hops, hj)
	}
	if trace.NoRepliesBeyondFirstHop(hops) {
		log.Printf("trace to %s: no replies beyond the first hop (Docker Desktop network, or a firewall dropping TTL-exceeded replies)", ip)
		resp.Warning = noRepliesWarning
	}
	resp.Destination.IP = ip.String()
	if loc, ok := a.locate(ip); ok {
		resp.Destination.City, resp.Destination.Lat, resp.Destination.Lng = loc.City, loc.Lat, loc.Lng
	}
	writeJSON(w, resp)
}

func (a *API) locate(ip netip.Addr) (geo.Location, bool) {
	if !geo.IsPublic(ip) {
		return geo.Location{}, false
	}
	return a.Geo.Lookup(ip)
}

func writeTraceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, trace.ErrInvalidEndpoint):
		http.Error(w, "enter a valid host name or IP address", http.StatusBadRequest)
	case errors.Is(err, trace.ErrUnresolvable):
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
	case errors.Is(err, trace.ErrForbiddenTarget):
		http.Error(w, err.Error(), http.StatusForbidden)
	case errors.Is(err, context.Canceled):
		// The client went away; nothing to send.
	case errors.Is(err, context.DeadlineExceeded):
		http.Error(w, "trace timed out", http.StatusGatewayTimeout)
	default:
		log.Printf("trace failed: %v", err)
		http.Error(w, "trace failed", http.StatusInternalServerError)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

// Health handles GET /healthz.
func Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]string{"status": "ok"})
}
