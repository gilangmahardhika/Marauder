// Package netguard is the only HTTP client that reaches a URL a tribe admin
// typed (RFC-093). It refuses internal addresses, proxies, and redirects.
package netguard

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

var (
	ErrURL      = errors.New("url must be https, with no user info and no query string")
	ErrAddress  = errors.New("host resolves to an internal address")
	ErrRedirect = errors.New("redirects are not followed")
)

const (
	DialTimeout   = 5 * time.Second
	HeaderTimeout = 15 * time.Second
)

// Policy loosens the guard for tests and the fixture only: AllowLoopbackHTTP
// accepts http://127.0.0.1 and dials loopback.
type Policy struct {
	AllowLoopbackHTTP bool
}

// Resolver looks up a host's addresses. net.DefaultResolver satisfies it.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("fd00:ec2::254/128"),
}

// Blocked reports whether addr is loopback, private, link-local, CGNAT,
// unspecified, multicast, or an IPv4 address carried inside IPv6.
func Blocked(addr netip.Addr) bool {
	if !addr.IsValid() {
		return true
	}
	if addr.Is4In6() {
		return true
	}
	addr = addr.Unmap()
	if addr.IsLoopback() || addr.IsPrivate() || addr.IsLinkLocalUnicast() || addr.IsLinkLocalMulticast() ||
		addr.IsInterfaceLocalMulticast() || addr.IsMulticast() || addr.IsUnspecified() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(addr) {
			return true
		}
	}
	return false
}

// CheckURL parses raw and refuses anything but an https URL with no user
// info, query, or fragment. An IP literal host must not be Blocked.
func CheckURL(raw string, p Policy) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.Opaque != "" {
		return nil, ErrURL
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, ErrURL
	}
	host := u.Hostname()
	switch u.Scheme {
	case "https":
	case "http":
		if !p.AllowLoopbackHTTP || host != "127.0.0.1" {
			return nil, ErrURL
		}
	default:
		return nil, ErrURL
	}
	if addr, err := netip.ParseAddr(host); err == nil && Blocked(addr) && !loopbackOK(addr, p) {
		return nil, ErrAddress
	}
	return u, nil
}

func loopbackOK(addr netip.Addr, p Policy) bool {
	return p.AllowLoopbackHTTP && addr.Unmap().IsLoopback()
}

// Client returns an http.Client that resolves each host once, refuses
// internal addresses, dials the address it checked, checks it again at
// connect, ignores proxy variables, and follows no redirect. A nil resolver
// is net.DefaultResolver.
func Client(p Policy, resolver Resolver) *http.Client {
	if resolver == nil {
		resolver = net.DefaultResolver
	}
	dialer := &net.Dialer{
		Timeout: DialTimeout,
		Control: func(_, address string, _ syscall.RawConn) error {
			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return ErrAddress
			}
			if Blocked(ap.Addr()) && !loopbackOK(ap.Addr(), p) {
				return ErrAddress
			}
			return nil
		},
	}
	transport := &http.Transport{
		Proxy: nil,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, ErrAddress
			}
			addrs, err := lookup(ctx, resolver, host)
			if err != nil {
				return nil, err
			}
			for _, addr := range addrs {
				if Blocked(addr) && !loopbackOK(addr, p) {
					return nil, ErrAddress
				}
			}
			var last error
			for _, addr := range addrs {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.Unmap().String(), port))
				if err == nil {
					return conn, nil
				}
				last = err
			}
			return nil, last
		},
		TLSHandshakeTimeout:   DialTimeout,
		ResponseHeaderTimeout: HeaderTimeout,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return ErrRedirect
		},
	}
}

func lookup(ctx context.Context, resolver Resolver, host string) ([]netip.Addr, error) {
	if addr, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{addr}, nil
	}
	addrs, err := resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	if len(addrs) == 0 {
		return nil, ErrAddress
	}
	// The resolver may return an IPv4 answer in its IPv6-mapped form.
	for i, addr := range addrs {
		addrs[i] = addr.Unmap()
	}
	return addrs, nil
}
