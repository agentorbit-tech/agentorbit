package config

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

var rfc6598CGNAT = func() *net.IPNet {
	_, n, _ := net.ParseCIDR("100.64.0.0/10")
	return n
}()

// IsPrivate reports whether ip is loopback, RFC1918, RFC6598 carrier-grade
// NAT, link-local (IPv4 169.254/16), unique-local (fc00::/7), or
// IPv6 link-local (fe80::/10). All of those are acceptable targets for
// HTTP-internal-API traffic between the proxy and processing services.
func IsPrivate(ip net.IP) bool {
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	if ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if ip.IsPrivate() {
		// Go stdlib covers RFC1918 (10/8, 172.16/12, 192.168/16) and
		// RFC4193 (fc00::/7).
		return true
	}
	if rfc6598CGNAT != nil && rfc6598CGNAT.Contains(ip) {
		return true
	}
	return false
}

// LookupFunc abstracts net.Resolver.LookupIPAddr so tests can inject a stub.
// The 2-second internal timeout is applied by the caller.
type LookupFunc func(ctx context.Context, host string) ([]net.IPAddr, error)

// defaultResolver wraps net.DefaultResolver.LookupIPAddr.
func defaultResolver(ctx context.Context, host string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, host)
}

// ValidateInternalURL enforces the operational rule for PROCESSING_URL:
//   - https is always accepted
//   - http is accepted only when allowPlaintext is true AND every IP the
//     hostname resolves to is private/loopback. A public IP fails closed
//     even when the override flag is set.
//
// If lookup is nil, defaultResolver is used.
func ValidateInternalURL(rawURL string, allowPlaintext bool, lookup LookupFunc) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("PROCESSING_URL parse: %w", err)
	}
	switch u.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("PROCESSING_URL scheme %q is not http/https", u.Scheme)
	}
	if u.Scheme == "https" {
		return nil
	}
	if !allowPlaintext {
		return fmt.Errorf("PROCESSING_URL uses plain HTTP — provider API keys would transit in cleartext. Set ALLOW_PLAINTEXT_INTERNAL=true to override (e.g. Docker networking)")
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("PROCESSING_URL has no host")
	}

	var ips []net.IP
	if literal := net.ParseIP(host); literal != nil {
		ips = []net.IP{literal}
	} else {
		if lookup == nil {
			lookup = defaultResolver
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		addrs, err := lookup(ctx, host)
		if err != nil {
			return fmt.Errorf("PROCESSING_URL host %q DNS lookup failed: %w", host, err)
		}
		if len(addrs) == 0 {
			return fmt.Errorf("PROCESSING_URL host %q resolved to no IPs", host)
		}
		for _, a := range addrs {
			ips = append(ips, a.IP)
		}
	}

	var public []string
	for _, ip := range ips {
		if !IsPrivate(ip) {
			public = append(public, ip.String())
		}
	}
	if len(public) > 0 {
		return fmt.Errorf("PROCESSING_URL host %q resolves to public IP(s) %s — ALLOW_PLAINTEXT_INTERNAL is restricted to loopback / RFC1918 / link-local / RFC6598 hosts", host, strings.Join(public, ","))
	}
	return nil
}
