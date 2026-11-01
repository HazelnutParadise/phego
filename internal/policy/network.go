package policy

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

// networkGrant finds the grant covering scheme, host and port.
func (p *Policy) networkGrant(scheme, host string, port int) (NetworkGrant, error) {
	scheme, host = strings.ToLower(scheme), normalizeHost(host)
	target := net.JoinHostPort(host, strconv.Itoa(port))
	for _, g := range p.network {
		if g.Scheme != scheme || !hostMatches(g.Host, host) {
			continue
		}
		for _, allowed := range g.Ports {
			if allowed == port {
				return g, nil
			}
		}
	}
	return NetworkGrant{}, &PolicyError{Kind: Network, Op: scheme, Target: target, Rule: "not-granted"}
}

// normalizeHost folds DNS names but preserves case-sensitive IPv6 zone names.
func normalizeHost(host string) string {
	if a, err := netip.ParseAddr(host); err == nil {
		return a.String()
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}

func hostMatches(pattern, host string) bool {
	if suffix, ok := strings.CutPrefix(pattern, "*"); ok {
		return strings.HasSuffix(host, suffix) && len(host) > len(suffix)
	}
	if a, err := netip.ParseAddr(pattern); err == nil {
		b, err := netip.ParseAddr(host)
		return err == nil && a.Unmap() == b.Unmap()
	}
	return pattern == host
}

// DialContext connects to host:port for the given scheme. The grant is
// checked against the name PHP asked for, and every address the connection
// actually uses is checked when the socket is created, so a granted name
// that resolves to a private address is refused unless the grant allows
// private addresses. Redirects and proxies are the caller's to re-check.
func (p *Policy) DialContext(ctx context.Context, scheme, network, host string, port int) (net.Conn, error) {
	switch network {
	case "tcp", "tcp4", "tcp6", "udp", "udp4", "udp6":
	default:
		return nil, &PolicyError{Kind: Network, Op: scheme, Target: network, Rule: "unsupported-network"}
	}
	grant, err := p.networkGrant(scheme, host, port)
	if err != nil {
		return nil, err
	}
	target := net.JoinHostPort(host, strconv.Itoa(port))
	var mu sync.Mutex
	var refused error
	d := net.Dialer{Control: func(_, address string, _ syscall.RawConn) error {
		ap, err := netip.ParseAddrPort(address)
		var denial error
		if err != nil {
			denial = &PolicyError{Kind: Network, Op: scheme, Target: target, Rule: "unparsable-address"}
		} else if !grant.AllowPrivate && !Public(ap.Addr()) {
			denial = &PolicyError{Kind: Network, Op: scheme, Target: target + " -> " + ap.Addr().String(), Rule: "private-address"}
		}
		if denial != nil {
			mu.Lock()
			refused = denial
			mu.Unlock()
		}
		return denial
	}}
	conn, err := d.DialContext(ctx, network, target)
	mu.Lock()
	defer mu.Unlock()
	if err != nil && refused != nil {
		return nil, refused
	}
	return conn, err
}

// Public reports whether a is a globally routable unicast address. Loopback,
// private, link-local, multicast, unspecified, shared (100.64.0.0/10),
// benchmarking, documentation and reserved ranges are not public.
func Public(a netip.Addr) bool {
	a = a.WithZone("").Unmap()
	if !a.IsValid() || a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsLinkLocalMulticast() ||
		a.IsInterfaceLocalMulticast() || a.IsMulticast() || a.IsUnspecified() {
		return false
	}
	for _, p := range nonPublic {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

var nonPublic = func() []netip.Prefix {
	var out []netip.Prefix
	for _, s := range []string{
		"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "255.255.255.255/32",
		"::/96", "fec0::/10", "64:ff9b:1::/48", "100::/64", "2001::/23", "2001:db8::/32", "fc00::/7",
	} {
		out = append(out, netip.MustParsePrefix(s))
	}
	return out
}()
