package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	compiler "example.com/m/V2/compiler"
)

// doRequestWithoutChain issues a request that names no signing chain, as a
// caller that expects pdf-core to hold one of its own would.
func doRequestWithoutChain(method, path string, body io.Reader, contentType string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	newServer().ServeHTTP(rec, req)
	return rec
}

// TestRenderEndpointsRefuseWithoutASigningChain pins the decision that a request
// naming no chain is refused rather than served under a configured default.
// pdf-core is shared by several DCS instances; a default is what let it sign one
// instance's documents under another instance's identity, and a fallback is how
// that survived unnoticed.
func TestRenderEndpointsRefuseWithoutASigningChain(t *testing.T) {
	pdf := compilePDF(t)

	for _, tc := range []struct {
		endpoint    string
		body        io.Reader
		contentType string
	}{
		{"/render", bytes.NewBufferString(minimalPayload), "application/ld+json"},
		{"/render/reanchor", bytes.NewReader(pdf), "application/pdf"},
		{"/verify", bytes.NewReader(pdf), "application/pdf"},
		{"/verify/content", bytes.NewReader(pdf), "application/pdf"},
	} {
		rec := doRequestWithoutChain(http.MethodPost, tc.endpoint, tc.body, tc.contentType)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s without a signing chain: status %d, want 400 (body: %s)", tc.endpoint, rec.Code, rec.Body.String())
		}
		if !bytes.Contains(rec.Body.Bytes(), []byte(signingChainHeader)) {
			t.Errorf("%s: refusal does not name the missing header: %s", tc.endpoint, rec.Body.String())
		}
	}
}

// TestRenderRefusesAChainWhoseLeafHasNoOrganization keeps the C2PA certificate
// profile enforced now that the chain arrives per request rather than at boot:
// c2pa-rs reports a leaf without an organizationName as claimSignature.mismatch,
// so accepting one produces manifests every verifier calls forged.
func TestRenderRefusesAChainWhoseLeafHasNoOrganization(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/render", bytes.NewBufferString(minimalPayload))
	req.Header.Set("Content-Type", "application/ld+json")
	req.Header.Set(signingChainHeader, base64.StdEncoding.EncodeToString(cnOnlyChainPEM(t)))

	rec := httptest.NewRecorder()
	newServer().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte("organizationName")) {
		t.Errorf("refusal does not name the missing field: %s", rec.Body.String())
	}
}

// TestVerifyReportsWhoSignedEachManifest is the second half of the verify
// answer. Replaying a manifest under the certificate it carries shows the bytes
// are self-consistent; it cannot show whose certificate that is, because the
// artifact supplied it. The caller therefore also names the chain it expects,
// and pdf-core reports per manifest whether that is what signed it — so a
// federated contract reads as "reproduces, and this hop was signed by someone
// else" instead of an unqualified match.
func TestVerifyReportsWhoSignedEachManifest(t *testing.T) {
	original := compilePDF(t)
	peerAmended := amendUnderPeerChain(t, original)

	rec := doRequest(http.MethodPost, "/verify", bytes.NewReader(peerAmended), "application/pdf")
	if rec.Code != http.StatusOK {
		t.Fatalf("verify: status %d: %s", rec.Code, rec.Body.String())
	}
	var result verifyResponse
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if !result.Match {
		t.Fatal("a peer-amended PDF must still reproduce")
	}
	if len(result.Signers) != 2 {
		t.Fatalf("signers: got %d entries, want one per manifest (2)", len(result.Signers))
	}
	if !result.Signers[0].Expected {
		t.Error("the genesis manifest was signed under the caller's own chain and must be reported as expected")
	}
	if result.Signers[1].Expected {
		t.Error("the peer's amendment was not signed under the caller's chain and must not be reported as expected")
	}
	if result.Signers[0].LeafSHA256 == result.Signers[1].LeafSHA256 {
		t.Error("the two manifests were signed under different leaves and must be fingerprinted apart")
	}
	if result.Signers[1].Subject == "" {
		t.Error("a foreign signer must be named, not merely flagged")
	}
}

// cnOnlyChainPEM issues a leaf with a CommonName and no organizationName.
func cnOnlyChainPEM(t *testing.T) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(9),
		Subject:               pkix.Name{CommonName: "no-organization signer"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return certPEM(der)
}

// amendUnderPeerChain appends an amendment signed under a chain this instance
// does not hold, as the counterparty's own pdf-core produces.
func amendUnderPeerChain(t *testing.T, original []byte) []byte {
	t.Helper()
	amended, err := compiler.UpdatePDFWithOptions(peerRenderContext(t), original,
		[]byte(minimalPayloadAmended), nil, "", compiler.CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("peer amendment: %v", err)
	}
	return amended
}
