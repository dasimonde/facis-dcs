package request

import (
	"testing"
	"time"
)

// TestBuildDocumentRetrievalJWTMatchesEUDIShape locks the request object to the
// EUDI walletdriven-signer / eudi-lib-jvm-rqes-csc-kt wire contract, so a real
// EUDI wallet can consume what the DCS publishes.
func TestBuildDocumentRetrievalJWTMatchesEUDIShape(t *testing.T) {
	signer := &captureSigner{}
	clientID := X509SANDNSClientID("dcs.example.org")
	_, err := BuildDocumentRetrievalJWT(signer, DocRetrievalParams{
		ClientID:           clientID,
		ResponseURI:        "https://rp.example/cb",
		Nonce:              "nonce-1",
		ExpiresAt:          time.Now().UTC().Add(time.Minute),
		SignatureQualifier: "eu_eidas_aes",
		DocumentDigests:    []DocumentDigest{{Hash: "abc==", Label: "SignerOne"}},
		DocumentLocations:  []DocumentLocation{{URI: "https://rp.example/doc", Method: DocumentLocationMethod{Type: "public"}}},
	})
	if err != nil {
		t.Fatalf("BuildDocumentRetrievalJWT returned error: %v", err)
	}

	c := signer.claims
	if c["response_type"] != "sign_response" {
		t.Fatalf("response_type must be sign_response, got %v", c["response_type"])
	}
	// OpenID4VP 1.0 / HAIP encoding: the scheme is a prefix on the identifier,
	// and a bare client_id alongside a client_id_scheme claim is the superseded
	// pre-1.0 draft encoding an ARF wallet may reject.
	if c["client_id"] != clientID {
		t.Fatalf("client_id must carry its x509_san_dns prefix, got %v", c["client_id"])
	}
	if _, present := c["client_id_scheme"]; present {
		t.Fatal("client_id_scheme must not accompany a prefixed client_id")
	}
	if c["iss"] != clientID {
		t.Fatalf("iss must name the verifier that signed the request: %v", c["iss"])
	}
	if c["response_mode"] != "direct_post" {
		t.Fatalf("response_mode must be direct_post, got %v", c["response_mode"])
	}
	if c["signatureQualifier"] != "eu_eidas_aes" {
		t.Fatalf("signatureQualifier mismatch: %v", c["signatureQualifier"])
	}
	if c["hashAlgorithmOID"] != SHA256OID {
		t.Fatalf("hashAlgorithmOID mismatch: %v", c["hashAlgorithmOID"])
	}

	digests, ok := c["documentDigests"].([]any)
	if !ok || len(digests) != 1 {
		t.Fatalf("documentDigests missing or wrong shape: %T %v", c["documentDigests"], c["documentDigests"])
	}
	digest := digests[0].(map[string]any)
	if digest["hash"] != "abc==" || digest["label"] != "SignerOne" {
		t.Fatalf("document digest members mismatch: %v", digest)
	}

	locations, ok := c["documentLocations"].([]any)
	if !ok || len(locations) != 1 {
		t.Fatalf("documentLocations missing or wrong shape: %T %v", c["documentLocations"], c["documentLocations"])
	}
	location := locations[0].(map[string]any)
	if location["uri"] != "https://rp.example/doc" {
		t.Fatalf("document location uri mismatch: %v", location)
	}
	method, ok := location["method"].(map[string]any)
	if !ok || method["type"] != "public" {
		t.Fatalf("document location method mismatch: %v", location["method"])
	}
}

// TestBuildDocumentRetrievalJWTEchoesWalletNonce covers the replay protection a
// wallet gets for POSTing the request_uri (the method the ceremony's deep link
// asks for): its own nonce comes back inside the signed request object, or the
// wallet cannot tell this response from a replayed one.
func TestBuildDocumentRetrievalJWTEchoesWalletNonce(t *testing.T) {
	params := DocRetrievalParams{
		ClientID:           X509SANDNSClientID("dcs.example.org"),
		ResponseURI:        "https://rp.example/cb",
		Nonce:              "nonce-1",
		ExpiresAt:          time.Now().UTC().Add(time.Minute),
		SignatureQualifier: "eu_eidas_aes",
		DocumentDigests:    []DocumentDigest{{Hash: "abc==", Label: "SignerOne"}},
		DocumentLocations:  []DocumentLocation{{URI: "https://rp.example/doc", Method: DocumentLocationMethod{Type: "public"}}},
		WalletNonce:        "wallet-1",
	}

	signer := &captureSigner{}
	if _, err := BuildDocumentRetrievalJWT(signer, params); err != nil {
		t.Fatalf("BuildDocumentRetrievalJWT returned error: %v", err)
	}
	if got := signer.claims["wallet_nonce"]; got != "wallet-1" {
		t.Fatalf("wallet_nonce echo mismatch: got %v", got)
	}

	// A GET fetch sends no wallet nonce, and an empty claim would be a value the
	// wallet never chose.
	params.WalletNonce = ""
	getSigner := &captureSigner{}
	if _, err := BuildDocumentRetrievalJWT(getSigner, params); err != nil {
		t.Fatalf("BuildDocumentRetrievalJWT returned error: %v", err)
	}
	if _, present := getSigner.claims["wallet_nonce"]; present {
		t.Fatalf("wallet_nonce must be absent when the wallet sent none: %v", getSigner.claims["wallet_nonce"])
	}
}

func TestBuildDocumentRetrievalJWTRejectsMismatchedDigestsAndLocations(t *testing.T) {
	signer := &captureSigner{}
	_, err := BuildDocumentRetrievalJWT(signer, DocRetrievalParams{
		ClientID:           X509SANDNSClientID("dcs.example.org"),
		ResponseURI:        "https://rp.example/cb",
		Nonce:              "nonce-1",
		ExpiresAt:          time.Now().UTC().Add(time.Minute),
		SignatureQualifier: "eu_eidas_aes",
		DocumentDigests:    []DocumentDigest{{Hash: "abc==", Label: "SignerOne"}},
		DocumentLocations:  nil,
	})
	if err == nil {
		t.Fatal("expected error when documentLocations is empty")
	}
}
