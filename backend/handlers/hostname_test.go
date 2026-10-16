package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/vincenttrin/Route-Hopper/backend/geo"
	"github.com/vincenttrin/Route-Hopper/backend/trace"
)

// hostnameRoute is a route whose hops cover each way a hop can be placed. The
// GeoIP answers are deliberately wrong for the hops that name their site.
func hostnameRoute() []trace.Hop {
	return []trace.Hop{
		{Number: 1, IP: addr("192.168.1.1"), Hostname: "gw.lan", RTT: 1},
		{Number: 2, IP: addr("10.0.0.1"), Hostname: "agg1.example.net", RTT: 2},
		// Names its site; GeoIP says the owner's head office, in Omaha.
		{Number: 3, IP: addr("198.51.100.4"), Hostname: "ae1.pr02.ord3.example.net", RTT: 9},
		// Names its site; GeoIP is on another continent.
		{Number: 4, IP: addr("198.51.100.5"), Hostname: "ae2.cr1.ams2.example.net", RTT: 20},
		// Names nothing: GeoIP is used.
		{Number: 5, IP: addr("198.51.100.6"), Hostname: "ae3.core.example.net", RTT: 30},
		// Names its site but GeoIP has nothing for it.
		{Number: 6, IP: addr("198.51.100.7"), Hostname: "ae4.lax1.example.net", RTT: 40},
		// No site and no GeoIP: stays unlocated.
		{Number: 7, IP: addr("198.51.100.8"), Hostname: "ae5.core.example.net", RTT: 50},
		{Number: 8},
		// A private address never takes a position from its name.
		{Number: 9, IP: addr("172.16.0.9"), Hostname: "ae6.sea1.example.net", RTT: 60},
		// The destination itself.
		{Number: 10, IP: addr("203.0.113.10"), Hostname: "example.com", RTT: 70},
	}
}

func hostnameAPI(tracer Tracer, withHostnames bool) *API {
	g := fakeGeo{
		addr("198.51.100.4"): {Lat: 41.26, Lng: -95.94, City: "Omaha", Country: "US", Org: "Example Telecom"},
		addr("198.51.100.5"): {Lat: 40.71, Lng: -74.0, City: "New York", Country: "US", Org: "Example Telecom"},
		addr("198.51.100.6"): {Lat: 47.61, Lng: -122.33, City: "Seattle", Country: "US", Org: "Example Telecom"},
		addr("198.51.100.7"): {Org: "Example Telecom"},
		addr("172.16.0.9"):   {Lat: 1, Lng: 1, City: "must not be used for private IPs"},
		addr("203.0.113.10"): {Lat: 52.37, Lng: 4.9, City: "Amsterdam", Country: "NL", Org: "Example CDN"},
	}
	api := NewAPI(tracer, fakeResolver{}, g, 2)
	if withHostnames {
		h, err := geo.NewHostnameLocator()
		if err != nil {
			panic(err)
		}
		api.Hostnames = h
	}
	return api
}

// wantPlacements is what hostnameRoute must come out as with the locator on.
func checkPlacements(t *testing.T, hops []hopJSON) {
	t.Helper()
	type want struct {
		source, hint, city, org string
		located                 bool
	}
	wants := map[int]want{
		3: {"hostname", "ord3", "Chicago", "Example Telecom", true},
		4: {"hostname", "ams2", "Amsterdam", "Example Telecom", true},
		5: {"geoip", "", "Seattle", "Example Telecom", true},
		6: {"hostname", "lax1", "Los Angeles", "Example Telecom", true},
		7: {"", "", "", "", false},
		8: {"", "", "", "", false},
		9: {"", "", "", "", false},
		// The destination is a plain GeoIP result: example.com carries no site.
		10: {"geoip", "", "Amsterdam", "Example CDN", true},
	}
	if len(hops) != len(hostnameRoute()) {
		t.Fatalf("got %d hops: %+v", len(hops), hops)
	}
	for _, h := range hops {
		w, ok := wants[h.HopNumber]
		if !ok {
			continue
		}
		if h.LocationSource != w.source || h.LocationHint != w.hint || h.City != w.city || h.Org != w.org {
			t.Errorf("hop %d = %+v, want %+v", h.HopNumber, h, w)
		}
		if located := h.Lat != 0 || h.Lng != 0; located != w.located {
			t.Errorf("hop %d located = %v, want %v (%v, %v)", h.HopNumber, located, w.located, h.Lat, h.Lng)
		}
	}
	if h := hops[2]; h.Lat < 41 || h.Lat > 43 || h.Lng > -87 || h.Lng < -89 {
		t.Errorf("hop 3 is at %v, %v, want Chicago", h.Lat, h.Lng)
	}
}

func TestHostnameBeatsGeoIPInPlainResponse(t *testing.T) {
	api := hostnameAPI(&fakeTracer{hops: hostnameRoute()}, true)
	rec := post(api, `{"endpoint":"203.0.113.10"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var got traceResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	checkPlacements(t, got.Hops)
	if got.Warning != "" {
		t.Errorf("warning = %q", got.Warning)
	}
	// The source and hint are in the JSON under their documented names.
	if !strings.Contains(rec.Body.String(), `"locationSource":"hostname","locationHint":"ord3"`) {
		t.Errorf("body has no locationSource and locationHint:\n%s", rec.Body)
	}
}

func TestHostnameBeatsGeoIPInStream(t *testing.T) {
	api := hostnameAPI(&fakeTracer{hops: hostnameRoute()}, true)
	rec := postStream(api, `{"endpoint":"203.0.113.10"}`)
	var hops []hopJSON
	for _, ev := range decodeEvents(t, rec.Body.String()) {
		if ev.Type == "hop" {
			hops = append(hops, *ev.Hop)
		}
	}
	checkPlacements(t, hops)
}

func TestGeoIPOnlyWithoutHostnameLocator(t *testing.T) {
	api := hostnameAPI(&fakeTracer{hops: hostnameRoute()}, false)
	got := decodeTrace(t, api, "203.0.113.10")
	for _, h := range got.Hops {
		if h.LocationHint != "" || h.LocationSource == "hostname" {
			t.Errorf("hop %d placed from its hostname without a locator: %+v", h.HopNumber, h)
		}
	}
	if h := got.Hops[2]; h.City != "Omaha" || h.LocationSource != "geoip" {
		t.Errorf("hop 3 = %+v, want the GeoIP answer", h)
	}
	if h := got.Hops[5]; h.Lat != 0 || h.LocationSource != "" {
		t.Errorf("hop 6 = %+v, want it unlocated", h)
	}
}

func TestHostnamePlacementCountsAsLocated(t *testing.T) {
	// GeoIP knows nothing, but the names do: the trace is not "nothing could be located".
	hops := []trace.Hop{
		{Number: 1, IP: addr("198.51.100.1")}, {Number: 2, IP: addr("198.51.100.2")},
		{Number: 3, IP: addr("198.51.100.4"), Hostname: "ae1.ord3.example.net"},
		{Number: 4, IP: addr("198.51.100.5"), Hostname: "ae1.ams2.example.net"},
	}
	api := hostnameAPI(&fakeTracer{hops: hops}, true)
	api.Geo = geo.Nop{}
	api.HiddenHops = 2
	got := decodeTrace(t, api, "198.51.100.5")
	if got.Warning != "" {
		t.Errorf("warning = %q, want none", got.Warning)
	}
	if h := got.Hops[0]; h.City != "Chicago" || h.LocationSource != "hostname" {
		t.Errorf("hop 1 = %+v", h)
	}
}

func TestHiddenHopsAreNotPlacedFromTheirNames(t *testing.T) {
	// The first two hops are the server's own network. Their names name sites,
	// but neither the hops nor their sites may reach a client.
	hops := []trace.Hop{
		{Number: 1, IP: addr("198.51.100.1"), Hostname: "ae1.sea1.example.net", RTT: 1.11},
		{Number: 2, IP: addr("198.51.100.2"), Hostname: "ae1.sea2.example.net", RTT: 2.22},
		{Number: 3, IP: addr("198.51.100.4"), Hostname: "ae1.ord3.example.net", RTT: 9},
		{Number: 4, IP: addr("203.0.113.10"), Hostname: "example.com", RTT: 12},
	}
	api := hostnameAPI(&fakeTracer{hops: hops}, true)
	api.HiddenHops = 2

	plain := post(api, `{"endpoint":"203.0.113.10"}`).Body.String()
	stream := postStream(api, `{"endpoint":"203.0.113.10"}`).Body.String()
	for what, body := range map[string]string{"plain": plain, "stream": stream} {
		for _, leak := range []string{"Seattle", "sea1", "sea2", "198.51.100.1", "198.51.100.2", "1.11", "2.22"} {
			if strings.Contains(body, leak) {
				t.Errorf("%s response leaks %q:\n%s", what, leak, body)
			}
		}
		if !strings.Contains(body, "Chicago") {
			t.Errorf("%s response has no Chicago hop:\n%s", what, body)
		}
	}
}
