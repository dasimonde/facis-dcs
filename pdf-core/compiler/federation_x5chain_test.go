package compiler

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

// peerSigner signs with a key that is not the one TestMain configured for this
// process, standing in for the second DCS instance's dcs-c2pa key.
type peerSigner struct{ key *ecdsa.PrivateKey }

func (p peerSigner) Sign(_ context.Context, data []byte) ([]byte, error) {
	return deterministicES256(p.key, data), nil
}

// peerInstanceContext returns a render context standing in for a federation
// peer's pdf-core: its own signing key and its own x5chain leaf, neither of
// which this process holds. Every DCS instance provisions its own PKCS#11 token
// (deployment/helm/values.bdd2.yml runs a second pdf-core for exactly that
// reason), so the manifests on a contract that crossed the peer boundary carry
// a certificate the receiving instance cannot produce from its own config.
func peerInstanceContext(t *testing.T) context.Context {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate peer key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "DCS-PEER test c2pa signer", Organization: []string{"DCS-PEER"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatalf("create peer certificate: %v", err)
	}
	return WithSigningChain(WithSigner(context.Background(), peerSigner{key: key}), [][]byte{der})
}

// TestVerifyIncrementalUpdateReproducesAPeerAmendedPDF is the federated case of
// the two-instance vertical: this instance compiled the contract, shipped it,
// and the peer amended the bytes it received and shipped them back. The stored
// artifact is the peer's, verbatim, so its last hop was signed under the peer's
// leaf — the deterministic replay has to reproduce that hop under the same leaf
// rather than substituting this instance's.
func TestVerifyIncrementalUpdateReproducesAPeerAmendedPDF(t *testing.T) {
	original, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	amended, err := UpdatePDFWithOptions(peerInstanceContext(t), original, []byte(minimalPayloadAmended), nil, "", CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("peer UpdatePDFWithOptions: %v", err)
	}

	reproduced, err := VerifyIncrementalUpdate(testSigningContext(), amended)
	if err != nil {
		t.Fatalf("VerifyIncrementalUpdate on a peer-amended PDF: %v", err)
	}
	if !bytes.HasPrefix(ZeroCOSESignatures(amended), ZeroCOSESignatures(reproduced)) {
		t.Error("reproduction does not match the stored peer-amended bytes")
	}
}

// TestVerifyIncrementalUpdateReproducesAPeerCompiledBase is the mirror case: the
// contract was compiled by the peer and this instance amended what it received.
// The base compile — VerifyIncrementalUpdate's first boundary check — must then
// be replayed under the peer's leaf.
func TestVerifyIncrementalUpdateReproducesAPeerCompiledBase(t *testing.T) {
	peerCtx := peerInstanceContext(t)
	original, err := CompilePDF(peerCtx, []byte(minimalPayloadBase), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("peer CompilePDF: %v", err)
	}

	amended, err := UpdatePDFWithOptions(testSigningContext(), original, []byte(minimalPayloadAmended), nil, "", CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("UpdatePDFWithOptions: %v", err)
	}

	reproduced, err := VerifyIncrementalUpdate(testSigningContext(), amended)
	if err != nil {
		t.Fatalf("VerifyIncrementalUpdate on a peer-compiled base: %v", err)
	}
	if !bytes.HasPrefix(ZeroCOSESignatures(amended), ZeroCOSESignatures(reproduced)) {
		t.Error("reproduction does not match the stored bytes")
	}
}

// TestExtractSigningChainReturnsTheEmbeddedLeaf checks the read-back the verify
// path depends on: the chain recovered from a compiled PDF is the one its
// manifest was signed under, not this process's configured chain.
func TestExtractSigningChainReturnsTheEmbeddedLeaf(t *testing.T) {
	peerCtx := peerInstanceContext(t)
	peerChain, _ := signingChainFromContext(peerCtx)

	pdf, err := CompilePDF(peerCtx, []byte(minimalPayloadBase), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("peer CompilePDF: %v", err)
	}

	got, err := ExtractSigningChain(pdf)
	if err != nil {
		t.Fatalf("ExtractSigningChain: %v", err)
	}
	if len(got) != len(peerChain) || !bytes.Equal(got[0], peerChain[0]) {
		t.Error("ExtractSigningChain did not return the leaf the manifest was signed under")
	}
	if bytes.Equal(got[0], testC2PAChain[0]) {
		t.Error("peer-compiled PDF must not carry this instance's own leaf")
	}
}

// TestPeerCompiledManifestClaimSignatureVerifies guards the read-back against
// becoming a hole in the provenance check: a manifest signed under the peer's
// leaf must still verify against that leaf, so trusting the embedded chain for
// the byte reproduction does not trust it for authenticity.
func TestPeerCompiledManifestClaimSignatureVerifies(t *testing.T) {
	pdf, err := CompilePDF(peerInstanceContext(t), []byte(minimalPayloadBase), CanonicalCompiledAt)
	if err != nil {
		t.Fatalf("peer CompilePDF: %v", err)
	}
	if err := VerifyC2PAClaimSignatures(pdf); err != nil {
		t.Fatalf("VerifyC2PAClaimSignatures on a peer-compiled PDF: %v", err)
	}
}
