package middleware

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
)

type contextKey string

const IPKey contextKey = "client_ip"

// defaultTrustedProxies are the addresses of the hops inside the deployment:
// the TLS proxy in the pod (loopback) and the ingress controller (a private
// cluster address).
const defaultTrustedProxies = "127.0.0.0/8,::1/128,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,fc00::/7"

// InjectIP puts the client address into the request context. The trusted
// proxies come from DCS_TRUSTED_PROXIES (comma-separated CIDRs).
func InjectIP(next http.Handler) http.Handler {
	trusted := TrustedProxies(os.Getenv("DCS_TRUSTED_PROXIES"))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := context.WithValue(r.Context(), IPKey, extractIP(r, trusted))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func IPFromContext(ctx context.Context) string {
	ip, _ := ctx.Value(IPKey).(string)
	return ip
}

// TrustedProxies parses a comma-separated CIDR list. An empty list gives the
// default. An entry that does not parse is logged and skipped.
func TrustedProxies(list string) []netip.Prefix {
	if strings.TrimSpace(list) == "" {
		list = defaultTrustedProxies
	}
	var out []netip.Prefix
	for _, entry := range strings.Split(list, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		p, err := netip.ParsePrefix(entry)
		if err != nil {
			log.Printf("DCS_TRUSTED_PROXIES: ignore %q: %v", entry, err) // #nosec G706 -- operator configuration, quoted
			continue
		}
		out = append(out, p.Masked())
	}
	return out
}

func isTrusted(addr string, trusted []netip.Prefix) bool {
	a, err := netip.ParseAddr(strings.TrimSpace(addr))
	if err != nil {
		return false
	}
	a = a.Unmap()
	for _, p := range trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// extractIP returns the client address that the lockout is keyed on
// (FR-UC-01-4). X-Forwarded-For is read only when the direct peer is a
// trusted proxy. Then the result is the right-most entry that is not a
// trusted proxy: every entry to its right was appended by a trusted hop, and
// entries to its left are sent by the client and can name any address.
func extractIP(r *http.Request, trusted []netip.Prefix) string {
	peer, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		peer = r.RemoteAddr
	}
	if !isTrusted(peer, trusted) {
		return peer
	}
	var hops []string
	for _, v := range r.Header.Values("X-Forwarded-For") {
		for _, hop := range strings.Split(v, ",") {
			if hop = strings.TrimSpace(hop); hop != "" {
				hops = append(hops, hop)
			}
		}
	}
	for i := len(hops) - 1; i >= 0; i-- {
		if !isTrusted(hops[i], trusted) {
			return hops[i]
		}
	}
	if len(hops) > 0 {
		return hops[0]
	}
	return peer
}
