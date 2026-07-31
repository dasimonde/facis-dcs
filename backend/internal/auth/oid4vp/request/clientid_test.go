package request

import (
	"testing"
	"time"
)

// A wallet reads the prefix as part of the identifier. Without one the value
// means the "pre-registered" prefix — "you already know me out of band" — which
// a wallet with no such prior arrangement refuses before it looks at any
// credential, so login fails on the request rather than on the presentation.
func TestX509SANDNSClientIDCarriesThePrefix(t *testing.T) {
	got := X509SANDNSClientID("dcs.example.org")
	if got != "x509_san_dns:dcs.example.org" {
		t.Fatalf("client id lost its prefix: %q", got)
	}
}

// Rendering an already-prefixed value must not double it: the identifier is
// passed around as a whole, and x509_san_dns:x509_san_dns:host matches no
// certificate SAN.
func TestX509SANDNSClientIDIsIdempotent(t *testing.T) {
	once := X509SANDNSClientID("dcs.example.org")
	twice := X509SANDNSClientID(once)
	if once != twice {
		t.Fatalf("prefix was applied twice: %q", twice)
	}
}

func TestX509SANDNSClientIDRejectsAnEmptyHostname(t *testing.T) {
	if got := X509SANDNSClientID("   "); got != "" {
		t.Fatalf("expected empty client id for a blank hostname, got %q", got)
	}
}

// The request object must name the verifier by the same prefixed identifier the
// wallet was handed in the deep link, and it must be the issuer too: a wallet
// checks iss against client_id before trusting the request.
func TestBuildJWTCarriesThePrefixedClientIDAsIssuer(t *testing.T) {
	signer := &captureSigner{}
	clientID := X509SANDNSClientID("dcs.example.org")

	_, err := BuildJWT(signer, Params{
		ClientID:    clientID,
		ResponseURI: "https://dcs.example.org/api/auth/presentation/callback",
		State:       "state-1",
		Nonce:       "nonce-1",
		ExpiresAt:   time.Now().UTC().Add(time.Minute),
		DCQLQuery:   map[string]any{"credentials": []any{map[string]any{"id": "q1"}}},
	})
	if err != nil {
		t.Fatalf("BuildJWT returned error: %v", err)
	}

	if got := signer.claims["client_id"]; got != clientID {
		t.Fatalf("client_id lost its prefix in the request object: %v", got)
	}
	if got := signer.claims["iss"]; got != clientID {
		t.Fatalf("iss does not match client_id: %v", got)
	}
	// client_id_scheme was folded into the prefix; sending it alongside a
	// prefixed identifier is the older, superseded encoding.
	if _, present := signer.claims["client_id_scheme"]; present {
		t.Fatal("client_id_scheme must not accompany a prefixed client_id")
	}
}

// A deployment reached on a non-default port still identifies itself by name:
// a dNSName SAN holds no port, so carrying one produces an identifier no
// certificate can back and every wallet refuses it.
func TestX509SANDNSClientIDDropsThePort(t *testing.T) {
	got := X509SANDNSClientID("dcs-a.localhost:18080")
	if got != "x509_san_dns:dcs-a.localhost" {
		t.Fatalf("port leaked into the client id: %q", got)
	}
}

// Applying it to an already-prefixed value must still drop the port and not
// mistake the prefix's own colon for a port separator.
func TestX509SANDNSClientIDIsIdempotentWithAPort(t *testing.T) {
	once := X509SANDNSClientID("dcs-a.localhost:18080")
	if twice := X509SANDNSClientID(once); twice != once {
		t.Fatalf("re-rendering changed the client id: %q -> %q", once, twice)
	}
}
