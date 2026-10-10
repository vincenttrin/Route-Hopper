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
	// Stream is Trace that reports each hop to sink as it is found.
	Stream(ctx context.Context, ip netip.Addr, maxHops int, sink trace.Sink) ([]trace.Hop, error)
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
	// Reached is false when the destination itself never answered a probe.
	// Many hosts and firewalls drop traceroute probes, so this is not an error.
	Reached bool `json:"reached"`
	// Warning explains why the trace may be incomplete. Optional.
	Warning string `json:"warning,omitempty"`
}

// noRepliesWarning is shown when only the first hop answered. It is the
// signature of Docker Desktop on macOS and Windows, but a firewall that drops
// all TTL-exceeded replies looks the same, so it says "may".
const noRepliesWarning = "No hop past the first one replied, so this trace is incomplete. " +
	"If the backend runs in Docker Desktop (macOS or Windows), its network drops the replies " +
	"traceroute needs: run the backend natively (see backend/README.md) or on a Linux host."

// noLocationWarning is shown when the trace crossed public addresses but none
// could be placed on the map. The usual cause is a backend started without the
// GeoIP databases (it logs "geolocation disabled" once at startup).
const noLocationWarning = "None of the hops could be placed on the map. The backend has no GeoIP database loaded, " +
	"or it has no entry for these addresses. The backend downloads the database on start when GEOIP_AUTO_UPDATE is on; " +
	"otherwise run scripts/fetch-geoip.sh and restart it (see backend/README.md)."

// startTrace validates a trace request and claims a concurrency slot for it.
// On failure it has already written the error response and ok is false.
// Otherwise the caller must call release when the trace ends.
func (a *API) startTrace(w http.ResponseWriter, r *http.Request) (ip netip.Addr, maxHops int, release func(), ok bool) {
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
	ip, err = trace.Resolve(r.Context(), a.Resolver, host, a.AllowPrivate)
	if err != nil {
		writeTraceError(w, err)
		return
	}

	select {
	case a.slots <- struct{}{}:
		return ip, req.MaxHops, func() { <-a.slots }, true
	default:
		http.Error(w, "too many traces are running, try again shortly", http.StatusServiceUnavailable)
		return
	}
}

// Trace handles POST /api/trace.
func (a *API) Trace(w http.ResponseWriter, r *http.Request) {
	ip, maxHops, release, ok := a.startTrace(w, r)
	if !ok {
		return
	}
	defer release()

	hops, err := a.Tracer.Trace(r.Context(), ip, maxHops)
	if err != nil {
		writeTraceError(w, err)
		return
	}
	resp := traceResponse{Hops: make([]hopJSON, 0, len(hops)), Reached: trace.Reached(hops, ip)}
	for _, h := range hops {
		resp.Hops = append(resp.Hops, a.enrich(h))
	}
	resp.Warning = a.warning(ip, hops)
	resp.Destination = a.destination(ip)
	writeJSON(w, resp)
}

// streamEvent is one line of the NDJSON body of POST /api/trace/stream.
// Type is "start", "hop", "phase", "reset", "done" or "error", and decides
// which of the other fields are present.
type streamEvent struct {
	Type        string           `json:"type"`
	Hop         *hopJSON         `json:"hop,omitempty"`
	Phase       string           `json:"phase,omitempty"`
	Destination *destinationJSON `json:"destination,omitempty"`
	Reached     *bool            `json:"reached,omitempty"`
	Warning     string           `json:"warning,omitempty"`
	Error       string           `json:"error,omitempty"`
	// Status is the HTTP status the same failure would have had without streaming.
	Status int `json:"status,omitempty"`
}

// TraceStream handles POST /api/trace/stream. It takes the same request as
// Trace but answers with newline-delimited JSON, one streamEvent per line, as
// the trace progresses: "start" with the destination, a "hop" for each hop as
// soon as traceroute prints it (already named and located), "phase" and
// "reset" around the ICMP retry (see trace.Sink), then "done" with whether the
// destination replied and any warning. A failure once streaming has begun is an
// "error" event; failures before that are ordinary HTTP errors, as in Trace.
func (a *API) TraceStream(w http.ResponseWriter, r *http.Request) {
	ip, maxHops, release, ok := a.startTrace(w, r)
	if !ok {
		return
	}
	defer release()

	// Stop tracing as soon as the client goes away or a write fails.
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	rc := http.NewResponseController(w)
	enc := json.NewEncoder(w)
	send := func(ev streamEvent) {
		if enc.Encode(ev) != nil || rc.Flush() != nil {
			cancel()
		}
	}

	h := w.Header()
	h.Set("Content-Type", "application/x-ndjson")
	h.Set("Cache-Control", "no-cache")
	// Tell nginx not to hold the response back to fill a buffer.
	h.Set("X-Accel-Buffering", "no")
	dest := a.destination(ip)
	send(streamEvent{Type: "start", Destination: &dest})

	hops, err := a.Tracer.Stream(ctx, ip, maxHops, trace.Sink{
		Hop: func(h trace.Hop) {
			hj := a.enrich(h)
			send(streamEvent{Type: "hop", Hop: &hj})
		},
		Phase: func(p trace.Phase) { send(streamEvent{Type: "phase", Phase: string(p)}) },
		Reset: func() { send(streamEvent{Type: "reset"}) },
	})
	if err != nil {
		if ctx.Err() == nil {
			status, msg := traceErrorStatus(err)
			send(streamEvent{Type: "error", Error: msg, Status: status})
		}
		return
	}
	reached := trace.Reached(hops, ip)
	send(streamEvent{Type: "done", Destination: &dest, Reached: &reached, Warning: a.warning(ip, hops)})
}

// enrich converts a hop to its JSON form with geolocation filled in.
func (a *API) enrich(h trace.Hop) hopJSON {
	hj := hopJSON{HopNumber: h.Number, Hostname: h.Hostname, RTT: h.RTT}
	if h.IP.IsValid() {
		hj.IP = h.IP.String()
		if loc, ok := a.locate(h.IP); ok {
			hj.City, hj.Country, hj.Org = loc.City, loc.Country, loc.Org
			hj.Lat, hj.Lng = loc.Lat, loc.Lng
		}
	}
	return hj
}

func (a *API) destination(ip netip.Addr) destinationJSON {
	d := destinationJSON{IP: ip.String()}
	if loc, ok := a.locate(ip); ok {
		d.City, d.Lat, d.Lng = loc.City, loc.Lat, loc.Lng
	}
	return d
}

// warning explains why a finished trace may be incomplete, or returns "".
func (a *API) warning(ip netip.Addr, hops []trace.Hop) string {
	if trace.NoRepliesBeyondFirstHop(hops) {
		log.Printf("trace to %s: no replies beyond the first hop (Docker Desktop network, or a firewall dropping TTL-exceeded replies)", ip)
		return noRepliesWarning
	}
	var anyPublic, anyLocated bool
	for _, h := range hops {
		if !h.IP.IsValid() {
			continue
		}
		anyPublic = anyPublic || geo.IsPublic(h.IP)
		if _, ok := a.locate(h.IP); ok {
			anyLocated = true
		}
	}
	if anyPublic && !anyLocated {
		return noLocationWarning
	}
	return ""
}

func (a *API) locate(ip netip.Addr) (geo.Location, bool) {
	if !geo.IsPublic(ip) {
		return geo.Location{}, false
	}
	return a.Geo.Lookup(ip)
}

// traceErrorStatus maps a trace error to the HTTP status and message it is
// reported with. Context cancellation is the client leaving and has no status.
func traceErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, trace.ErrInvalidEndpoint):
		return http.StatusBadRequest, "enter a valid host name or IP address"
	case errors.Is(err, trace.ErrUnresolvable):
		return http.StatusUnprocessableEntity, err.Error()
	case errors.Is(err, trace.ErrForbiddenTarget):
		return http.StatusForbidden, err.Error()
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout, "trace timed out"
	default:
		log.Printf("trace failed: %v", err)
		return http.StatusInternalServerError, "trace failed"
	}
}

func writeTraceError(w http.ResponseWriter, err error) {
	if errors.Is(err, context.Canceled) {
		// The client went away; nothing to send.
		return
	}
	status, msg := traceErrorStatus(err)
	http.Error(w, msg, status)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

// geoInfo describes the location database in use.
func (a *API) geoInfo() geo.Info {
	if d, ok := a.Geo.(geo.Describer); ok {
		return d.Info()
	}
	return geo.Info{}
}

// Health handles GET /healthz. The server is healthy without a location
// database, which may still be downloading or unreachable: "geoip" says which.
func (a *API) Health(w http.ResponseWriter, _ *http.Request) {
	state := "missing"
	if a.geoInfo().Available {
		state = "ready"
	}
	writeJSON(w, map[string]string{"status": "ok", "geoip": state})
}

// GeoInfo handles GET /api/geoip: which location database is loaded and when it was built.
func (a *API) GeoInfo(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	writeJSON(w, a.geoInfo())
}
