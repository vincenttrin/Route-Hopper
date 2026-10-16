package geo

import (
	"math"
	"testing"
)

func newLocator(t *testing.T) *HostnameLocator {
	t.Helper()
	h, err := NewHostnameLocator()
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestLocateHostname(t *testing.T) {
	h := newLocator(t)
	tests := []struct {
		host        string
		city, ctry  string
		hint        string
		lat, lng    float64 // approximate, to the degree
		description string
	}{
		{"ae1.pr02.ord3.example.net", "Chicago", "US", "ord3", 42, -88, "airport code and site number"},
		{"ae1.pr02.ord03.example.net", "Chicago", "US", "ord03", 42, -88, "zero padded site number"},
		{"be1.lax1.example.net", "Los Angeles", "US", "lax1", 34, -118, "lax"},
		{"ae-5.cr2.ams2.example.net", "Amsterdam", "NL", "ams2", 52, 5, "outside the US"},
		{"iad-edge-01.example.net", "Washington, D.C.", "US", "iad", 39, -77, "airport code before a dash"},
		{"be-100-ord-core.example.net", "Chicago", "US", "ord", 42, -88, "airport code between dashes"},
		{"ORD3.AE1.Example.NET.", "Chicago", "US", "ord3", 42, -88, "case and trailing dot"},
		{"ae1.pr02.ord3.example.co.uk", "Chicago", "US", "ord3", 42, -88, "two part public suffix"},
		{"lax17s55-in-f14.example.net", "Los Angeles", "US", "lax17s55", 34, -118, "Google style label"},
		{"edge-sfo53.r.example.net", "San Francisco", "US", "sfo53", 38, -122, "airport code in front of a number"},
		{"ae0.cr1.nycmny01.example.net", "New York", "US", "nycmny01", 41, -74, "CLLI with building number"},
		{"ae0.cr1.chcgil.example.net", "Chicago", "US", "chcgil", 42, -88, "bare CLLI"},
		{"xe-1-0-0.dllstx09.example.net", "Dallas", "US", "dllstx09", 33, -97, "CLLI dallas"},
		{"ae-2.lsanca03.example.net", "Los Angeles", "US", "lsanca03", 34, -118, "CLLI los angeles"},
		{"ae1.cr1.chi.example.net", "Chicago", "US", "chi", 42, -88, "metro code with no airport"},
		{"ae1.cr1.nyc2.example.net", "New York", "US", "nyc2", 41, -74, "metro code with a number"},
		{"ae-1.edge.chicago2.example.net", "Chicago", "US", "chicago2", 42, -88, "city name with a number"},
		{"ae1.cr1.frankfurt.example.net", "Frankfurt", "DE", "frankfurt", 50, 9, "city name"},
		// The label closest to the domain is the site; labels further left lose to it.
		{"lax1.ae1.ord3.example.net", "Chicago", "US", "ord3", 42, -88, "rightmost label wins"},
		{"ae1-lax-ord.example.net", "Chicago", "US", "ord", 42, -88, "rightmost dash part wins"},
		{"ae1.core.ord3.lax.example.net", "Los Angeles", "US", "lax", 34, -118, "rightmost label wins over a nearer-looking one"},
	}
	for _, tt := range tests {
		t.Run(tt.description, func(t *testing.T) {
			loc, hint, ok := h.Locate(tt.host)
			if !ok {
				t.Fatalf("Locate(%q) found nothing", tt.host)
			}
			if loc.City != tt.city || loc.Country != tt.ctry || hint != tt.hint {
				t.Errorf("Locate(%q) = %q, %q, hint %q; want %q, %q, hint %q", tt.host, loc.City, loc.Country, hint, tt.city, tt.ctry, tt.hint)
			}
			if math.Abs(loc.Lat-tt.lat) > 1.5 || math.Abs(loc.Lng-tt.lng) > 1.5 {
				t.Errorf("Locate(%q) at %v, %v; want about %v, %v", tt.host, loc.Lat, loc.Lng, tt.lat, tt.lng)
			}
		})
	}
}

func TestLocateHostnameFalsePositives(t *testing.T) {
	h := newLocator(t)
	for _, host := range []string{
		"",
		".",
		"localhost",
		"example.net",
		// The operator's own domain is never read, even when it is an airport code.
		"ord.net",
		"core.lax.net",
		"ae1.ord.co.uk",
		"ae1.pr02.example.net",
		// Router words that are not sites.
		"ae1.pr02.core.example.net",
		"xe-0-0-1.edge1.bb.example.net",
		"cr1.br2.gw3.lag4.example.net",
		"ten-0-0-0-1.example-ten.example.net",
		"ae1.ten1.example.net",
		"pop1.mgt.example.net",
		"vpn.example.net",
		"man1.example.net",
		"asr1.example.net",
		"ae1.ear1.example.net",
		// Not whole labels: an airport code buried in a word, or the wrong shape.
		"ae1.border1.example.net",
		"laxative.example.net",
		"loopback0.example.net",
		"portland.example.net",
		"ordinary.example.net",
		"lax1234.example.net",
		// An IP address spelled into the name.
		"203-0-113-7.example.net",
		"host-203-0-113-7.dyn.example.net",
		// Six letters that look like CLLI but are not a known site.
		"router.example.net",
		"sttlxx.example.net",
		// Unknown airport codes.
		"zzz1.example.net",
		"xyz.example.net",
	} {
		if loc, hint, ok := h.Locate(host); ok {
			t.Errorf("Locate(%q) = %q (from %q), want nothing", host, loc.City, hint)
		}
	}
}

func TestEverySiteResolvesToAnAirport(t *testing.T) {
	h := newLocator(t)
	for name, tables := range map[string]map[string]site{"city": citySites, "clli": clliSites} {
		for key, s := range tables {
			if loc, ok := h.site(s); !ok || !loc.HasCoords() {
				t.Errorf("%s site %q: airport %q is not in the table", name, key, s.airport)
			}
		}
	}
	for code := range notAirports {
		if len(code) != 3 {
			t.Errorf("notAirports has %q, want 3 letters", code)
		}
	}
	for _, key := range []string{"ord", "lax", "ams", "fra", "lhr", "nrt"} {
		if !h.airports[key].HasCoords() {
			t.Errorf("airport %s missing", key)
		}
	}
	if n := len(h.airports); n < 2500 {
		t.Errorf("only %d airports loaded", n)
	}
}

func TestPlace(t *testing.T) {
	h := newLocator(t)
	omaha := Location{Lat: 41.26, Lng: -95.94, City: "Omaha", Country: "US", Org: "Example Telecom"}
	amsterdam := Location{Lat: 52.37, Lng: 4.9, City: "Amsterdam", Country: "NL", Org: "Example CDN"}

	t.Run("hostname beats GeoIP and keeps the org", func(t *testing.T) {
		p, ok := Place(h, "ae1.pr02.ord3.example.net", omaha, true)
		if !ok || p.Source != SourceHostname || p.Hint != "ord3" || p.City != "Chicago" {
			t.Fatalf("placement = %+v, %v", p, ok)
		}
		if p.Org != "Example Telecom" {
			t.Errorf("org = %q, want the GeoIP org", p.Org)
		}
		// Omaha to Chicago is about 650 km: same region, not a disagreement.
		if p.DisagreementKm < 500 || p.DisagreementKm > 800 || p.Disagrees() {
			t.Errorf("disagreement = %.0f km, wild = %v", p.DisagreementKm, p.Disagrees())
		}
	})
	t.Run("a wild disagreement still goes to the hostname", func(t *testing.T) {
		p, ok := Place(h, "ae1.pr02.ord3.example.net", amsterdam, true)
		if !ok || p.Source != SourceHostname || p.City != "Chicago" {
			t.Fatalf("placement = %+v, %v", p, ok)
		}
		if p.DisagreementKm < 6000 || !p.Disagrees() {
			t.Errorf("disagreement = %.0f km, wild = %v", p.DisagreementKm, p.Disagrees())
		}
	})
	t.Run("hostname alone", func(t *testing.T) {
		p, ok := Place(h, "ae1.ord3.example.net", Location{}, false)
		if !ok || p.Source != SourceHostname || p.DisagreementKm != 0 || p.Disagrees() {
			t.Errorf("placement = %+v, %v", p, ok)
		}
	})
	t.Run("a GeoIP entry with an org but no position takes the hostname position", func(t *testing.T) {
		p, ok := Place(h, "ae1.ord3.example.net", Location{Org: "Example"}, true)
		if !ok || p.Source != SourceHostname || p.Org != "Example" || p.DisagreementKm != 0 {
			t.Errorf("placement = %+v, %v", p, ok)
		}
	})
	t.Run("no usable hostname falls back to GeoIP", func(t *testing.T) {
		for _, host := range []string{"", "ae1.core.example.net", "router.example.net"} {
			p, ok := Place(h, host, omaha, true)
			if !ok || p.Source != SourceGeoIP || p.Hint != "" || p.Location != omaha {
				t.Errorf("Place(%q) = %+v, %v", host, p, ok)
			}
		}
	})
	t.Run("a nil locator is GeoIP only", func(t *testing.T) {
		p, ok := Place(nil, "ae1.ord3.example.net", omaha, true)
		if !ok || p.Source != SourceGeoIP || p.Location != omaha {
			t.Errorf("placement = %+v, %v", p, ok)
		}
	})
	t.Run("GeoIP knowing only the org leaves the hop unplaced", func(t *testing.T) {
		p, ok := Place(h, "ae1.core.example.net", Location{Org: "Example"}, true)
		if !ok || p.Source != "" || p.HasCoords() || p.Org != "Example" {
			t.Errorf("placement = %+v, %v", p, ok)
		}
	})
	t.Run("nothing at all", func(t *testing.T) {
		if p, ok := Place(h, "ae1.core.example.net", Location{}, false); ok || p.Source != "" {
			t.Errorf("placement = %+v, %v", p, ok)
		}
	})
}
