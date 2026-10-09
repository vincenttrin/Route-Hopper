package trace

import (
	"context"
	"errors"
	"net/netip"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func newTestTracer(run Runner) *Tracer {
	return &Tracer{
		Run: run,
		LookupAddr: func(_ context.Context, ip string) ([]string, error) {
			if ip == "10.0.0.1" {
				return []string{"gw.example.net."}, nil
			}
			return nil, errors.New("nxdomain")
		},
		Timeout:      time.Second,
		ICMPTimeout:  time.Second,
		DNSTimeout:   time.Second,
		ProbesPerHop: 2,
		WaitSeconds:  1,
	}
}

func TestTraceBuildsCommandAndResolvesNames(t *testing.T) {
	var gotName string
	var gotArgs []string
	tr := newTestTracer(func(_ context.Context, name string, args ...string) ([]byte, error) {
		gotName, gotArgs = name, args
		return []byte(linuxOutput), nil
	})
	hops, err := tr.Trace(context.Background(), netip.MustParseAddr("93.184.216.34"), 500)
	if err != nil {
		t.Fatal(err)
	}
	if gotName != "traceroute" {
		t.Errorf("binary = %q", gotName)
	}
	if want := "-n -m 64 -q 2 -w 1 93.184.216.34"; strings.Join(gotArgs, " ") != want {
		t.Errorf("args = %q, want %q", strings.Join(gotArgs, " "), want)
	}
	if len(hops) != 3 || hops[0].Hostname != "gw.example.net" || hops[1].Hostname != "" {
		t.Errorf("hops = %+v", hops)
	}
}

func TestTraceIPv6UsesTraceroute6(t *testing.T) {
	var gotName string
	tr := newTestTracer(func(_ context.Context, name string, _ ...string) ([]byte, error) {
		gotName = name
		return []byte(macOutput), nil
	})
	if _, err := tr.Trace(context.Background(), netip.MustParseAddr("2606:4700::1111"), 0); err != nil {
		t.Fatal(err)
	}
	if gotName != "traceroute6" {
		t.Errorf("binary = %q", gotName)
	}
}

func TestTraceErrors(t *testing.T) {
	ip := netip.MustParseAddr("1.1.1.1")

	missing := newTestTracer(func(context.Context, string, ...string) ([]byte, error) {
		return nil, &exec.Error{Name: "traceroute", Err: exec.ErrNotFound}
	})
	if _, err := missing.Trace(context.Background(), ip, 5); err == nil || !strings.Contains(err.Error(), "not installed") {
		t.Errorf("missing binary err = %v", err)
	}

	// A timeout with partial output still yields the hops seen so far.
	partial := newTestTracer(func(context.Context, string, ...string) ([]byte, error) {
		return []byte(" 1  10.0.0.1  1 ms\n"), context.DeadlineExceeded
	})
	hops, err := partial.Trace(context.Background(), ip, 5)
	if err != nil || len(hops) != 1 {
		t.Errorf("partial = %+v, %v", hops, err)
	}

	// A timeout with no output is an error the handler can map to 504.
	empty := newTestTracer(func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	empty.Timeout = 20 * time.Millisecond
	if _, err := empty.Trace(context.Background(), ip, 5); err == nil {
		t.Error("expected error when nothing was traced")
	}
}

// facebookUDP is what traceroute's default UDP probes see towards a host such as
// facebook.com: the path answers, then the destination silently drops the probes.
const facebookUDP = `traceroute to 57.144.20.1 (57.144.20.1), 30 hops max, 40 byte packets
 1  192.168.1.1  4.3 ms  5.1 ms
 2  96.34.20.4  15.2 ms  16.0 ms
 3  157.240.69.22  88.4 ms  66.1 ms
 4  * *
 5  * *
 6  * *
`

// facebookICMP is the same path probed with ICMP echo, which the host answers.
const facebookICMP = `traceroute to 57.144.20.1 (57.144.20.1), 30 hops max, 48 byte packets
 1  192.168.1.1  4.4 ms  12.4 ms
 2  96.34.20.4  15.3 ms  54.0 ms
 3  157.240.69.22  88.4 ms  66.1 ms
 4  57.144.20.1  68.6 ms  159.8 ms
`

// probeRunner answers UDP and ICMP (-I) traceroute runs separately and records
// which kinds were run, in order.
func probeRunner(udp, icmp []byte, icmpErr error, calls *[]string) Runner {
	return func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "-I" {
			*calls = append(*calls, "icmp")
			return icmp, icmpErr
		}
		*calls = append(*calls, "udp")
		return udp, nil
	}
}

func TestTraceFallsBackToICMPWhenDestinationIgnoresUDP(t *testing.T) {
	var calls []string
	tr := newTestTracer(probeRunner([]byte(facebookUDP), []byte(facebookICMP), nil, &calls))
	dest := netip.MustParseAddr("57.144.20.1")
	hops, err := tr.Trace(context.Background(), dest, 30)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls, ",") != "udp,icmp" {
		t.Errorf("calls = %v", calls)
	}
	if !Reached(hops, dest) || len(hops) != 4 {
		t.Errorf("hops = %+v", hops)
	}
}

func TestTraceICMPPassRunsAfterUDPUsesWholeBudget(t *testing.T) {
	tr := newTestTracer(func(ctx context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "-I" {
			return []byte(facebookICMP), nil
		}
		<-ctx.Done()
		return []byte(facebookUDP), ctx.Err()
	})
	tr.Timeout = 20 * time.Millisecond
	dest := netip.MustParseAddr("57.144.20.1")
	hops, err := tr.Trace(context.Background(), dest, 30)
	if err != nil {
		t.Fatal(err)
	}
	if !Reached(hops, dest) {
		t.Errorf("hops = %+v", hops)
	}
}

func TestTraceICMPPassUsesICMPFlag(t *testing.T) {
	var icmpArgs string
	tr := newTestTracer(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "-I" {
			icmpArgs = strings.Join(args, " ")
			return []byte(facebookICMP), nil
		}
		return []byte(facebookUDP), nil
	})
	if _, err := tr.Trace(context.Background(), netip.MustParseAddr("57.144.20.1"), 30); err != nil {
		t.Fatal(err)
	}
	if want := "-I -n -m 30 -q 2 -w 1 57.144.20.1"; icmpArgs != want {
		t.Errorf("icmp args = %q, want %q", icmpArgs, want)
	}
}

func TestTraceSkipsICMPWhenUDPReachesDestination(t *testing.T) {
	var calls []string
	tr := newTestTracer(probeRunner([]byte(linuxOutput), []byte(facebookICMP), nil, &calls))
	if _, err := tr.Trace(context.Background(), netip.MustParseAddr("93.184.216.34"), 30); err != nil {
		t.Fatal(err)
	}
	if strings.Join(calls, ",") != "udp" {
		t.Errorf("calls = %v", calls)
	}
}

func TestTraceKeepsUDPResultWhenICMPDoesNotHelp(t *testing.T) {
	dest := netip.MustParseAddr("57.144.20.1")
	tests := []struct {
		name string
		icmp []byte
		err  error
	}{
		// Without CAP_NET_RAW traceroute -I exits straight away with nothing.
		{"icmp not permitted", nil, errors.New("exit status 1")},
		{"destination ignores icmp too", []byte(facebookUDP), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			tr := newTestTracer(probeRunner([]byte(facebookUDP), tt.icmp, tt.err, &calls))
			hops, err := tr.Trace(context.Background(), dest, 30)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(calls, ",") != "udp,icmp" {
				t.Errorf("calls = %v", calls)
			}
			if Reached(hops, dest) || len(hops) != 6 {
				t.Errorf("hops = %+v", hops)
			}
		})
	}
}

func TestReached(t *testing.T) {
	dest := netip.MustParseAddr("57.144.20.1")
	tests := []struct {
		name string
		hops []Hop
		want bool
	}{
		{"no hops", nil, false},
		{"destination answered", []Hop{{Number: 1, IP: netip.MustParseAddr("192.168.1.1")}, {Number: 2, IP: dest}}, true},
		{"path answered, destination silent", []Hop{{Number: 1, IP: netip.MustParseAddr("192.168.1.1")}, {Number: 2}}, false},
		{"ipv4-mapped destination", []Hop{{Number: 1, IP: netip.MustParseAddr("::ffff:57.144.20.1")}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Reached(tt.hops, dest); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNoRepliesBeyondFirstHop(t *testing.T) {
	silent := Hop{}
	answered := Hop{IP: netip.MustParseAddr("203.0.113.9")}
	tests := []struct {
		name string
		hops []Hop
		want bool
	}{
		{"no hops", nil, false},
		{"docker desktop: gateway then silence", []Hop{answered, silent, silent, silent}, true},
		{"nothing answered at all", []Hop{silent, silent, silent}, true},
		{"later hop answered", []Hop{answered, silent, answered}, false},
		{"first hop silent but later answered", []Hop{silent, silent, answered}, false},
		{"fully answered", []Hop{answered, answered, answered}, false},
		{"too short to judge", []Hop{answered, silent}, false},
		{"single hop", []Hop{answered}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NoRepliesBeyondFirstHop(tt.hops); got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewTracerGivesICMPPassABudget(t *testing.T) {
	tr := NewTracer()
	if tr.Timeout <= 0 || tr.ICMPTimeout <= 0 {
		t.Errorf("Timeout = %v, ICMPTimeout = %v", tr.Timeout, tr.ICMPTimeout)
	}
	var icmpDeadline time.Duration
	tr.LookupAddr = func(context.Context, string) ([]string, error) { return nil, errors.New("nxdomain") }
	tr.Run = func(ctx context.Context, _ string, args ...string) ([]byte, error) {
		if args[0] == "-I" {
			dl, ok := ctx.Deadline()
			if !ok {
				t.Error("ICMP pass has no deadline")
			}
			icmpDeadline = time.Until(dl)
			return []byte(facebookICMP), nil
		}
		return []byte(facebookUDP), nil
	}
	dest := netip.MustParseAddr("57.144.20.1")
	hops, err := tr.Trace(context.Background(), dest, 30)
	if err != nil || !Reached(hops, dest) {
		t.Fatalf("hops = %+v, err = %v", hops, err)
	}
	if icmpDeadline <= 0 {
		t.Errorf("ICMP pass deadline already expired: %v", icmpDeadline)
	}
}
