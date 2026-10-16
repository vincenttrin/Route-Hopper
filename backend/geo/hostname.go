package geo

import (
	_ "embed"
	"encoding/csv"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// airportsCSV is the bundled airport table (see data/airports.csv and
// gen_airports.go). Nothing is fetched at trace time.
//
//go:embed data/airports.csv
var airportsCSV string

// Where a hop's coordinates came from.
const (
	SourceHostname = "hostname"
	SourceGeoIP    = "geoip"
)

// HostnameLocator places a router from the location code in its reverse DNS
// name, such as ord3 in ae1.pr02.ord3.example.net. Operators name routers
// after the site they sit in, which is far more reliable than a GeoIP guess
// for router infrastructure. It only recognises what it can decode with
// confidence, so most host names yield nothing and fall back to GeoIP.
type HostnameLocator struct {
	airports map[string]Location // by lower-case IATA code
}

// NewHostnameLocator loads the bundled airport table.
func NewHostnameLocator() (*HostnameLocator, error) {
	rows, err := csv.NewReader(strings.NewReader(airportsCSV)).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read airports: %w", err)
	}
	h := &HostnameLocator{airports: make(map[string]Location, len(rows))}
	for _, r := range rows[1:] { // the first row is the header
		if len(r) != 5 {
			return nil, fmt.Errorf("airports: want 5 columns, got %d in %v", len(r), r)
		}
		lat, err1 := strconv.ParseFloat(r[3], 64)
		lng, err2 := strconv.ParseFloat(r[4], 64)
		if err1 != nil || err2 != nil {
			return nil, fmt.Errorf("airports: bad coordinates in %v", r)
		}
		h.airports[strings.ToLower(r[0])] = Location{Lat: lat, Lng: lng, City: r[1], Country: r[2]}
	}
	return h, nil
}

// Locate returns the position a host name points at and the token it was read
// from, or false when the name carries no location code it recognises.
//
// The registered domain (example.net) is ignored: it names the operator, and
// operators have codes of their own. The remaining labels are read from the
// right, so that in ae1.pr02.ord3.example.net the site label wins over
// anything to its left, and each label is split at dashes so that
// be-100-ord-core is read as well. The first token that decodes wins.
func (h *HostnameLocator) Locate(hostname string) (Location, string, bool) {
	labels := strings.Split(strings.ToLower(strings.TrimSuffix(strings.TrimSpace(hostname), ".")), ".")
	labels = labels[:len(labels)-registeredDomainLabels(labels)]
	for i := len(labels) - 1; i >= 0; i-- {
		parts := strings.Split(labels[i], "-")
		for j := len(parts) - 1; j >= 0; j-- {
			if loc, ok := h.decode(parts[j]); ok {
				return loc, parts[j], true
			}
		}
	}
	return Location{}, "", false
}

// secondLevelWords are the labels that sit under a two-letter country code as
// part of the public suffix (example.co.uk), not as a registered name.
var secondLevelWords = map[string]bool{
	"co": true, "com": true, "net": true, "org": true, "edu": true, "gov": true, "ac": true, "ne": true, "or": true, "go": true,
}

// registeredDomainLabels is how many labels at the end of a name belong to the
// operator's domain. It guesses at the public suffix rather than carrying the
// whole list: wrongly keeping or dropping a label there costs at most a missed
// or a spurious match on a label that has to decode as a location anyway.
func registeredDomainLabels(labels []string) int {
	n := len(labels)
	if n >= 3 && len(labels[n-1]) == 2 && secondLevelWords[labels[n-2]] {
		return 3
	}
	return min(n, 2)
}

var (
	// iataToken is an airport code with the site number operators append:
	// ord, ord3, ord03, and Google-style lax17s55.
	iataToken = regexp.MustCompile(`^([a-z]{3})(?:\d{1,3}(?:s\d{1,3})?)?$`)
	// clliToken is a 6-letter CLLI city and state code with an optional
	// 2-character building suffix: nycmny, nycmny01, nycmnyzr.
	clliToken = regexp.MustCompile(`^([a-z]{6})(?:[a-z0-9]{2})?$`)
	// nameToken is a letters-only word with an optional site number: chicago2.
	nameToken = regexp.MustCompile(`^([a-z]{3,})\d{0,3}$`)
)

// decode reads one token as a location, trying the most specific convention
// first.
func (h *HostnameLocator) decode(tok string) (Location, bool) {
	if m := clliToken.FindStringSubmatch(tok); m != nil {
		if s, ok := clliSites[m[1]]; ok {
			return h.site(s)
		}
	}
	if m := nameToken.FindStringSubmatch(tok); m != nil {
		if s, ok := citySites[m[1]]; ok {
			return h.site(s)
		}
	}
	if m := iataToken.FindStringSubmatch(tok); m != nil && !notAirports[m[1]] {
		if loc, ok := h.airports[m[1]]; ok {
			return loc, true
		}
	}
	return Location{}, false
}

// site resolves a named site to the coordinates of its airport.
func (h *HostnameLocator) site(s site) (Location, bool) {
	loc, ok := h.airports[strings.ToLower(s.airport)]
	if !ok {
		return Location{}, false
	}
	loc.City = s.city
	return loc, true
}

// site is a place named by something other than its own airport code. Its
// position is that of the airport, which is within a few tens of kilometres of
// the city centre and plenty for a map of a route.
type site struct{ city, airport string }

// notAirports are three-letter labels that are airport codes but far more
// often mean something else in a router name: roles, interface types, vendor
// models and ordinary words. Reading them as places would put routers on
// another continent. Add to this list when a bad match turns up; it costs a
// missed location at worst.
var notAirports = setOf(
	"aar", "acc", "acx", "act", "add", "ags", "asr", "asw", "bar", "bbr", "bgw", "bsr", "cap", "cat", "col", "cor",
	"dig", "dog", "ear", "ext", "for", "get", "gig", "isp", "lan", "lap", "law", "let", "lns", "lsr", "man", "mgt",
	"mlx", "mob", "nas", "nat", "noc", "ont", "ops", "pat", "pop", "pos", "ppp", "pri", "qfx", "res", "row", "run",
	"san", "sdn", "sig", "srv", "ssw", "stg", "sub", "svr", "sys", "tac", "ten", "the", "tun", "vpn", "win",
)

func setOf(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

// citySites are city abbreviations and full names used in router names. The
// abbreviations are metro codes that no airport table has (chi, nyc, wdc); the
// names are long enough that they cannot be anything else.
var citySites = map[string]site{
	"chi": {"Chicago", "ORD"}, "nyc": {"New York", "JFK"}, "wdc": {"Washington, D.C.", "IAD"},
	"lon": {"London", "LHR"}, "par": {"Paris", "CDG"}, "tyo": {"Tokyo", "HND"},
	"sel": {"Seoul", "ICN"}, "sao": {"São Paulo", "GRU"}, "ash": {"Ashburn", "IAD"},

	"chicago": {"Chicago", "ORD"}, "newyork": {"New York", "JFK"}, "losangeles": {"Los Angeles", "LAX"},
	"sanfrancisco": {"San Francisco", "SFO"}, "sanjose": {"San Jose", "SJC"}, "seattle": {"Seattle", "SEA"},
	"dallas": {"Dallas", "DFW"}, "houston": {"Houston", "IAH"}, "atlanta": {"Atlanta", "ATL"},
	"miami": {"Miami", "MIA"}, "denver": {"Denver", "DEN"}, "boston": {"Boston", "BOS"},
	"ashburn": {"Ashburn", "IAD"}, "sandiego": {"San Diego", "SAN"}, "lasvegas": {"Las Vegas", "LAS"},
	"amsterdam": {"Amsterdam", "AMS"}, "frankfurt": {"Frankfurt", "FRA"}, "london": {"London", "LHR"},
	"paris": {"Paris", "CDG"}, "tokyo": {"Tokyo", "HND"}, "singapore": {"Singapore", "SIN"},
	"hongkong": {"Hong Kong", "HKG"}, "sydney": {"Sydney", "SYD"}, "toronto": {"Toronto", "YYZ"},
	"vancouver": {"Vancouver", "YVR"}, "montreal": {"Montreal", "YUL"}, "stockholm": {"Stockholm", "ARN"},
	"madrid": {"Madrid", "MAD"}, "milan": {"Milan", "MXP"}, "vienna": {"Vienna", "VIE"},
	"zurich": {"Zurich", "ZRH"}, "warsaw": {"Warsaw", "WAW"}, "mumbai": {"Mumbai", "BOM"},
	"saopaulo": {"São Paulo", "GRU"}, "johannesburg": {"Johannesburg", "JNB"}, "dubai": {"Dubai", "DXB"},
}

// clliSites are the 6-letter CLLI city and state codes (4 letters for the
// place, 2 for the state or province) that carrier backbone names embed, for
// the main North American hubs. Only codes known to be right are listed: a
// CLLI code cannot be decoded by rule, so an unknown one is left to GeoIP.
var clliSites = map[string]site{
	"nycmny": {"New York", "JFK"}, "chcgil": {"Chicago", "ORD"}, "dllstx": {"Dallas", "DFW"},
	"lsanca": {"Los Angeles", "LAX"}, "snjsca": {"San Jose", "SJC"}, "snfcca": {"San Francisco", "SFO"},
	"sttlwa": {"Seattle", "SEA"}, "atlnga": {"Atlanta", "ATL"}, "miamfl": {"Miami", "MIA"},
	"hstntx": {"Houston", "IAH"}, "phnxaz": {"Phoenix", "PHX"}, "dnvrco": {"Denver", "DEN"},
	"wsngdc": {"Washington, D.C.", "IAD"}, "ashbva": {"Ashburn", "IAD"}, "bstnma": {"Boston", "BOS"},
	"phlapa": {"Philadelphia", "PHL"}, "sndgca": {"San Diego", "SAN"}, "lsvnnv": {"Las Vegas", "LAS"},
	"slkcut": {"Salt Lake City", "SLC"}, "kscymo": {"Kansas City", "MCI"}, "stlsmo": {"St. Louis", "STL"},
	"mplsmn": {"Minneapolis", "MSP"}, "dtrtmi": {"Detroit", "DTW"}, "clevoh": {"Cleveland", "CLE"},
	"clmboh": {"Columbus", "CMH"}, "cncnoh": {"Cincinnati", "CVG"}, "pitbpa": {"Pittsburgh", "PIT"},
	"orldfl": {"Orlando", "MCO"}, "tampfl": {"Tampa", "TPA"}, "jcvlfl": {"Jacksonville", "JAX"},
	"chrlnc": {"Charlotte", "CLT"}, "rlghnc": {"Raleigh", "RDU"}, "nshvtn": {"Nashville", "BNA"},
	"mmphtn": {"Memphis", "MEM"}, "nworla": {"New Orleans", "MSY"}, "austtx": {"Austin", "AUS"},
	"snantx": {"San Antonio", "SAT"}, "okcyok": {"Oklahoma City", "OKC"}, "omahne": {"Omaha", "OMA"},
	"sacrca": {"Sacramento", "SMF"}, "bltmmd": {"Baltimore", "BWI"}, "rcmdva": {"Richmond", "RIC"},
	"indpin": {"Indianapolis", "IND"}, "milwwi": {"Milwaukee", "MKE"}, "albqnm": {"Albuquerque", "ABQ"},
	"elpstx": {"El Paso", "ELP"}, "tucsaz": {"Tucson", "TUS"}, "hrfrct": {"Hartford", "BDL"},
	"prvdri": {"Providence", "PVD"}, "bflony": {"Buffalo", "BUF"}, "torton": {"Toronto", "YYZ"},
	"mtrlpq": {"Montreal", "YUL"}, "vancbc": {"Vancouver", "YVR"}, "clgrab": {"Calgary", "YYC"},
	"otwaon": {"Ottawa", "YOW"}, "wnpgmb": {"Winnipeg", "YWG"},
}

// HasCoords reports whether l has a position. A zero latitude and longitude is
// how a lookup without one is recorded.
func (l Location) HasCoords() bool { return l.Lat != 0 || l.Lng != 0 }

// WildDisagreementKm is how far a host name position may be from the GeoIP
// position before the two are said to disagree wildly: past it they are not
// merely different cities of one region but different parts of a continent or
// the world, so one of them is wrong.
const WildDisagreementKm = 3000

// Placement is where a hop was placed and on whose word.
type Placement struct {
	Location
	// Source is SourceHostname or SourceGeoIP, or "" when the hop has no
	// position (it may still carry an Org).
	Source string
	// Hint is the part of the host name the position was read from. Only set
	// for SourceHostname.
	Hint string
	// DisagreementKm is the distance between the host name position and the
	// GeoIP position when both exist, else 0.
	DisagreementKm float64
}

// Disagrees reports whether the host name and GeoIP positions of the hop are
// further apart than WildDisagreementKm. The host name still wins; this is for
// logging and for tuning the heuristics.
func (p Placement) Disagrees() bool { return p.DisagreementKm > WildDisagreementKm }

// Place decides where a hop sits. The position comes from the host name when
// hosts can decode one (h may be nil, which skips this step), and otherwise
// from the GeoIP result ip, found being whether the lookup returned anything.
// The network Org always comes from ip, as host names do not carry it. The
// result is false when there is neither a position nor an Org.
func Place(h *HostnameLocator, hostname string, ip Location, found bool) (Placement, bool) {
	var p Placement
	if found {
		p.Location = ip
		if ip.HasCoords() {
			p.Source = SourceGeoIP
		}
	}
	if h != nil && hostname != "" {
		if loc, hint, ok := h.Locate(hostname); ok {
			if ip.HasCoords() {
				p.DisagreementKm = DistanceKm(loc, ip)
			}
			p.Lat, p.Lng, p.City, p.Country = loc.Lat, loc.Lng, loc.City, loc.Country
			p.Source, p.Hint = SourceHostname, hint
			return p, true
		}
	}
	return p, found
}
