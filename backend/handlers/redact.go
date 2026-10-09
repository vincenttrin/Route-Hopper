package handlers

import (
	"net/netip"
	"strings"

	"github.com/tamtrinh/traffic-visualizer/backend/geo"
	"github.com/tamtrinh/traffic-visualizer/backend/trace"
)

// NearSourceKm is how close to the first public hop a later hop must be located
// to count as part of the source's own network. See redactor.
const NearSourceKm = 100

// cgnat is the carrier-grade NAT range: addresses inside an ISP, which
// geo.IsPublic counts as public but which are not routable on the internet.
var cgnat = netip.MustParsePrefix("100.64.0.0/10")

// leavesNetwork reports whether ip is a routable public address, one that
// could be the first hop outside the source's own network.
func leavesNetwork(ip netip.Addr) bool {
	return geo.IsPublic(ip) && !cgnat.Contains(ip.Unmap())
}

// redactor hides where a trace starts from. The source is the server's own
// network, and the first hops of a traceroute describe it: the LAN gateway, then
// the ISP's routers around it, with their addresses, host names and locations
// (an ISP's router names usually carry a city code). None of that may reach the
// client. The redactor cuts the start of the route and replaces everything it
// removed with ONE placeholder hop (Hidden, nothing else set), so neither the
// hops' details nor how many there were gets out.
//
// The hidden segment, the "origin", is:
//   - every hop before the first public one: private and loopback addresses,
//     CGNAT and silent hops (the LAN, the router, the ISP's inner network);
//   - the first public hop: the ISP's first router on the internet;
//   - each following hop that belongs to the same network as that first public
//     hop (same network owner in the ASN database, or same registered domain in
//     the reverse DNS name) or is located within NearSourceKm of it. That is the
//     access ISP's own backbone, up to where it hands the packet to another
//     network. Silent and private hops in between are part of it too. The origin
//     ends at the first public hop that is none of these.
//
// The destination hop is never hidden. Without a database or reverse DNS names
// nothing says which later hops belong to the ISP, so only the first public hop
// (and what precedes it) is hidden.
//
// Hops reach the redactor in order, one at a time, as the trace streams.
// Add returns the hops that are ready to send, Flush the rest once the trace is
// over, and reset starts over when the trace restarts (the ICMP retry).
type redactor struct {
	locate func(netip.Addr) (geo.Location, bool)
	enrich func(trace.Hop) hopJSON
	dest   netip.Addr

	sent      int         // hops handed out so far, which numbers the next one
	hidden    bool        // the placeholder has been sent
	anchor    originHop   // the first public hop
	sawPublic bool        // the first public hop has passed
	done      bool        // the origin has ended: everything else is exposed
	held      []trace.Hop // silent or private hops after the first public hop, not yet known to be in or out
}

// originHop is what is known about the first public hop, to recognise the rest of its network.
type originHop struct {
	loc    *geo.Location
	org    string
	domain string
}

// registeredDomain is the last two labels of a host name ("cox.net" for "ae-1.rtr.omaha.cox.net"), or "".
func registeredDomain(host string) string {
	labels := strings.Split(strings.ToLower(strings.TrimSuffix(host, ".")), ".")
	if len(labels) < 2 {
		return ""
	}
	return strings.Join(labels[len(labels)-2:], ".")
}

func originOf(h trace.Hop, loc geo.Location, located bool) originHop {
	o := originHop{org: loc.Org, domain: registeredDomain(h.Hostname)}
	if located && (loc.Lat != 0 || loc.Lng != 0) {
		o.loc = &loc
	}
	return o
}

// contains reports whether a public hop belongs to the same network as the first one.
func (o originHop) contains(h trace.Hop, loc geo.Location, located bool) bool {
	if located && o.org != "" && loc.Org == o.org {
		return true
	}
	if o.domain != "" && registeredDomain(h.Hostname) == o.domain {
		return true
	}
	return located && o.loc != nil && geo.DistanceKm(*o.loc, loc) <= NearSourceKm
}

func newRedactor(dest netip.Addr, locate func(netip.Addr) (geo.Location, bool), enrich func(trace.Hop) hopJSON) *redactor {
	return &redactor{dest: dest, locate: locate, enrich: enrich}
}

func (r *redactor) reset() {
	*r = redactor{dest: r.dest, locate: r.locate, enrich: r.enrich}
}

func (r *redactor) isDest(h trace.Hop) bool {
	return h.IP.IsValid() && h.IP.Unmap() == r.dest.Unmap()
}

// emit numbers a hop and converts it for the client.
func (r *redactor) emit(h trace.Hop) hopJSON {
	r.sent++
	hj := r.enrich(h)
	hj.HopNumber = r.sent
	return hj
}

// hide sends the placeholder the first time something is hidden.
func (r *redactor) hide() []hopJSON {
	if r.hidden {
		return nil
	}
	r.hidden = true
	r.sent++
	return []hopJSON{{HopNumber: r.sent, Hidden: true}}
}

// finish ends the origin and returns the hops held back, then h.
func (r *redactor) finish(h trace.Hop) []hopJSON {
	r.done = true
	var out []hopJSON
	for _, p := range r.held {
		out = append(out, r.emit(p))
	}
	r.held = nil
	return append(out, r.emit(h))
}

func (r *redactor) Add(h trace.Hop) []hopJSON {
	if r.done {
		return []hopJSON{r.emit(h)}
	}
	if r.isDest(h) {
		return r.finish(h)
	}
	if !r.sawPublic {
		out := r.hide()
		if leavesNetwork(h.IP) {
			r.sawPublic = true
			loc, ok := r.locate(h.IP)
			r.anchor = originOf(h, loc, ok)
		}
		return out
	}
	// Past the first public hop: still in the origin while hops stay close to it.
	if !leavesNetwork(h.IP) {
		r.held = append(r.held, h)
		return nil
	}
	if loc, ok := r.locate(h.IP); r.anchor.contains(h, loc, ok) {
		r.held = nil
		return nil
	}
	return r.finish(h)
}

// Flush returns what is still held once the trace has ended.
func (r *redactor) Flush() []hopJSON {
	var out []hopJSON
	for _, p := range r.held {
		out = append(out, r.emit(p))
	}
	r.held = nil
	return out
}
