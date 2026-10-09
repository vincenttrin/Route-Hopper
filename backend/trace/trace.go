// Package trace runs the system traceroute and parses its output.
package trace

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultMaxHops = 30
	MaxHops        = 64
)

// Runner executes a traceroute command and calls onLine with each line of its
// stdout as soon as it is printed. It returns once the command has exited, or
// the context ended first, in which case it stops the command and returns an
// error.
type Runner func(ctx context.Context, name string, args []string, onLine func(line string)) error

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
		ICMPTimeout:  15 * time.Second,
		DNSTimeout:   2 * time.Second,
		ProbesPerHop: 2,
		WaitSeconds:  1,
	}
}

func execRun(ctx context.Context, name string, args []string, onLine func(string)) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 2 * time.Second
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	// Cancelling ctx kills the process, which closes the pipe and ends the scan.
	sc := bufio.NewScanner(stdout)
	for sc.Scan() {
		onLine(sc.Text())
	}
	err = cmd.Wait()
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

// Phase names a stage of a trace that the caller may want to announce.
type Phase string

// PhaseICMP is the retry with ICMP echo probes, run when the destination did
// not answer the default UDP probes.
const PhaseICMP Phase = "icmp"

// Sink receives the progress of a trace. Every field is optional. Callbacks are
// never called concurrently and arrive in order.
type Sink struct {
	// Hop is called for each hop as soon as it is parsed, with its reverse DNS
	// name filled in.
	Hop func(Hop)
	// Phase announces a stage that takes a while with no hops to show.
	Phase func(Phase)
	// Reset tells the caller to discard the hops it has seen: the ICMP pass got
	// further than the UDP pass and its hops follow.
	Reset func()
}

// Trace runs traceroute towards ip and returns every hop once it has finished.
// See Stream for how the route is probed.
func (t *Tracer) Trace(ctx context.Context, ip netip.Addr, maxHops int) ([]Hop, error) {
	return t.Stream(ctx, ip, maxHops, Sink{})
}

// Stream runs traceroute towards ip, handing each hop to sink as traceroute
// prints it. If the destination does not answer the default UDP probes, the
// route is probed again with ICMP echo, which hosts such as facebook.com
// answer. If the time limit is hit, the hops seen so far are returned rather
// than an error. It returns the hops that stand at the end, which are the ones
// the sink last saw.
func (t *Tracer) Stream(ctx context.Context, ip netip.Addr, maxHops int, sink Sink) ([]Hop, error) {
	if maxHops <= 0 {
		maxHops = DefaultMaxHops
	}
	if maxHops > MaxHops {
		maxHops = MaxHops
	}
	runCtx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	hops, err := t.pass(ctx, runCtx, ip, maxHops, false, sink.Hop)
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
	// needs CAP_NET_RAW on Linux, and some hosts ignore it as well. Its hops are
	// held back until then, since they would otherwise repeat the UDP ones.
	if !Reached(hops, ip) && ctx.Err() == nil {
		if sink.Phase != nil {
			sink.Phase(PhaseICMP)
		}
		icmpCtx, icmpCancel := context.WithTimeout(ctx, t.ICMPTimeout)
		defer icmpCancel()
		if icmp, _ := t.pass(ctx, icmpCtx, ip, maxHops, true, nil); Reached(icmp, ip) {
			hops = icmp
			if sink.Reset != nil {
				sink.Reset()
			}
			if sink.Hop != nil {
				for _, h := range hops {
					sink.Hop(h)
				}
			}
		}
	}
	return hops, nil
}

func binFor(ip netip.Addr) string {
	if ip.Is6() {
		return "traceroute6"
	}
	return "traceroute"
}

// pass runs one traceroute pass, with ICMP echo probes when icmp is set, and
// parses what it prints while it runs. Each hop's reverse lookup starts as soon
// as the hop is parsed; hops reach emit (if set) in order once resolved. The
// process runs under runCtx, lookups under ctx so that the hops printed just
// before runCtx expires still get names. Hops printed before an error are
// still returned.
func (t *Tracer) pass(ctx, runCtx context.Context, ip netip.Addr, maxHops int, icmp bool, emit func(Hop)) ([]Hop, error) {
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

	type pending struct {
		hop  *Hop
		done chan struct{}
	}
	queue := make(chan pending, MaxHops)
	var hops []Hop
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for p := range queue {
			<-p.done
			hops = append(hops, *p.hop)
			if emit != nil {
				emit(*p.hop)
			}
		}
	}()

	err := t.Run(runCtx, binFor(ip), args, func(line string) {
		h, ok := ParseLine(line)
		if !ok {
			return
		}
		p := pending{hop: &h, done: make(chan struct{})}
		go func() {
			defer close(p.done)
			t.resolveName(ctx, p.hop)
		}()
		queue <- p
	})
	close(queue)
	<-finished
	return hops, err
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

// resolveName fills in the reverse DNS name of h.
func (t *Tracer) resolveName(ctx context.Context, h *Hop) {
	if !h.IP.IsValid() {
		return
	}
	lctx, cancel := context.WithTimeout(ctx, t.DNSTimeout)
	defer cancel()
	if names, err := t.LookupAddr(lctx, h.IP.String()); err == nil && len(names) > 0 {
		h.Hostname = strings.TrimSuffix(names[0], ".")
	}
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
