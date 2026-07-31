package safehttp

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestRedirectsAreRefused(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	defer final.Close()

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redirector.Close()

	_, err := Client(2*time.Second, Policy{AllowLoopback: true}).Get(redirector.URL)
	if err == nil {
		t.Fatal("followed a redirect; the responder must not choose where key material is read from")
	}
	if !strings.Contains(err.Error(), "refusing redirect") {
		t.Fatalf("failed for the wrong reason: %v", err)
	}
}

func TestLinkLocalIsRefusedEvenWhenLoopbackIsAllowed(t *testing.T) {
	// The cloud instance metadata service answers only because the request comes
	// from this host, which is exactly what makes it a target.
	_, err := Client(2*time.Second, Policy{AllowLoopback: true}).Get("http://169.254.169.254/latest/meta-data/")
	if err == nil {
		t.Fatal("dialled the link-local metadata address")
	}
	if !strings.Contains(err.Error(), "link-local") {
		t.Fatalf("failed for the wrong reason: %v", err)
	}
}

func TestLoopbackIsRefusedUnlessAllowed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("admin"))
	}))
	defer server.Close()

	if _, err := Client(2*time.Second, Policy{}).Get(server.URL); err == nil {
		t.Fatal("dialled loopback under the default policy")
	} else if !strings.Contains(err.Error(), "loopback") {
		t.Fatalf("failed for the wrong reason: %v", err)
	}

	resp, err := Client(2*time.Second, Policy{AllowLoopback: true}).Get(server.URL)
	if err != nil {
		t.Fatalf("loopback refused although the policy allows it: %v", err)
	}
	_ = resp.Body.Close()
}

func TestAllowedHostsExcludesEverythingElse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	host, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}

	client := Client(2*time.Second, Policy{AllowLoopback: true, AllowedHosts: []string{"issuer.example"}})
	if _, err := client.Get(server.URL); err == nil {
		t.Fatalf("dialled %s although the allow-list names only issuer.example", host.Hostname())
	} else if !strings.Contains(err.Error(), "allow-list") {
		t.Fatalf("failed for the wrong reason: %v", err)
	}
}
