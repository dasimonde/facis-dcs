package jades

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
)

// testDIDDocument builds a DIDDocument backed by a fresh P-256 key with a
// self-signed x5c leaf, via the same NewDIDDocument path production uses.
func testDIDDocument(t *testing.T) *identity.DIDDocument {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "jades-test.localhost"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}

	didJSON := map[string]any{
		"id":              "did:web:jades-test.localhost",
		"assertionMethod": []any{"#key-1"},
		"verificationMethod": []map[string]any{
			{
				"id": "did:web:jades-test.localhost#key-1",
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

func TestJAdESSignVerifyRoundTrip(t *testing.T) {
	doc := testDIDDocument(t)

	payload, err := BuildContractPayload("did:web:example#contract-1", 3, []byte(`{"dcs:name":"Test Contract","b":1,"a":{"z":true,"y":"ä<&"}}`))
	if err != nil {
		t.Fatalf("BuildContractPayload: %v", err)
	}

	jws, err := Sign(doc, payload)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if strings.Count(jws, ".") != 2 {
		t.Fatalf("expected a compact JWS with three segments, got %q", jws)
	}

	got, leaf, err := Verify(jws)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if string(got) != string(payload) {
		t.Fatalf("payload mismatch after verification:\n got %s\nwant %s", got, payload)
	}
	pub := doc.PublicKey()
	if leaf.X.Cmp(pub.X) != 0 || leaf.Y.Cmp(pub.Y) != 0 {
		t.Fatal("expected the x5c leaf key to equal the DID document key")
	}

	// The protected header must carry the JAdES B-B essentials.
	headerBytes, err := base64.RawURLEncoding.DecodeString(strings.SplitN(jws, ".", 2)[0])
	if err != nil {
		t.Fatal(err)
	}
	var header map[string]any
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		t.Fatal(err)
	}
	if header["alg"] != "ES256" {
		t.Fatalf("expected alg ES256, got %v", header["alg"])
	}
	if header["sigT"] == nil || header["sigT"] == "" {
		t.Fatal("expected a sigT claimed-signing-time header")
	}
	crit, _ := header["crit"].([]any)
	if len(crit) != 1 || crit[0] != "sigT" {
		t.Fatalf("expected crit to list exactly sigT, got %v", header["crit"])
	}
	if _, ok := header["x5c"].([]any); !ok {
		t.Fatal("expected an x5c certificate chain in the protected header")
	}
}

func TestJAdESVerifyRejectsTamperedPayload(t *testing.T) {
	doc := testDIDDocument(t)
	payload, err := BuildContractPayload("did:web:example#contract-1", 1, []byte(`{"dcs:name":"Original"}`))
	if err != nil {
		t.Fatal(err)
	}
	jws, err := Sign(doc, payload)
	if err != nil {
		t.Fatal(err)
	}

	tampered, err := BuildContractPayload("did:web:example#contract-1", 1, []byte(`{"dcs:name":"Tampered"}`))
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(jws, ".")
	parts[1] = base64.RawURLEncoding.EncodeToString(tampered)
	if _, _, err := Verify(strings.Join(parts, ".")); err == nil {
		t.Fatal("expected verification of a tampered payload to fail")
	}
}

func TestJAdESVerifyRejectsMissingSigT(t *testing.T) {
	doc := testDIDDocument(t)
	payload := []byte(`{"x":1}`)
	jws, err := Sign(doc, payload)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(jws, ".")
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatal(err)
	}
	var header map[string]any
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		t.Fatal(err)
	}
	delete(header, "sigT")
	mutated, err := json.Marshal(header)
	if err != nil {
		t.Fatal(err)
	}
	parts[0] = base64.RawURLEncoding.EncodeToString(mutated)
	// Signature is now broken too, but the header check must reject first
	// with a specific error naming sigT.
	_, _, err = Verify(strings.Join(parts, "."))
	if err == nil || !strings.Contains(err.Error(), "sigT") {
		t.Fatalf("expected a sigT rejection, got: %v", err)
	}
}

func TestBuildContractPayloadIsCanonical(t *testing.T) {
	a, err := BuildContractPayload("did:x", 1, []byte(`{"b":2,"a":1}`))
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildContractPayload("did:x", 1, []byte(`{"a":1,"b":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) {
		t.Fatalf("expected key order in the input not to matter:\n%s\n%s", a, b)
	}
	if strings.Contains(string(a), "\\u003c") {
		t.Fatal("expected canonical form without HTML escaping")
	}
}
