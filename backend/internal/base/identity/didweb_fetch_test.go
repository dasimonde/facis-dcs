package identity

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const peerDIDDocument = `{
  "@context": ["https://www.w3.org/ns/did/v1"],
  "id": "did:web:peer.example",
  "verificationMethod": [{
    "id": "did:web:peer.example#key-1",
    "controller": "did:web:peer.example",
    "type": "JsonWebKey2020",
    "publicKeyJwk": {"kty":"EC","crv":"P-256","kid":"key-1",
      "x":"VlBNhqQn6gLyQXqKkLDHBwXlJsi0IES4OovRv9FrAHI",
      "y":"vZMT1rkIeVaj7Om-FuIIcMHA1-xHtSk3OTGgovfeHCk"}
  }],
  "assertionMethod": ["did:web:peer.example#key-1"]
}`

func didForServer(t *testing.T, server *httptest.Server) string {
	t.Helper()
	authority := strings.TrimPrefix(server.URL, "http://")
	return "did:web:" + strings.Replace(authority, ":", "%3A", 1)
}

// A DID document is served at the address its identifier names, not redirected
// to from there. Following redirects let the responder choose the second
// address after the first had been vetted, and an https -> http hop undid the
// https-only rule DIDWebSchemes exists to enforce without anything logging that
// it had happened.
func TestDIDDocumentFetchRefusesRedirects(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(peerDIDDocument))
	}))
	defer target.Close()

	// The control: the same document, fetched directly, does resolve — so the
	// refusal below is the redirect and not the harness.
	if _, err := FetchDIDDocument(didForServer(t, target)); err != nil {
		t.Fatalf("a directly served did.json must resolve: %v", err)
	}

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL+"/.well-known/did.json", http.StatusFound)
	}))
	defer redirector.Close()

	_, err := FetchDIDDocument(didForServer(t, redirector))
	if err == nil {
		t.Fatal("a redirected did.json was accepted; the responder must not choose where a peer's key material is read from")
	}
	if !strings.Contains(err.Error(), "refusing redirect") {
		t.Fatalf("resolution failed for the wrong reason: %v", err)
	}
}
