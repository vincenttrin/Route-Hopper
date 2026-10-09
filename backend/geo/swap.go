package geo

import (
	"net/netip"
	"sync"
	"time"
)

// Info describes the loaded databases, for the UI's "database last updated" note.
type Info struct {
	// Available is false while no City database is loaded.
	Available bool `json:"available"`
	// Provider names the database publisher, e.g. "DB-IP Lite".
	Provider string `json:"provider,omitempty"`
	// Updated is the City database's build date (UTC, YYYY-MM-DD).
	Updated string `json:"updated,omitempty"`
}

// Describer is implemented by Locators that can report which database they hold.
type Describer interface {
	Info() Info
}

// provider maps a database_type from the mmdb metadata to a publisher name.
func provider(databaseType string) string {
	switch {
	case len(databaseType) >= 4 && databaseType[:4] == "DBIP":
		return "DB-IP Lite"
	case len(databaseType) >= 8 && databaseType[:8] == "GeoLite2":
		return "MaxMind GeoLite2"
	default:
		return databaseType
	}
}

func infoFor(l Locator) Info {
	if d, ok := l.(Describer); ok {
		return d.Info()
	}
	return Info{}
}

// Swappable is a Locator whose databases can be replaced while lookups are in
// flight. Lookups are cached; the cache goes with the databases it came from.
type Swappable struct {
	mu  sync.RWMutex
	cur Locator
}

// NewSwappable starts with initial, which may be Nop.
func NewSwappable(initial Locator) *Swappable {
	return &Swappable{cur: Cached(initial)}
}

func (s *Swappable) Lookup(ip netip.Addr) (Location, bool) {
	// The read lock is held for the whole lookup so that Swap can only close
	// the old databases once no lookup is using them.
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur.Lookup(ip)
}

// Swap makes next the active Locator and closes the previous one.
func (s *Swappable) Swap(next Locator) {
	s.mu.Lock()
	old := s.cur
	s.cur = Cached(next)
	s.mu.Unlock()
	old.Close()
}

// Info reports the active databases.
func (s *Swappable) Info() Info {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return infoFor(s.cur)
}

func (s *Swappable) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur.Close()
}

func (c *cached) Info() Info { return infoFor(c.Locator) }

// BuildTime parses Info.Updated, or returns the zero time.
func (i Info) BuildTime() time.Time {
	t, _ := time.Parse(time.DateOnly, i.Updated)
	return t
}
