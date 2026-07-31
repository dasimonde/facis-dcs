package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/base/jades"
)

// jadesTestDIDDocument builds a DIDDocument backed by a fresh P-256 key with a
// self-signed x5c leaf, via the same NewDIDDocument path production uses.
func jadesTestDIDDocument(t *testing.T, host string) *identity.DIDDocument {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	// assertionMethod names the signing key by a relative DID URL, and an
	// embedded method sits beside it: both are shapes DID Core permits and this
	// deployment's own gendid does not emit.
	didJSON := map[string]any{
		"id":              "did:web:" + host,
		"assertionMethod": []any{"#key-1"},
		"verificationMethod": []map[string]any{
			{
				"id": "did:web:" + host + "#key-1",
				"publicKeyJwk": map[string]any{
					"kty": "EC",
					"crv": "P-256",
					"x":   base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))),
					"y":   base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
					"x5c": []string{base64.StdEncoding.EncodeToString(certDER)},
				},
			},
		},
	}
	raw, err := json.Marshal(didJSON)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "did.json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	doc, err := identity.NewDIDDocument(path, key)
	if err != nil {
		t.Fatalf("NewDIDDocument: %v", err)
	}
	return doc
}

func signShippedContract(t *testing.T, doc *identity.DIDDocument, iri string, version int, pdfPayload []byte) string {
	t.Helper()
	payload, err := jades.BuildContractPayload(iri, version, pdfPayload)
	if err != nil {
		t.Fatal(err)
	}
	jws, err := jades.Sign(doc, payload)
	if err != nil {
		t.Fatal(err)
	}
	return jws
}

func TestVerifyShippedJadesAcceptsMatchingShip(t *testing.T) {
	sender := jadesTestDIDDocument(t, "dcs-a.localhost")
	pdfPayload := []byte(`{"dcs:metadata":{"dcs:title":"Peer Contract"},"b":1,"a":{"nested":true}}`)
	iri := "did:web:dcs-a.localhost:contract:42"
	jws := signShippedContract(t, sender, iri, 7, pdfPayload)

	sig, err := verifyShippedJades(jws, iri, "did:web:dcs-a.localhost", pdfPayload, sender)
	if err != nil {
		t.Fatalf("expected the matching ship to verify, got: %v", err)
	}
	if sig.DID != iri || sig.ContractVersion != 7 || sig.FromPeerDID != "did:web:dcs-a.localhost" || sig.JadesSignature != jws {
		t.Fatalf("provenance artifact fields wrong: %+v", sig)
	}
}

func TestVerifyShippedJadesAcceptsReorderedEquivalentPayload(t *testing.T) {
	// JCS canonicalization: key order and whitespace in the PDF-embedded
	// payload must not matter, only structural JSON equality.
	sender := jadesTestDIDDocument(t, "dcs-a.localhost")
	iri := "did:web:dcs-a.localhost:contract:42"
	jws := signShippedContract(t, sender, iri, 1, []byte(`{"b":1,"a":{"nested":true}}`))

	if _, err := verifyShippedJades(jws, iri, "did:web:dcs-a.localhost",
		[]byte(`  {"a": {"nested": true}, "b": 1}`), sender); err != nil {
		t.Fatalf("expected the structurally equal payload to verify, got: %v", err)
	}
}

func TestVerifyShippedJadesRejectsForeignKey(t *testing.T) {
	sender := jadesTestDIDDocument(t, "dcs-a.localhost")
	imposter := jadesTestDIDDocument(t, "dcs-evil.localhost")
	pdfPayload := []byte(`{"x":1}`)
	iri := "did:web:dcs-a.localhost:contract:42"
	jws := signShippedContract(t, imposter, iri, 1, pdfPayload)

	_, err := verifyShippedJades(jws, iri, "did:web:dcs-a.localhost", pdfPayload, sender)
	if err == nil || !strings.Contains(err.Error(), "not published by peer") {
		t.Fatalf("expected the foreign-key ship to be rejected with a key mismatch, got: %v", err)
	}
}

// A key the peer publishes, but for key agreement only, may not sign a contract.
func TestVerifyShippedJadesRejectsKeyNotPublishedForAssertions(t *testing.T) {
	sender := jadesTestDIDDocument(t, "dcs-a.localhost")
	pdfPayload := []byte(`{"x":1}`)
	iri := "did:web:dcs-a.localhost:contract:42"
	jws := signShippedContract(t, sender, iri, 1, pdfPayload)

	// Same document, same key — published for key agreement instead of assertions.
	demoted := *sender
	demoted.KeyAgreement = demoted.AssertionMethod
	demoted.AssertionMethod = nil

	_, err := verifyShippedJades(jws, iri, "did:web:dcs-a.localhost", pdfPayload, &demoted)
	if err == nil || !strings.Contains(err.Error(), "not published by peer") {
		t.Fatalf("expected a key published only for key agreement to be refused, got: %v", err)
	}
}

func TestVerifyShippedJadesRejectsWrongContract(t *testing.T) {
	sender := jadesTestDIDDocument(t, "dcs-a.localhost")
	pdfPayload := []byte(`{"x":1}`)
	jws := signShippedContract(t, sender, "did:web:dcs-a.localhost:contract:OTHER", 1, pdfPayload)

	_, err := verifyShippedJades(jws, "did:web:dcs-a.localhost:contract:42", "did:web:dcs-a.localhost", pdfPayload, sender)
	if err == nil || !strings.Contains(err.Error(), "binds contract") {
		t.Fatalf("expected the wrong-contract ship to be rejected, got: %v", err)
	}
}

func TestVerifyShippedJadesRejectsDivergentDocument(t *testing.T) {
	// The JAdES covers one document, the shipped PDF embeds another: the
	// signature must not be accepted as provenance for the shipped content.
	sender := jadesTestDIDDocument(t, "dcs-a.localhost")
	iri := "did:web:dcs-a.localhost:contract:42"
	jws := signShippedContract(t, sender, iri, 1, []byte(`{"clause":"the signed terms"}`))

	_, err := verifyShippedJades(jws, iri, "did:web:dcs-a.localhost", []byte(`{"clause":"tampered terms"}`), sender)
	if err == nil || !strings.Contains(err.Error(), "does not match the contract document") {
		t.Fatalf("expected the divergent-document ship to be rejected, got: %v", err)
	}
}
