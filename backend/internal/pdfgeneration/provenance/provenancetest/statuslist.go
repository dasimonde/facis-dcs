// Package provenancetest serves a status list the way a deployment serves its
// own, for tests on either side of that boundary.
//
// A test that stubs the revocation lookup itself proves only that the caller
// called something. The whole point of ADR-34 is that the answer comes from a
// list somebody signed, so what a test has to exercise is the real fetch,
// media-type routing, signature check, chain verification, leaf-identifies-issuer
// binding and bit decoding — the path a deployment runs for anyone's status list,
// including its own. This starts a server that satisfies all of it, and hands
// back the verifier wired the way the running process wires it.
package provenancetest

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"digital-contracting-service/internal/auth/oid4vp/status"
	"digital-contracting-service/internal/auth/oid4vp/status/handler"
	"digital-contracting-service/internal/pdfgeneration/provenance"
)

// SignedStatusList is a running status-list endpoint plus the verifier that
// trusts it.
type SignedStatusList struct {
	// IssuerURL is the origin the list is served under and the `iss` its token
	// names — the identity the chain's leaf carries as a SAN URI.
	IssuerURL string
	// ListURI is what a credential names, what a verifier fetches, and the
	// token's own `sub`. All three are this string; a verifier refuses the list
	// on any difference.
	ListURI string
	// Root is the anchor the served chain reaches, for a caller that needs to
	// configure trust itself.
	Root *x509.Certificate
	// Verifier resolves a credential's status against this list, through the
	// ordinary path.
	Verifier *provenance.CredentialStatusVerifier
	fetched  *bool
}

// Fetched reports whether the list was actually retrieved — the observable
// difference between following a credential's own status pointer and refusing to.
func (s *SignedStatusList) Fetched() bool { return *s.fetched }

// Credential is a credential advertising entry index in this list, in the shape
// the lifecycle and signing-summary credentials carry.
func (s *SignedStatusList) Credential(index uint32) []byte {
	vc, _ := json.Marshal(map[string]any{
		"@context": []any{"https://www.w3.org/ns/credentials/v2"},
		"type":     []any{"VerifiableCredential", "ContractLifecycleCredential"},
		"issuer":   "did:web:example.org",
		"credentialStatus": map[string]any{
			"id":                   fmt.Sprintf("%s#%d", s.ListURI, index),
			"type":                 "TokenStatusList",
			"statusPurpose":        "revocation",
			"statusListIndex":      fmt.Sprintf("%d", index),
			"statusListCredential": s.ListURI,
		},
	})
	return vc
}

// NewSignedStatusList starts a server serving list 1 with the given entries
// revoked, signed under a freshly minted root that the returned verifier — and
// only it — anchors.
func NewSignedStatusList(t *testing.T, revoked ...uint32) *SignedStatusList {
	t.Helper()

	srv := httptest.NewUnstartedServer(nil)
	issuerURL := "http://" + srv.Listener.Addr().String()

	caKey, ca := mintRoot(t)

	// The leaf NAMES the issuer, by SAN URI. Without it the chain still verifies
	// and the list is still refused: a chain says the root vouched for the
	// certificate, not whose it is.
	issuerURI, err := url.Parse(issuerURL)
	if err != nil {
		t.Fatalf("parse issuer url: %v", err)
	}
	leafKey, leafDER := mintLeaf(t, ca, caKey, issuerURI)

	fetched := false
	signer := &provenance.StatusListSigner{
		Issuer:      issuerURL,
		ListURI:     func(listID int) string { return provenance.StatusListURI(issuerURL, listID) },
		ListID:      provenance.DefaultListID,
		Chain:       []string{base64.StdEncoding.EncodeToString(leafDER), base64.StdEncoding.EncodeToString(ca.Raw)},
		Signer:      leafKey,
		Revocations: &fixedRevocations{indices: revoked},
		Size:        1024,
	}
	served := provenance.StatusListHandler(signer)
	srv.Config = &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fetched = true
			served.ServeHTTP(w, r)
		}),
	}
	srv.Start()
	t.Cleanup(srv.Close)
	if srv.URL != issuerURL {
		t.Fatalf("server moved from %s to %s after start", issuerURL, srv.URL)
	}

	roots := x509.NewCertPool()
	roots.AddCert(ca)

	return &SignedStatusList{
		IssuerURL: issuerURL,
		ListURI:   provenance.StatusListURI(issuerURL, provenance.DefaultListID),
		Root:      ca,
		Verifier:  provenance.NewCredentialStatusVerifier(VerifierAnchoring(roots)),
		fetched:   &fetched,
	}
}

// VerifierAnchoring is the status-list verifier the running process wires,
// trusting exactly the given roots and no bundled key.
func VerifierAnchoring(roots *x509.CertPool) *status.Verifier {
	return handler.NewVerifier(
		&status.TrustConfig{Issuers: map[string]status.TrustIssuerEntry{}, X5CRoots: roots},
		handler.Options{})
}

func mintRoot(t *testing.T) (*ecdsa.PrivateKey, *x509.Certificate) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate root key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test DCS Root CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create root certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse root certificate: %v", err)
	}
	return key, cert
}

func mintLeaf(t *testing.T, ca *x509.Certificate, caKey *ecdsa.PrivateKey, issuer *url.URL) (*ecdsa.PrivateKey, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate leaf key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(2),
		Subject:               pkix.Name{CommonName: "Test DCS Signer"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		URIs:                  []*url.URL{issuer},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create leaf certificate: %v", err)
	}
	return key, der
}

type fixedRevocations struct{ indices []uint32 }

func (r *fixedRevocations) Revoke(context.Context, string) error { return nil }
func (r *fixedRevocations) RevokedIndices(context.Context, int) ([]uint32, error) {
	return r.indices, nil
}
