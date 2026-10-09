// Package trace runs the system traceroute and parses its output.
package trace

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	DefaultMaxHops = 30
	MaxHops        = 64
)

// Runner executes a traceroute command and returns its stdout. When the
// context ends first it returns whatever output was produced along with the
// context error.
type Runner func(ctx context.Context, name string, args ...string) ([]byte, error)

// Tracer traces the route to an address.
type Tracer struct {
	Run          Runner
	LookupAddr   func(ctx context.Context, ip string) ([]string, error)
	Timeout      time.Duration // limit for the UDP pass
	ICMPTimeout  time.Duration // separate limit for the ICMP fallback pass
	DNSTimeout   time.Duration // limit for each reverse lookup
	ProbesPerHop int
	WaitSeconds  int
}

// NewTracer returns a Tracer that shells out to traceroute.
func NewTracer() *Tracer {
	return &Tracer{
		Run:          execRun,
		LookupAddr:   net.DefaultResolver.LookupAddr,
		Timeout:      60 * time.Second,
		DNSTimeout:   2 * time.Second,
		ProbesPerHop: 2,
		WaitSeconds:  1,
	}
}

func execRun(ctx context.Context, name string, args ...string) ([]byte, error) {
	var out bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &out
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Run()
	return out.Bytes(), err
}

// Trace runs traceroute towards ip. If the destination does not answer the
// default UDP probes, the route is probed again with ICMP echo, which hosts
// such as facebook.com answer. If the time limit is hit, the hops seen so far
// are returned rather than an error.
func (t *Tracer) Trace(ctx context.Context, ip netip.Addr, maxHops int) ([]Hop, error) {
	if maxHops <= 0 {
		maxHops = DefaultMaxHops
	}
	if maxHops > MaxHops {
		maxHops = MaxHops
	}
	runCtx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	hops, err := t.probe(runCtx, ip, maxHops, false)
	if err != nil && len(hops) == 0 {
		var notFound *exec.Error
		if errors.As(err, &notFound) {
			return nil, fmt.Errorf("%s is not installed on the server", binFor(ip))
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%s failed: %w", binFor(ip), err)
	}
	// The ICMP pass has its own time limit so a long silent UDP tail cannot
	// starve it. It only replaces the UDP hops if it got further: ICMP echo
	// needs CAP_NET_RAW on Linux, and some hosts ignore it as well.
	if !Reached(hops, ip) && ctx.Err() == nil {
		icmpCtx, icmpCancel := context.WithTimeout(ctx, t.ICMPTimeout)
		defer icmpCancel()
		if icmp, _ := t.probe(icmpCtx, ip, maxHops, true); Reached(icmp, ip) {
			hops = icmp
		}
	}
	t.resolveNames(ctx, hops)
	return hops, nil
}

func binFor(ip netip.Addr) string {
	if ip.Is6() {
		return "traceroute6"
	}
	return "traceroute"
}

// probe runs one traceroute pass, with ICMP echo probes when icmp is set, and
// parses what it printed. Output captured before an error is still returned.
func (t *Tracer) probe(ctx context.Context, ip netip.Addr, maxHops int, icmp bool) ([]Hop, error) {
	var args []string
	if icmp {
		args = append(args, "-I")
	}
	args = append(args,
		"-n",
		"-m", strconv.Itoa(maxHops),
		"-q", strconv.Itoa(t.ProbesPerHop),
		"-w", strconv.Itoa(t.WaitSeconds),
		ip.String(),
	)
	out, err := t.Run(ctx, binFor(ip), args...)
	return Parse(string(out)), err
}

// Reached reports whether the destination itself answered a probe, as opposed
// to the trace ending (or timing out) somewhere along the path.
func Reached(hops []Hop, dest netip.Addr) bool {
	for _, h := range hops {
		if h.IP.IsValid() && h.IP.Unmap() == dest.Unmap() {
			return true
		}
	}
	return false
}

// resolveNames fills in reverse DNS names concurrently.
func (t *Tracer) resolveNames(ctx context.Context, hops []Hop) {
	var wg sync.WaitGroup
	for i := range hops {
		if !hops[i].IP.IsValid() {
			continue
		}
		wg.Add(1)
		go func(h *Hop) {
			defer wg.Done()
			lctx, cancel := context.WithTimeout(ctx, t.DNSTimeout)
			defer cancel()
			if names, err := t.LookupAddr(lctx, h.IP.String()); err == nil && len(names) > 0 {
				h.Hostname = strings.TrimSuffix(names[0], ".")
			}
		}(&hops[i])
	}
	wg.Wait()
}

// minHopsForNoReplyCheck is how many hops a trace needs before silence past
// the first one is treated as a sign of a broken network path rather than a
// short route to a host that does not answer.
const minHopsForNoReplyCheck = 3

// NoRepliesBeyondFirstHop reports whether the trace got no reply from any hop
// past the first. This is what traceroute looks like from Docker Desktop on
// macOS and Windows, whose VM network drops the TTL-exceeded replies: the
// container's gateway answers and everything after it is silent.
func NoRepliesBeyondFirstHop(hops []Hop) bool {
	if len(hops) < minHopsForNoReplyCheck {
		return false
	}
	for _, h := range hops[1:] {
		if h.IP.IsValid() {
			return false
		}
	}
	return true
}
