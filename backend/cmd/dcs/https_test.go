package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// DCS-IR-CI-03: behind the TLS-terminating proxy, plain HTTP is redirected to
// the public HTTPS origin and HTTPS responses carry HSTS.
func TestHTTPSMiddleware(t *testing.T) {
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	cases := []struct {
		name, base, proto, method string
		wantCode                  int
		wantLocation, wantHSTS    string
	}{
		{"plain HTTP is redirected", "https://dcs.example.org/digital-contracting-service", "http", http.MethodGet,
			http.StatusPermanentRedirect, "https://dcs.example.org/digital-contracting-service/ui/?a=1", ""},
		{"POST keeps its method", "https://dcs.example.org", "http", http.MethodPost,
			http.StatusPermanentRedirect, "https://dcs.example.org/digital-contracting-service/ui/?a=1", ""},
		{"HTTPS gets HSTS", "https://dcs.example.org", "https", http.MethodGet, http.StatusOK, "", hstsHeader},
		{"proxy list, last hop counts", "https://dcs.example.org", "http, https", http.MethodGet, http.StatusOK, "", hstsHeader},
		{"client-sent https before the proxy hop is ignored", "https://dcs.example.org", "https, http", http.MethodGet,
			http.StatusPermanentRedirect, "https://dcs.example.org/digital-contracting-service/ui/?a=1", ""},
		{"no proxy header passes", "https://dcs.example.org", "", http.MethodGet, http.StatusOK, "", ""},
		{"http public URL (BDD) passes", "http://localhost:18080/x", "http", http.MethodGet, http.StatusOK, "", ""},
		{"unset public URL passes", "", "http", http.MethodGet, http.StatusOK, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := httptest.NewRequest(c.method, "http://evil.example/digital-contracting-service/ui/?a=1", nil)
			if c.proto != "" {
				r.Header.Set("X-Forwarded-Proto", c.proto)
			}
			w := httptest.NewRecorder()
			httpsMiddleware(c.base, ok).ServeHTTP(w, r)
			if w.Code != c.wantCode {
				t.Fatalf("status %d, want %d", w.Code, c.wantCode)
			}
			if got := w.Header().Get("Location"); got != c.wantLocation {
				t.Fatalf("Location %q, want %q", got, c.wantLocation)
			}
			if got := w.Header().Get("Strict-Transport-Security"); got != c.wantHSTS {
				t.Fatalf("HSTS %q, want %q", got, c.wantHSTS)
			}
		})
	}
}
