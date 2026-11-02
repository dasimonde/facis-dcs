package middleware

import (
	"net/http/httptest"
	"testing"
)

// A client cannot choose the address its attempts are counted under: the
// X-Forwarded-For entries count only when a trusted proxy sent them, and then
// only the right-most entry that no trusted proxy appended.
func TestExtractIPTakesTheRightMostUntrustedHop(t *testing.T) {
	trusted := TrustedProxies("127.0.0.0/8,10.0.0.0/8")
	for name, tc := range map[string]struct {
		remote    string
		forwarded []string
		want      string
	}{
		"client sent a spoofed entry":                  {"10.1.0.5:4711", []string{"198.51.100.7, 203.0.113.9"}, "203.0.113.9"},
		"ingress appended a second header":             {"10.1.0.5:4711", []string{"198.51.100.7", "203.0.113.9"}, "203.0.113.9"},
		"TLS proxy appended the ingress address":       {"127.0.0.1:4711", []string{"198.51.100.7, 203.0.113.9, 10.1.0.5"}, "203.0.113.9"},
		"TLS proxy caller in the cluster":              {"127.0.0.1:4711", []string{"10.2.0.9"}, "10.2.0.9"},
		"untrusted peer cannot set its own key":        {"192.0.2.1:4711", []string{"203.0.113.9"}, "192.0.2.1"},
		"untrusted peer, no header":                    {"192.0.2.1:4711", nil, "192.0.2.1"},
		"trusted peer, no header uses its own address": {"10.1.0.5:4711", nil, "10.1.0.5"},
	} {
		t.Run(name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = tc.remote
			for _, v := range tc.forwarded {
				r.Header.Add("X-Forwarded-For", v)
			}
			if got := extractIP(r, trusted); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTrustedProxiesDefaultsToTheClusterHops(t *testing.T) {
	trusted := TrustedProxies("")
	for _, addr := range []string{"127.0.0.1", "::1", "10.244.0.7", "172.17.0.2", "192.168.1.4"} {
		if !isTrusted(addr, trusted) {
			t.Errorf("%s is not trusted by default", addr)
		}
	}
	if isTrusted("203.0.113.9", trusted) {
		t.Error("a public address is trusted by default")
	}
}
