package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"math/big"
	"net/http"
	"testing"
	"time"

	compiler "example.com/m/V2/compiler"
)

// peerRenderSigner signs with a key this pdf-core process does not hold, as the
// federation peer's own DCS backend does with its dcs-c2pa key.
type peerRenderSigner struct{ key *ecdsa.PrivateKey }

func (p peerRenderSigner) Sign(_ context.Context, data []byte) ([]byte, error) {
	digest := sha256.Sum256(data)
	r, s, err := ecdsa.Sign(rand.Reader, p.key, digest[:])
	if err != nil {
		return nil, err
	}
	out := make([]byte, 64)
	r.FillBytes(out[:32])
	s.FillBytes(out[32:])
	return out, nil
}

// peerRenderContext returns a render context standing in for the other DCS
// instance's pdf-core: its own PKCS#11 token issued its own C2PA leaf, so the
// x5chain in the manifests it produces is not the one this process is configured
// with (deployment/helm/values.bdd2.yml gives instance B its own pdf-core for
// exactly that reason).
func peerRenderContext(t *testing.T) context.Context {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate peer key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(7),
		Subject:               pkix.Name{Organization: []string{"FACIS DCS PEER"}, CommonName: "peer c2pa signer"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageEmailProtection},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatalf("create peer certificate: %v", err)
	}
	return compiler.WithSigningChain(
		compiler.WithSigner(context.Background(), peerRenderSigner{key: key}),
		[][]byte{der})
}

// TestVerify_PeerCompiledPDFReproduces covers /verify's plain branch on a PDF
// this instance did not compile. The federated vertical stores the counterparty's
// bytes verbatim, so the document under verification was rendered under the
// peer's signing leaf; re-rendering it under this instance's leaf reproduced
// different bytes and the endpoint answered 409 on an untampered contract.
func TestVerify_PeerCompiledPDFReproduces(t *testing.T) {
	pdf, err := compiler.CompilePDF(peerRenderContext(t), []byte(minimalPayload), compiler.CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("peer CompilePDF: %v", err)
	}

	rec := doRequest(http.MethodPost, "/verify", bytes.NewReader(pdf), "application/pdf")
	if rec.Code != http.StatusOK {
		t.Fatalf("verify a peer-compiled PDF: status %d: %s", rec.Code, rec.Body.String())
	}
	var result verifyResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode verify response: %v", err)
	}
	if !result.Match {
		t.Error("expected match=true for an untampered peer-compiled PDF")
	}
	// The lifecycle credential is only reported for a document /verify accepted,
	// so a refusal here would also have read as "no credential in the PDF".
	if result.VCPresent {
		t.Error("a freshly compiled base carries no lifecycle credential yet")
	}
}
