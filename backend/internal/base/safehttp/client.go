// Package safehttp builds HTTP clients for fetches whose URL is derived from
// data a caller supplies — a did:web identifier off an unauthenticated wallet
// callback, an ORCE resolver path. Such a fetch is a server-side request
// primitive: without constraints it reads whatever the service itself can
// reach, and its result is then trusted as key material.
package safehttp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"
)

// Policy bounds where a client may connect.
//
// Private ranges stay reachable by default because that is where these
// deployments genuinely live: an in-cluster ORCE resolver and a peer DCS are
// Service addresses. Blocking them wholesale would break the normal case while
// an attacker who can name a public host is unaffected. AllowedHosts is the
// control for a deployment that wants the tighter bound, and the primary defence
// remains that only issuers named in the trust configuration are ever resolved.
type Policy struct {
	// AllowedHosts, when non-empty, is the exhaustive set of hostnames that may
	// be dialled. Compared case-insensitively against the URL host, port excluded.
	AllowedHosts []string
	// AllowLoopback permits 127.0.0.0/8 and ::1, which dev and CI stacks need and
	// a real deployment never does — loopback there is the service's own
	// admin surface.
	AllowLoopback bool
}

// Client returns an HTTP client that refuses redirects and applies p to every
// address it dials.
func Client(timeout time.Duration, p Policy) *http.Client {
	allowed := make(map[string]bool, len(p.AllowedHosts))
	for _, h := range p.AllowedHosts {
		if h = strings.ToLower(strings.TrimSpace(h)); h != "" {
			allowed[h] = true
		}
	}

	// No per-dial timeout: it would equal the whole client budget, so the first
	// address that blackholes would consume it and the remaining ones below
	// would never be tried. The context carries the deadline, and an address
	// that refuses outright fails immediately, which is the case failover is
	// for.
	dialer := &net.Dialer{}
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, fmt.Errorf("dial %s: %w", addr, err)
			}
			if len(allowed) > 0 && !allowed[strings.ToLower(host)] {
				return nil, fmt.Errorf("dial %s: host is not in the resolver allow-list", host)
			}
			// The connection is made to the address that was checked, not to the
			// name. Checking a name and then handing the name to the dialer
			// leaves it to resolve a second time, and a record with a short TTL
			// can answer differently on that second lookup — the check passes on
			// a public address and the connection lands on a private one.
			//
			// Dialling an IP does not weaken TLS: http.Transport takes the
			// ServerName for the handshake from the request URL, not from the
			// address this returns.
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolve %s: %w", host, err)
			}
			if len(ips) == 0 {
				return nil, fmt.Errorf("resolve %s: no addresses", host)
			}
			for _, ip := range ips {
				if err := permitted(ip.IP, p.AllowLoopback); err != nil {
					return nil, fmt.Errorf("dial %s: %w", host, err)
				}
			}

			var lastErr error
			for _, ip := range ips {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, lastErr
		},
	}

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		// A DID document and a JWKS are served, not redirected to. Following a
		// redirect would let the responder pick the next address after the first
		// was checked, and would silently undo the https-only rule by sending the
		// second hop to http.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return fmt.Errorf("refusing redirect to %s: key material is fetched from the address the identifier names, not one it is pointed at", req.URL.Redacted())
		},
	}
}

// permitted rejects addresses that no legitimate issuer or peer is published on
// and that a request forgery aims at.
func permitted(ip net.IP, allowLoopback bool) error {
	// Go's own predicates read the IPv4-mapped form (::ffff:127.0.0.1) but not
	// the deprecated IPv4-compatible one (::127.0.0.1), which would otherwise
	// reach this check as an ordinary IPv6 address and pass every test below.
	if v4 := ipv4Compatible(ip); v4 != nil {
		ip = v4
	}

	switch {
	case ip.IsUnspecified():
		return fmt.Errorf("address %s is unspecified", ip)
	case ip.IsLoopback():
		if allowLoopback {
			return nil
		}
		return fmt.Errorf("address %s is loopback", ip)
	// 0.0.0.0/8 is "this network": Linux routes the whole block locally, so
	// 0.0.0.1 reaches this host while IsUnspecified only catches 0.0.0.0.
	case ip.To4() != nil && ip.To4()[0] == 0:
		return fmt.Errorf("address %s is in the local 0.0.0.0/8 block", ip)
	case ip.Equal(net.IPv4bcast):
		return fmt.Errorf("address %s is broadcast", ip)
	case ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast():
		// 169.254.169.254 and fe80:: neighbours: the cloud instance metadata
		// service and anything else that answers only because the request comes
		// from this host.
		return fmt.Errorf("address %s is link-local", ip)
	case ip.IsMulticast():
		return fmt.Errorf("address %s is multicast", ip)
	case ip.IsInterfaceLocalMulticast():
		return fmt.Errorf("address %s is interface-local", ip)
	}
	return nil
}

// ipv4Compatible returns the embedded IPv4 address of an ::a.b.c.d address, or
// nil. The all-zero prefix would otherwise make ::127.0.0.1 look like an
// ordinary global IPv6 address to every predicate in permitted.
func ipv4Compatible(ip net.IP) net.IP {
	if len(ip) != net.IPv6len || ip.To4() != nil {
		return nil
	}
	for _, b := range ip[:12] {
		if b != 0 {
			return nil
		}
	}
	// ::0.0.0.x below 256 is the unspecified address and ::1, which the
	// IsUnspecified and IsLoopback cases already name correctly.
	if ip[12] == 0 && ip[13] == 0 && ip[14] == 0 {
		return nil
	}
	return net.IPv4(ip[12], ip[13], ip[14], ip[15])
}
