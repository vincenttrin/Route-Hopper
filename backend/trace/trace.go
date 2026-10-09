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
	Timeout      time.Duration // overall limit for one trace
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

// Trace runs traceroute towards ip. If the time limit is hit, the hops seen so
// far are returned rather than an error.
func (t *Tracer) Trace(ctx context.Context, ip netip.Addr, maxHops int) ([]Hop, error) {
	if maxHops <= 0 {
		maxHops = DefaultMaxHops
	}
	if maxHops > MaxHops {
		maxHops = MaxHops
	}
	bin := "traceroute"
	if ip.Is6() {
		bin = "traceroute6"
	}
	args := []string{
		"-n",
		"-m", strconv.Itoa(maxHops),
		"-q", strconv.Itoa(t.ProbesPerHop),
		"-w", strconv.Itoa(t.WaitSeconds),
		ip.String(),
	}
	runCtx, cancel := context.WithTimeout(ctx, t.Timeout)
	defer cancel()
	out, err := t.Run(runCtx, bin, args...)
	hops := Parse(string(out))
	if err != nil && len(hops) == 0 {
		var notFound *exec.Error
		if errors.As(err, &notFound) {
			return nil, fmt.Errorf("%s is not installed on the server", bin)
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%s failed: %w", bin, err)
	}
	t.resolveNames(ctx, hops)
	return hops, nil
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
