// Package geo resolves IP addresses to coordinates and network owners.
package geo

import (
	"math"
	"net/netip"
	"sync"
)

// Location is the geolocation result for one IP address.
type Location struct {
	Lat, Lng float64
	City     string
	Country  string
	Org      string
}

// Locator looks up the location of a public IP address.
type Locator interface {
	// Lookup returns false when nothing is known about the address.
	Lookup(ip netip.Addr) (Location, bool)
	Close() error
}

// IsPublic reports whether ip is a globally routable unicast address.
func IsPublic(ip netip.Addr) bool {
	return ip.IsValid() && ip.IsGlobalUnicast() && !ip.IsPrivate()
}

// Nop is a Locator that knows nothing. It is used when no database is configured.
type Nop struct{}

func (Nop) Lookup(netip.Addr) (Location, bool) { return Location{}, false }
func (Nop) Close() error                       { return nil }

const maxCacheEntries = 10000

type cacheEntry struct {
	loc Location
	ok  bool
}

type cached struct {
	Locator
	mu sync.RWMutex
	m  map[netip.Addr]cacheEntry
}

// Cached wraps l with an in-memory lookup cache. The cache is dropped when it
// grows past a fixed size, which keeps memory bounded without tracking recency.
func Cached(l Locator) Locator {
	return &cached{Locator: l, m: make(map[netip.Addr]cacheEntry)}
}

func (c *cached) Lookup(ip netip.Addr) (Location, bool) {
	c.mu.RLock()
	e, hit := c.m[ip]
	c.mu.RUnlock()
	if hit {
		return e.loc, e.ok
	}
	loc, ok := c.Locator.Lookup(ip)
	c.mu.Lock()
	if len(c.m) >= maxCacheEntries {
		clear(c.m)
	}
	c.m[ip] = cacheEntry{loc, ok}
	c.mu.Unlock()
	return loc, ok
}

// DistanceKm is the great-circle distance between two locations in kilometres.
func DistanceKm(a, b Location) float64 {
	const earthKm = 6371
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	s := math.Sin(rad(b.Lat-a.Lat)/2)*math.Sin(rad(b.Lat-a.Lat)/2) +
		math.Cos(rad(a.Lat))*math.Cos(rad(b.Lat))*math.Sin(rad(b.Lng-a.Lng)/2)*math.Sin(rad(b.Lng-a.Lng)/2)
	return earthKm * 2 * math.Asin(math.Sqrt(s))
}
