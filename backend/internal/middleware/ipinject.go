// middleware/ip.go
package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"
)

type contextKey string

const IPKey contextKey = "client_ip"

func InjectIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		ctx := context.WithValue(r.Context(), IPKey, ip)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func IPFromContext(ctx context.Context) string {
	ip, _ := ctx.Value(IPKey).(string)
	return ip
}

func extractIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if i := len(xff); i > 0 {
			for j := 0; j < i; j++ {
				if xff[j] == ',' {
					return strings.TrimSpace(xff[:j])
				}
			}
		}
		return strings.TrimSpace(xff)
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
