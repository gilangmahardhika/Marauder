package dbtools

import (
	"context"
	"net"
	"net/netip"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Guard refuses the board database (RFC-097). A connector to the same
// address, port, and database name as databaseUrl is refused at dial time.
// When the board is on this machine, every loopback and local interface
// address counts as the board's.
type Guard struct {
	host     string
	port     int
	database string
	// local lists this machine's addresses; nil reads them at dial time.
	local func() []netip.Addr
}

// BoardGuard reads the board's host, port, and database from databaseUrl.
func BoardGuard(databaseURL string) (*Guard, error) {
	cfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, err
	}
	return &Guard{host: cfg.Host, port: int(cfg.Port), database: cfg.Database}, nil
}

func (g *Guard) localAddrs() []netip.Addr {
	if g.local != nil {
		return g.local()
	}
	var out []netip.Addr
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if p, err := netip.ParsePrefix(a.String()); err == nil {
			out = append(out, p.Addr().Unmap())
		}
	}
	return out
}

func (g *Guard) isLocal(addr netip.Addr) bool {
	if addr.IsLoopback() || addr.IsUnspecified() {
		return true
	}
	for _, a := range g.localAddrs() {
		if a == addr {
			return true
		}
	}
	return false
}

// boardAddrs resolves the board host now. A socket path is this machine.
func (g *Guard) boardAddrs(ctx context.Context) (addrs []netip.Addr, local bool) {
	if g.host == "" || strings.HasPrefix(g.host, "/") {
		return nil, true
	}
	ips, _ := lookup(ctx, g.host)
	for _, ip := range ips {
		if g.isLocal(ip) {
			local = true
		}
	}
	return ips, local
}

// refuses reports whether addr:port with database is the board.
func (g *Guard) refuses(ctx context.Context, addr netip.Addr, port int, database string) bool {
	if g == nil || port != g.port || !strings.EqualFold(database, g.database) {
		return false
	}
	addr = addr.Unmap()
	boardIPs, local := g.boardAddrs(ctx)
	if local && g.isLocal(addr) {
		return true
	}
	for _, ip := range boardIPs {
		if ip == addr {
			return true
		}
	}
	return false
}

func lookup(ctx context.Context, host string) ([]netip.Addr, error) {
	if ip, err := netip.ParseAddr(host); err == nil {
		return []netip.Addr{ip.Unmap()}, nil
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	out := make([]netip.Addr, len(ips))
	for i, ip := range ips {
		out[i] = ip.Unmap()
	}
	return out, nil
}

// dialer resolves the address once, refuses the board, and dials the
// checked address.
type dialer struct {
	guard    *Guard
	database string
}

func (d dialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if network != "tcp" && network != "tcp4" && network != "tcp6" {
		return nil, ErrBoard
	}
	host, portText, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		return nil, err
	}
	ips, err := lookup(ctx, host)
	if err != nil {
		return nil, err
	}
	var lastErr error = ErrBoard
	nd := net.Dialer{Timeout: ConnectTimeout}
	for _, ip := range ips {
		if d.guard.refuses(ctx, ip, port, d.database) {
			return nil, ErrBoard
		}
	}
	for _, ip := range ips {
		conn, err := nd.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), portText))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}
