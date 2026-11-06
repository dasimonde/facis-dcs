package tsa

import (
	"crypto/tls"
	"net/http"
	"testing"
)

// A time-stamp token is signed by the TSA, so the client also reaches TSAs
// that offer only TLS 1.2.
func TestTSAClientAcceptsTLS12(t *testing.T) {
	c, err := NewClient("https://tsa.example/")
	if err != nil {
		t.Fatal(err)
	}
	rt, ok := c.client.Transport.(*http.Transport)
	if !ok || rt.TLSClientConfig == nil || rt.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("TSA transport %#v, want MinVersion TLS 1.2", c.client.Transport)
	}
}
