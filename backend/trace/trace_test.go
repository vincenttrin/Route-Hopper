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
