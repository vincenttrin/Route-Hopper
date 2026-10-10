package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/tamtrinh/traffic-visualizer/backend/geo"
	"github.com/tamtrinh/traffic-visualizer/backend/trace"
)

type fakeTracer struct {
	hops []trace.Hop
	err  error
	got  netip.Addr
	max  int
	// For Stream: events to report before the hops, and a hook run after each
	// hop is reported.
	pre   func(trace.Sink)
	after func(i int)
	// block makes Stream wait for the context to end after reporting hops.
	block   bool
	stopped chan struct{}
}

func (f *fakeTracer) Stream(ctx context.Context, ip netip.Addr, maxHops int, sink trace.Sink) ([]trace.Hop, error) {
	f.got, f.max = ip, maxHops
	if f.stopped != nil {
		defer close(f.stopped)
	}
	if f.pre != nil {
		f.pre(sink)
	}
	for i, h := range f.hops {
		sink.Hop(h)
		if f.after != nil {
			f.after(i)
		}
	}
	if f.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.hops, f.err
}

func (f *fakeTracer) Trace(_ context.Context, ip netip.Addr, maxHops int) ([]trace.Hop, error) {
	f.got, f.max = ip, maxHops
	return f.hops, f.err
}

type fakeResolver map[string]netip.Addr

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := f[host]; ok {
		return []netip.Addr{a}, nil
	}
	return nil, errors.New("no such host")
}

type fakeGeo map[netip.Addr]geo.Location

func (f fakeGeo) Lookup(ip netip.Addr) (geo.Location, bool) { l, ok := f[ip]; return l, ok }
func (fakeGeo) Close() error                                { return nil }

func post(api *API, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/trace", strings.NewReader(body))
	rec := httptest.NewRecorder()
	api.Trace(rec, req)
	return rec
}

func addr(s string) netip.Addr { return netip.MustParseAddr(s) }

func TestTraceResponseShape(t *testing.T) {
	tracer := &fakeTracer{hops: []trace.Hop{
		{Number: 1, IP: addr("192.168.1.1"), Hostname: "gateway.lan", RTT: 1.5},
		{Number: 2},
		{Number: 3, IP: addr("93.184.216.34"), Hostname: "example.com", RTT: 12},
	}}
	g := fakeGeo{
		addr("93.184.216.34"): {Lat: 52.37, Lng: 4.9, City: "Amsterdam", Country: "NL", Org: "Edgecast"},
		addr("192.168.1.1"):   {Lat: 1, Lng: 1, City: "must not be used for private IPs"},
	}
	api := NewAPI(tracer, fakeResolver{"example.com": addr("93.184.216.34")}, g, 2)

	rec := post(api, `{"endpoint":"https://example.com/path","maxHops":12}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if tracer.got != addr("93.184.216.34") || tracer.max != 12 {
		t.Errorf("tracer called with %v, %d", tracer.got, tracer.max)
	}
	var got traceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := []hopJSON{
		{HopNumber: 1, IP: "192.168.1.1", Hostname: "gateway.lan", RTT: 1.5},
		{HopNumber: 2},
		{HopNumber: 3, IP: "93.184.216.34", Hostname: "example.com", City: "Amsterdam", Country: "NL", Org: "Edgecast", Lat: 52.37, Lng: 4.9, RTT: 12},
	}
	if len(got.Hops) != len(want) {
		t.Fatalf("hops = %+v", got.Hops)
	}
	for i := range want {
		if got.Hops[i] != want[i] {
			t.Errorf("hop %d = %+v, want %+v", i+1, got.Hops[i], want[i])
		}
	}
	if d := got.Destination; d.IP != "93.184.216.34" || d.Lat != 52.37 || d.Lng != 4.9 || d.City != "Amsterdam" {
		t.Errorf("destination = %+v", d)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content type %q", ct)
	}
}

func TestTraceWarnsWhenNoRepliesBeyondFirstHop(t *testing.T) {
	dockerLike := []trace.Hop{{Number: 1, IP: addr("172.21.0.1"), RTT: 0.1}, {Number: 2}, {Number: 3}, {Number: 4}}
	healthy := []trace.Hop{{Number: 1, IP: addr("192.168.1.1")}, {Number: 2, IP: addr("93.184.216.34")}, {Number: 3, IP: addr("93.184.216.35")}}
	tests := []struct {
		name     string
		hops     []trace.Hop
		wantWarn bool
	}{
		{"docker desktop style", dockerLike, true},
		{"healthy trace", healthy, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := fakeGeo{addr("93.184.216.34"): {Lat: 52.37, Lng: 4.9, City: "Amsterdam"}}
			api := NewAPI(&fakeTracer{hops: tt.hops}, fakeResolver{}, g, 1)
			rec := post(api, `{"endpoint":"8.8.8.8"}`)
			if rec.Code != http.StatusOK {
				t.Fatalf("status %d: %s", rec.Code, rec.Body)
			}
			var got traceResponse
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if tt.wantWarn {
				if !strings.Contains(got.Warning, "Docker Desktop") || !strings.Contains(got.Warning, "natively") {
					t.Errorf("warning = %q", got.Warning)
				}
				if len(got.Hops) != len(tt.hops) {
					t.Errorf("hops still returned: %d", len(got.Hops))
				}
			} else if got.Warning != "" || strings.Contains(rec.Body.String(), "warning") {
				t.Errorf("unexpected warning in %s", rec.Body)
			}
		})
	}
}

func decodeTrace(t *testing.T, api *API, endpoint string) traceResponse {
	t.Helper()
	rec := post(api, `{"endpoint":"`+endpoint+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var got traceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestTraceReportsWhetherDestinationReplied(t *testing.T) {
	path := []trace.Hop{{Number: 1, IP: addr("192.168.1.1")}, {Number: 2, IP: addr("96.34.20.4")}, {Number: 3, IP: addr("157.240.69.22")}}
	tests := []struct {
		name string
		hops []trace.Hop
		want bool
	}{
		{"destination answered", append(path[:3:3], trace.Hop{Number: 4, IP: addr("57.144.20.1")}), true},
		// facebook.com: the path answers, then silence up to the hop limit.
		{"destination silent", append(path[:3:3], trace.Hop{Number: 4}, trace.Hop{Number: 5}), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := fakeGeo{addr("96.34.20.4"): {Lat: 42.5, Lng: -89, City: "Beloit"}}
			api := NewAPI(&fakeTracer{hops: tt.hops}, fakeResolver{}, g, 1)
			got := decodeTrace(t, api, "57.144.20.1")
			if got.Reached != tt.want {
				t.Errorf("reached = %v, want %v", got.Reached, tt.want)
			}
			if got.Warning != "" {
				t.Errorf("a silent destination is not a warning: %q", got.Warning)
			}
		})
	}
}

func TestTraceWarnsWhenNothingCanBeLocated(t *testing.T) {
	hops := []trace.Hop{{Number: 1, IP: addr("192.168.1.1")}, {Number: 2, IP: addr("96.34.20.4")}, {Number: 3, IP: addr("93.184.216.34")}}
	located := fakeGeo{addr("96.34.20.4"): {Lat: 42.5, Lng: -89, City: "Beloit"}}
	tests := []struct {
		name     string
		geo      geo.Locator
		hops     []trace.Hop
		wantWarn bool
	}{
		// scripts/dev.sh started the backend without finding the downloaded databases.
		{"no database", geo.Nop{}, hops, true},
		{"database knows a hop", located, hops, false},
		{"only private hops, nothing to locate", geo.Nop{}, hops[:1], false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewAPI(&fakeTracer{hops: tt.hops}, fakeResolver{}, tt.geo, 1)
			got := decodeTrace(t, api, "93.184.216.34")
			if tt.wantWarn {
				if !strings.Contains(got.Warning, "GeoIP") || !strings.Contains(got.Warning, "fetch-geoip.sh") {
					t.Errorf("warning = %q", got.Warning)
				}
			} else if got.Warning != "" {
				t.Errorf("unexpected warning %q", got.Warning)
			}
		})
	}
}

func TestTraceEmptyHopsEncodesAsArray(t *testing.T) {
	api := NewAPI(&fakeTracer{}, fakeResolver{}, geo.Nop{}, 1)
	rec := post(api, `{"endpoint":"8.8.8.8"}`)
	if !strings.Contains(rec.Body.String(), `"hops":[]`) {
		t.Errorf("body = %s", rec.Body)
	}
}

func TestTraceErrors(t *testing.T) {
	tests := []struct {
		name   string
		body   string
		tracer error
		want   int
	}{
		{"bad json", `nope`, nil, http.StatusBadRequest},
		{"empty endpoint", `{"endpoint":""}`, nil, http.StatusBadRequest},
		{"flag injection", `{"endpoint":"-m 1"}`, nil, http.StatusBadRequest},
		{"negative hops", `{"endpoint":"8.8.8.8","maxHops":-1}`, nil, http.StatusBadRequest},
		{"too many hops", `{"endpoint":"8.8.8.8","maxHops":65}`, nil, http.StatusBadRequest},
		{"unresolvable", `{"endpoint":"nope.invalid"}`, nil, http.StatusUnprocessableEntity},
		{"private target", `{"endpoint":"10.0.0.1"}`, nil, http.StatusForbidden},
		{"timeout", `{"endpoint":"8.8.8.8"}`, context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"tool failure", `{"endpoint":"8.8.8.8"}`, errors.New("boom"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			api := NewAPI(&fakeTracer{err: tt.tracer}, fakeResolver{}, geo.Nop{}, 1)
			if rec := post(api, tt.body); rec.Code != tt.want {
				t.Errorf("status %d, want %d (%s)", rec.Code, tt.want, rec.Body)
			}
		})
	}
}

func TestTraceAllowPrivate(t *testing.T) {
	api := NewAPI(&fakeTracer{}, fakeResolver{}, geo.Nop{}, 1)
	api.AllowPrivate = true
	if rec := post(api, `{"endpoint":"192.168.1.1"}`); rec.Code != http.StatusOK {
		t.Errorf("status %d", rec.Code)
	}
}

func TestTraceSaturated(t *testing.T) {
	api := NewAPI(&fakeTracer{}, fakeResolver{}, geo.Nop{}, 1)
	api.slots <- struct{}{}
	if rec := post(api, `{"endpoint":"8.8.8.8"}`); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status %d", rec.Code)
	}
}

func TestRateLimit(t *testing.T) {
	h := RateLimit(1, 2, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	codes := []int{}
	for range 3 {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = "1.2.3.4:5555"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		codes = append(codes, rec.Code)
	}
	if codes[0] != 200 || codes[1] != 200 || codes[2] != http.StatusTooManyRequests {
		t.Errorf("codes = %v", codes)
	}
	other := httptest.NewRequest(http.MethodPost, "/", nil)
	other.RemoteAddr = "5.6.7.8:1"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, other)
	if rec.Code != 200 {
		t.Errorf("other client status %d", rec.Code)
	}
}

func postStream(api *API, body string) *httptest.ResponseRecorder {
	return postStreamCtx(context.Background(), api, body)
}

func postStreamCtx(ctx context.Context, api *API, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/trace/stream", strings.NewReader(body)).WithContext(ctx)
	rec := httptest.NewRecorder()
	api.TraceStream(rec, req)
	return rec
}

func decodeEvents(t *testing.T, body string) []streamEvent {
	t.Helper()
	var events []streamEvent
	for _, line := range strings.Split(strings.TrimSpace(body), "\n") {
		var ev streamEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("bad event line %q: %v", line, err)
		}
		events = append(events, ev)
	}
	return events
}

func eventTypes(events []streamEvent) string {
	var types []string
	for _, ev := range events {
		types = append(types, ev.Type)
	}
	return strings.Join(types, " ")
}

func TestTraceStreamEmitsStartHopsAndDone(t *testing.T) {
	tracer := &fakeTracer{hops: []trace.Hop{
		{Number: 1, IP: addr("192.168.1.1"), Hostname: "gateway.lan", RTT: 1.5},
		{Number: 2},
		{Number: 3, IP: addr("93.184.216.34"), Hostname: "example.com", RTT: 12},
	}}
	g := fakeGeo{addr("93.184.216.34"): {Lat: 52.37, Lng: 4.9, City: "Amsterdam", Country: "NL", Org: "Edgecast"}}
	api := NewAPI(tracer, fakeResolver{"example.com": addr("93.184.216.34")}, g, 2)

	rec := postStream(api, `{"endpoint":"example.com","maxHops":12}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-ndjson" {
		t.Errorf("content type %q", ct)
	}
	if rec.Header().Get("X-Accel-Buffering") != "no" {
		t.Error("response does not ask nginx not to buffer")
	}
	if !rec.Flushed {
		t.Error("response was never flushed")
	}
	if tracer.got != addr("93.184.216.34") || tracer.max != 12 {
		t.Errorf("tracer called with %v, %d", tracer.got, tracer.max)
	}
	events := decodeEvents(t, rec.Body.String())
	if got := eventTypes(events); got != "start hop hop hop done" {
		t.Fatalf("events = %s", got)
	}
	if d := events[0].Destination; d == nil || d.IP != "93.184.216.34" || d.City != "Amsterdam" || d.Lat != 52.37 {
		t.Errorf("start destination = %+v", d)
	}
	want := hopJSON{HopNumber: 3, IP: "93.184.216.34", Hostname: "example.com", City: "Amsterdam", Country: "NL", Org: "Edgecast", Lat: 52.37, Lng: 4.9, RTT: 12}
	if *events[3].Hop != want {
		t.Errorf("hop 3 = %+v, want %+v", *events[3].Hop, want)
	}
	if h := events[1].Hop; h.City != "" || h.Lat != 0 {
		t.Errorf("private hop must not be located: %+v", h)
	}
	done := events[4]
	if done.Reached == nil || !*done.Reached || done.Warning != "" || done.Destination.IP != "93.184.216.34" {
		t.Errorf("done = %+v", done)
	}
}

func TestTraceStreamFlushesEachHopBeforeTheTraceEnds(t *testing.T) {
	var linesWhileRunning []int
	var api *API
	tracer := &fakeTracer{hops: []trace.Hop{{Number: 1, IP: addr("192.168.1.1")}, {Number: 2, IP: addr("96.34.20.4")}}}
	rec := httptest.NewRecorder()
	tracer.after = func(i int) {
		linesWhileRunning = append(linesWhileRunning, strings.Count(rec.Body.String(), "\n"))
	}
	api = NewAPI(tracer, fakeResolver{}, geo.Nop{}, 1)
	req := httptest.NewRequest(http.MethodPost, "/api/trace/stream", strings.NewReader(`{"endpoint":"8.8.8.8"}`))
	api.TraceStream(rec, req)
	// start + hop 1 are already written when hop 1 is reported; then hop 2 too.
	if len(linesWhileRunning) != 2 || linesWhileRunning[0] != 2 || linesWhileRunning[1] != 3 {
		t.Errorf("lines visible after each hop = %v", linesWhileRunning)
	}
}

func TestTraceStreamForwardsICMPRetryEvents(t *testing.T) {
	tracer := &fakeTracer{
		hops: []trace.Hop{{Number: 1, IP: addr("192.168.1.1")}},
		pre: func(s trace.Sink) {
			s.Hop(trace.Hop{Number: 1, IP: addr("192.168.1.1")})
			s.Hop(trace.Hop{Number: 2})
			s.Phase(trace.PhaseICMP)
			s.Reset()
		},
	}
	api := NewAPI(tracer, fakeResolver{}, geo.Nop{}, 1)
	events := decodeEvents(t, postStream(api, `{"endpoint":"57.144.20.1"}`).Body.String())
	if got := eventTypes(events); got != "start hop hop phase reset hop done" {
		t.Fatalf("events = %s", got)
	}
	if events[3].Phase != "icmp" {
		t.Errorf("phase = %q", events[3].Phase)
	}
	if events[6].Reached == nil || *events[6].Reached {
		t.Errorf("destination never replied: %+v", events[6])
	}
}

func TestTraceStreamDoneCarriesWarnings(t *testing.T) {
	dockerLike := []trace.Hop{{Number: 1, IP: addr("172.21.0.1")}, {Number: 2}, {Number: 3}, {Number: 4}}
	api := NewAPI(&fakeTracer{hops: dockerLike}, fakeResolver{}, geo.Nop{}, 1)
	events := decodeEvents(t, postStream(api, `{"endpoint":"8.8.8.8"}`).Body.String())
	done := events[len(events)-1]
	if done.Type != "done" || !strings.Contains(done.Warning, "Docker Desktop") {
		t.Errorf("done = %+v", done)
	}

	public := []trace.Hop{{Number: 1, IP: addr("192.168.1.1")}, {Number: 2, IP: addr("96.34.20.4")}, {Number: 3, IP: addr("93.184.216.34")}}
	api = NewAPI(&fakeTracer{hops: public}, fakeResolver{}, geo.Nop{}, 1)
	events = decodeEvents(t, postStream(api, `{"endpoint":"93.184.216.34"}`).Body.String())
	if w := events[len(events)-1].Warning; !strings.Contains(w, "fetch-geoip.sh") {
		t.Errorf("warning = %q", w)
	}
}

func TestTraceStreamErrorBeforeStreamingIsHTTPError(t *testing.T) {
	tests := []struct {
		name string
		body string
		want int
	}{
		{"bad json", `nope`, http.StatusBadRequest},
		{"flag injection", `{"endpoint":"-m 1"}`, http.StatusBadRequest},
		{"too many hops", `{"endpoint":"8.8.8.8","maxHops":65}`, http.StatusBadRequest},
		{"unresolvable", `{"endpoint":"nope.invalid"}`, http.StatusUnprocessableEntity},
		{"private target", `{"endpoint":"10.0.0.1"}`, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracer := &fakeTracer{}
			api := NewAPI(tracer, fakeResolver{}, geo.Nop{}, 1)
			rec := postStream(api, tt.body)
			if rec.Code != tt.want {
				t.Errorf("status %d, want %d (%s)", rec.Code, tt.want, rec.Body)
			}
			if strings.HasPrefix(rec.Header().Get("Content-Type"), "application/x-ndjson") {
				t.Error("error response claims to be a stream")
			}
			if tracer.got.IsValid() {
				t.Error("tracer ran for a rejected request")
			}
		})
	}
}

func TestTraceStreamSaturated(t *testing.T) {
	api := NewAPI(&fakeTracer{}, fakeResolver{}, geo.Nop{}, 1)
	api.slots <- struct{}{}
	if rec := postStream(api, `{"endpoint":"8.8.8.8"}`); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status %d", rec.Code)
	}
}

func TestTraceStreamReleasesSlotWhenDone(t *testing.T) {
	api := NewAPI(&fakeTracer{}, fakeResolver{}, geo.Nop{}, 1)
	for range 2 {
		if rec := postStream(api, `{"endpoint":"8.8.8.8"}`); rec.Code != http.StatusOK {
			t.Fatalf("status %d", rec.Code)
		}
	}
}

func TestTraceStreamErrorAfterHopsIsAnEvent(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantText   string
	}{
		{"timeout", context.DeadlineExceeded, http.StatusGatewayTimeout, "trace timed out"},
		{"tool failure", errors.New("boom"), http.StatusInternalServerError, "trace failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tracer := &fakeTracer{err: tt.err, pre: func(s trace.Sink) { s.Hop(trace.Hop{Number: 1, IP: addr("192.168.1.1")}) }}
			api := NewAPI(tracer, fakeResolver{}, geo.Nop{}, 1)
			rec := postStream(api, `{"endpoint":"8.8.8.8"}`)
			events := decodeEvents(t, rec.Body.String())
			if got := eventTypes(events); got != "start hop error" {
				t.Fatalf("events = %s", got)
			}
			if e := events[2]; e.Status != tt.wantStatus || e.Error != tt.wantText {
				t.Errorf("error event = %+v", e)
			}
		})
	}
}

func TestTraceStreamStopsTracerWhenClientDisconnects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	tracer := &fakeTracer{block: true, stopped: stopped}
	tracer.pre = func(trace.Sink) { cancel() } // the client leaves right after the stream starts
	api := NewAPI(tracer, fakeResolver{}, geo.Nop{}, 1)

	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() { finished <- postStreamCtx(ctx, api, `{"endpoint":"8.8.8.8"}`) }()
	select {
	case rec := <-finished:
		if strings.Contains(rec.Body.String(), `"type":"error"`) {
			t.Errorf("a disconnect is not an error to report: %s", rec.Body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler kept tracing after the client disconnected")
	}
	<-stopped
	if len(api.slots) != 0 {
		t.Error("concurrency slot was not released")
	}
}

type failingWriter struct{ *httptest.ResponseRecorder }

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("broken pipe") }

func TestTraceStreamStopsWhenWritesFail(t *testing.T) {
	stopped := make(chan struct{})
	tracer := &fakeTracer{block: true, stopped: stopped}
	api := NewAPI(tracer, fakeResolver{}, geo.Nop{}, 1)
	req := httptest.NewRequest(http.MethodPost, "/api/trace/stream", strings.NewReader(`{"endpoint":"8.8.8.8"}`))
	done := make(chan struct{})
	go func() {
		api.TraceStream(failingWriter{httptest.NewRecorder()}, req)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler kept tracing after a failed write")
	}
}

type describedGeo struct {
	fakeGeo
	info geo.Info
}

func (d describedGeo) Info() geo.Info { return d.info }

func TestHealthAndGeoInfo(t *testing.T) {
	get := func(h http.HandlerFunc) (int, string) {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		return rec.Code, strings.TrimSpace(rec.Body.String())
	}
	t.Run("without a database", func(t *testing.T) {
		// Still healthy: the server runs, only without locations.
		api := NewAPI(&fakeTracer{}, fakeResolver{}, geo.NewSwappable(geo.Nop{}), 1)
		if code, body := get(api.Health); code != 200 || body != `{"geoip":"missing","status":"ok"}` {
			t.Errorf("health = %d %s", code, body)
		}
		if code, body := get(api.GeoInfo); code != 200 || body != `{"available":false}` {
			t.Errorf("geoip = %d %s", code, body)
		}
	})
	t.Run("with a database", func(t *testing.T) {
		g := describedGeo{info: geo.Info{Available: true, Provider: "DB-IP Lite", Updated: "2026-10-01"}}
		api := NewAPI(&fakeTracer{}, fakeResolver{}, g, 1)
		if code, body := get(api.Health); code != 200 || body != `{"geoip":"ready","status":"ok"}` {
			t.Errorf("health = %d %s", code, body)
		}
		want := `{"available":true,"provider":"DB-IP Lite","updated":"2026-10-01"}`
		if code, body := get(api.GeoInfo); code != 200 || body != want {
			t.Errorf("geoip = %d %s", code, body)
		}
	})
}
