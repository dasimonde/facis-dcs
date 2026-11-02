package identity

import (
	"crypto/tls"
	"net/http"
	"testing"
)

// The national trusted lists stay reachable on a server that offers TLS 1.2
// only, while every other outbound client requires TLS 1.3.
func TestTrustedListClientAcceptsTLS12(t *testing.T) {
	rt, ok := tslClient.Transport.(*http.Transport)
	if !ok || rt.TLSClientConfig == nil || rt.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("trusted-list transport %#v, want MinVersion TLS 1.2", tslClient.Transport)
	}
}
