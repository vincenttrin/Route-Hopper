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

	"github.com/tamtrinh/traffic-visualizer/backend/geo"
	"github.com/tamtrinh/traffic-visualizer/backend/trace"
)

type fakeTracer struct {
	hops []trace.Hop
	err  error
	got  netip.Addr
	max  int
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
