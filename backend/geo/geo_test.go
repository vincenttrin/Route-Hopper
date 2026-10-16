package geo

import (
	"net/netip"
	"testing"
)

type counting struct{ calls int }

func (c *counting) Lookup(netip.Addr) (Location, bool) { c.calls++; return Location{City: "X"}, true }
func (c *counting) Close() error                       { return nil }

func TestCached(t *testing.T) {
	c := &counting{}
	l := Cached(c)
	ip := netip.MustParseAddr("198.51.100.1")
	for range 3 {
		if loc, ok := l.Lookup(ip); !ok || loc.City != "X" {
			t.Fatalf("lookup = %+v, %v", loc, ok)
		}
	}
	if c.calls != 1 {
		t.Errorf("underlying calls = %d, want 1", c.calls)
	}
}

func TestIsPublic(t *testing.T) {
	for ip, want := range map[string]bool{
		"203.0.113.53": true, "2001:db8::1111": true,
		"10.0.0.1": false, "192.168.1.1": false, "172.16.0.1": false, "127.0.0.1": false,
		"169.254.1.1": false, "100.64.0.1": true, "::1": false, "0.0.0.0": false,
	} {
		if got := IsPublic(netip.MustParseAddr(ip)); got != want {
			t.Errorf("IsPublic(%s) = %v, want %v", ip, got, want)
		}
	}
}
