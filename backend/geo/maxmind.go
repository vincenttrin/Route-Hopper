package geo

import (
	"fmt"
	"net/netip"
	"time"

	"github.com/oschwald/geoip2-golang/v2"
)

// MaxMind reads GeoLite2/GeoIP2 City and (optionally) ASN databases.
type MaxMind struct {
	city *geoip2.Reader
	asn  *geoip2.Reader // may be nil
}

// OpenMaxMind opens the City database and, when asnPath is not empty, the ASN
// database used to fill in the network owner.
func OpenMaxMind(cityPath, asnPath string) (*MaxMind, error) {
	city, err := geoip2.Open(cityPath)
	if err != nil {
		return nil, fmt.Errorf("open city database: %w", err)
	}
	m := &MaxMind{city: city}
	if asnPath != "" {
		if m.asn, err = geoip2.Open(asnPath); err != nil {
			city.Close()
			return nil, fmt.Errorf("open asn database: %w", err)
		}
	}
	return m, nil
}

func (m *MaxMind) Lookup(ip netip.Addr) (Location, bool) {
	var loc Location
	var found bool
	if rec, err := m.city.City(ip); err == nil && rec.Location.HasCoordinates() {
		loc.Lat, loc.Lng = *rec.Location.Latitude, *rec.Location.Longitude
		loc.City = rec.City.Names.English
		loc.Country = rec.Country.ISOCode
		found = true
	}
	if m.asn != nil {
		if rec, err := m.asn.ASN(ip); err == nil && rec.AutonomousSystemOrganization != "" {
			loc.Org = rec.AutonomousSystemOrganization
			found = true
		}
	}
	return loc, found
}

// Info reports the City database's publisher and build date.
func (m *MaxMind) Info() Info {
	meta := m.city.Metadata()
	return Info{
		Available: true,
		Provider:  provider(meta.DatabaseType),
		Updated:   meta.BuildTime().UTC().Format(time.DateOnly),
	}
}

func (m *MaxMind) Close() error {
	if m.asn != nil {
		m.asn.Close()
	}
	return m.city.Close()
}
