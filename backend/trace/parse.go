package trace

import (
	"math"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
)

// Hop is one row of traceroute output. IP is empty when no probe was answered.
type Hop struct {
	Number   int
	IP       netip.Addr
	Hostname string
	RTT      float64 // mean round trip in milliseconds, 0 when unanswered
}

var hopLine = regexp.MustCompile(`^\s*(\d+)\s+(.*)$`)

// Parse extracts hops from Linux and macOS traceroute output run with -n.
// Header lines and ECMP continuation lines (no hop number) are skipped, so a
// hop that answered from several routers reports the first one.
func Parse(output string) []Hop {
	var hops []Hop
	for _, line := range strings.Split(output, "\n") {
		m := hopLine.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n, _ := strconv.Atoi(m[1])
		hops = append(hops, parseProbes(n, m[2]))
	}
	return hops
}

// parseProbes reads the probe results of one hop, e.g. "10.0.0.1  1.2 ms  1.4 ms".
// RTTs are only counted while they belong to the first address on the line.
func parseProbes(n int, rest string) Hop {
	hop := Hop{Number: n}
	fields := strings.Fields(rest)
	var sum float64
	var count int
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if ip, err := netip.ParseAddr(strings.Trim(f, "()")); err == nil {
			if hop.IP.IsValid() && ip != hop.IP {
				break
			}
			hop.IP = ip
			continue
		}
		if i+1 < len(fields) && fields[i+1] == "ms" {
			if v, err := strconv.ParseFloat(f, 64); err == nil {
				sum += v
				count++
			}
		}
	}
	if count > 0 {
		hop.RTT = math.Round(sum/float64(count)*100) / 100
	}
	return hop
}
