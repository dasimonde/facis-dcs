package middleware

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// RateLimitAuthenticated enforces the per-credential request budget for API
// interactions (DCS-FR-CWE-28): every request presenting an Authorization
// header is counted against a fixed one-minute window keyed by that
// credential, and requests beyond limitPerMinute are answered 429 with a
// Retry-After. Requests without an Authorization header are not counted —
// unauthenticated routes (login, DID document, static assets, probes) are
// not "API interactions" in the CWE-28 sense, and API routes reject them via
// their own auth anyway.
func RateLimitAuthenticated(limitPerMinute int, next http.Handler) http.Handler {
	l := &rateLimiter{limit: limitPerMinute, windows: map[[16]byte]*window{}}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			next.ServeHTTP(w, r)
			return
		}
		if retryAfter, ok := l.take(auth, time.Now()); !ok {
			w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())+1))
			http.Error(w, fmt.Sprintf("rate limit exceeded: more than %d API requests per minute for this credential", l.limit), http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type window struct {
	start time.Time
	count int
}

type rateLimiter struct {
	mu      sync.Mutex
	limit   int
	windows map[[16]byte]*window
}

// take counts one request for the credential and reports whether it is within
// budget; when over budget it returns how long until the window resets. The
// credential is stored as a truncated SHA-256 so raw bearer tokens never sit
// in memory, and windows older than a minute are dropped opportunistically.
func (l *rateLimiter) take(credential string, now time.Time) (time.Duration, bool) {
	sum := sha256.Sum256([]byte(credential))
	var key [16]byte
	copy(key[:], sum[:16])

	l.mu.Lock()
	defer l.mu.Unlock()

	// Opportunistic GC: expired windows accumulate one entry per credential,
	// so sweep them whenever the map has grown noticeably.
	if len(l.windows) > 1024 {
		for k, win := range l.windows {
			if now.Sub(win.start) >= time.Minute {
				delete(l.windows, k)
			}
		}
	}

	win := l.windows[key]
	if win == nil || now.Sub(win.start) >= time.Minute {
		l.windows[key] = &window{start: now, count: 1}
		return 0, true
	}
	win.count++
	if win.count > l.limit {
		return time.Minute - now.Sub(win.start), false
	}
	return 0, true
}
