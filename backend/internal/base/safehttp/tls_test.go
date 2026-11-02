package safehttp

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// tls12Server accepts TLS 1.2 and nothing newer.
func tls12Server(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	srv.TLS = &tls.Config{MaxVersion: tls.VersionTLS12}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// trusting returns rt with the test server's certificate as the only root.
func trusting(rt *http.Transport, srv *httptest.Server) *http.Client {
	if rt.TLSClientConfig == nil {
		rt.TLSClientConfig = &tls.Config{}
	}
	rt.TLSClientConfig.RootCAs = srv.Client().Transport.(*http.Transport).TLSClientConfig.RootCAs
	return &http.Client{Transport: rt, Timeout: 5 * time.Second}
}

// requireTLS13 calls RequireTLS13 and restores http.DefaultTransport after t.
func requireTLS13(t *testing.T) {
	saved := http.DefaultTransport.(*http.Transport).TLSClientConfig
	t.Cleanup(func() { http.DefaultTransport.(*http.Transport).TLSClientConfig = saved })
	RequireTLS13()
}

func TestOutboundClientsRefuseTLS12(t *testing.T) {
	srv := tls12Server(t)
	requireTLS13(t)

	def := http.DefaultTransport.(*http.Transport).Clone()
	safe := Client(5*time.Second, Policy{AllowLoopback: true}).Transport.(*http.Transport).Clone()
	for name, rt := range map[string]*http.Transport{"default transport": def, "safehttp client": safe} {
		_, err := trusting(rt, srv).Get(srv.URL)
		if err == nil || !strings.Contains(err.Error(), "protocol version") {
			t.Errorf("%s: TLS 1.2 server, err = %v, want a protocol version failure", name, err)
		}
	}
}

func TestTLS12TransportReachesATLS12Server(t *testing.T) {
	srv := tls12Server(t)
	requireTLS13(t)
	resp, err := trusting(TLS12Transport(), srv).Get(srv.URL)
	if err != nil {
		t.Fatalf("TLS 1.2 server: %v", err)
	}
	_ = resp.Body.Close()
}
