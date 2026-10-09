package trace

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
)

var (
	ErrInvalidEndpoint = errors.New("invalid endpoint")
	ErrUnresolvable    = errors.New("endpoint could not be resolved")
	ErrForbiddenTarget = errors.New("endpoint is not a public address")
)

var hostLabel = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$`)

// ParseEndpoint turns user input such as "example.com", "https://example.com/a?b"
// or "1.2.3.4:443" into a bare host name or IP address. The result never starts
// with "-", so it is safe to pass as a command argument.
func ParseEndpoint(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 2048 {
		return "", ErrInvalidEndpoint
	}
	if ip, err := netip.ParseAddr(s); err == nil {
		return ip.Unmap().String(), nil
	}
	if !strings.Contains(s, "://") {
		s = "//" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", ErrInvalidEndpoint
	}
	host := strings.TrimSuffix(u.Hostname(), ".")
	if ip, err := netip.ParseAddr(host); err == nil {
		return ip.Unmap().String(), nil
	}
	if host == "" || len(host) > 253 {
		return "", ErrInvalidEndpoint
	}
	for _, label := range strings.Split(host, ".") {
		if !hostLabel.MatchString(label) {
			return "", ErrInvalidEndpoint
		}
	}
	return host, nil
}

// Resolver looks up host names.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Resolve returns the address to trace for host, preferring IPv4. Unless
// allowPrivate is set, loopback, private and other non-public targets are
// rejected so the service cannot be used to probe its own network.
func Resolve(ctx context.Context, r Resolver, host string, allowPrivate bool) (netip.Addr, error) {
	ip, err := netip.ParseAddr(host)
	if err != nil {
		addrs, err := r.LookupNetIP(ctx, "ip", host)
		if err != nil || len(addrs) == 0 {
			return netip.Addr{}, fmt.Errorf("%w: %s", ErrUnresolvable, host)
		}
		ip = addrs[0]
		for _, a := range addrs {
			if a.Unmap().Is4() {
				ip = a
				break
			}
		}
	}
	ip = ip.Unmap()
	if !allowPrivate && (!ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLinkLocalUnicast()) {
		return netip.Addr{}, ErrForbiddenTarget
	}
	return ip, nil
}

// SystemResolver resolves with the operating system's resolver.
var SystemResolver Resolver = net.DefaultResolver
