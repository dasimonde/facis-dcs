package main

import (
	"crypto/tls"
	"net/http"
	"testing"
)

func TestOutboundClientsRequireTLS13(t *testing.T) {
	cfg := http.DefaultTransport.(*http.Transport).TLSClientConfig
	if cfg == nil || cfg.MinVersion != tls.VersionTLS13 {
		t.Fatalf("http.DefaultTransport TLS config %#v, want MinVersion TLS 1.3", cfg)
	}
}
