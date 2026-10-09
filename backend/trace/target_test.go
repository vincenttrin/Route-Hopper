package trace

import (
	"context"
	"errors"
	"net/netip"
	"testing"
)

func TestParseEndpoint(t *testing.T) {
	good := map[string]string{
		"example.com":                    "example.com",
		"  Example.com. ":                "Example.com",
		"https://example.com/a/b?c=d":    "example.com",
		"http://user@example.com:8080/x": "example.com",
		"1.2.3.4":                        "1.2.3.4",
		"1.2.3.4:443":                    "1.2.3.4",
		"[2606:4700::1111]:443":          "2606:4700::1111",
		"2606:4700::1111":                "2606:4700::1111",
		"::ffff:1.2.3.4":                 "1.2.3.4",
	}
	for in, want := range good {
		got, err := ParseEndpoint(in)
		if err != nil || got != want {
			t.Errorf("ParseEndpoint(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"", "   ", "-m 1", "--help", "a b", "exa_mple.com", "foo..bar", "-x.com", "$(id).com", ";ls", "http://"} {
		if got, err := ParseEndpoint(in); !errors.Is(err, ErrInvalidEndpoint) {
			t.Errorf("ParseEndpoint(%q) = %q, %v; want ErrInvalidEndpoint", in, got, err)
		}
	}
}

type fakeResolver map[string][]netip.Addr

func (f fakeResolver) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	if a, ok := f[host]; ok {
		return a, nil
	}
	return nil, errors.New("no such host")
}

func TestResolve(t *testing.T) {
	r := fakeResolver{
		"dual.test":  {netip.MustParseAddr("2606:4700::1111"), netip.MustParseAddr("1.1.1.1")},
		"local.test": {netip.MustParseAddr("10.1.2.3")},
	}
	ctx := context.Background()

	if ip, err := Resolve(ctx, r, "dual.test", false); err != nil || ip.String() != "1.1.1.1" {
		t.Errorf("dual = %v, %v; want IPv4 preferred", ip, err)
	}
	if ip, err := Resolve(ctx, r, "8.8.8.8", false); err != nil || ip.String() != "8.8.8.8" {
		t.Errorf("literal = %v, %v", ip, err)
	}
	if _, err := Resolve(ctx, r, "missing.test", false); !errors.Is(err, ErrUnresolvable) {
		t.Errorf("missing err = %v", err)
	}
	for _, h := range []string{"local.test", "127.0.0.1", "192.168.1.1", "169.254.1.1", "::1", "0.0.0.0", "224.0.0.1"} {
		if _, err := Resolve(ctx, r, h, false); !errors.Is(err, ErrForbiddenTarget) {
			t.Errorf("Resolve(%q) err = %v; want ErrForbiddenTarget", h, err)
		}
	}
	if _, err := Resolve(ctx, r, "local.test", true); err != nil {
		t.Errorf("allowPrivate: %v", err)
	}
}
