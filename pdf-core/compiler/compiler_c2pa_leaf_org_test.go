package compiler

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"strings"
	"testing"
	"time"
)

// mustX5ChainPEMWithSubject issues a self-signed P-256 leaf carrying subject and
// returns it as an x5chain PEM.
func mustX5ChainPEMWithSubject(t *testing.T, subject pkix.Name) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               subject,
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageDigitalSignature,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return string(certPEM(der))
}

// TestLoadSigningMaterialRejectsLeafWithoutOrganization pins the reason a manifest
// signed with such a leaf is reported invalid by every C2PA verifier.
//
// c2pa-rs verify_cose validates the COSE signature and then reads the signing
// certificate's organizationName to populate the returned CertificateInfo:
//
//	validator.validate(&sign1.signature, &tbs, pk_der)?;
//	let subject = sign_cert.subject().iter_organization().last()
//	    .ok_or(CoseError::MissingSigningCertificateChain)?;
//
// A leaf with only a CN makes that lookup return None, so verify_cose returns an
// error even though the signature verified. The caller reports the failure as
// claimSignature.mismatch and the asset's validation_state becomes Invalid — a
// cryptographically sound signature indistinguishable from a forged one. The
// chain must therefore be refused when the caller supplies it rather than
// producing manifests no verifier accepts.
func TestParseSigningChainRejectsLeafWithoutOrganization(t *testing.T) {
	chain := mustX5ChainPEMWithSubject(t, pkix.Name{CommonName: "DCS Dev dcs-c2pa Signer"})

	_, err := ParseSigningChainPEM([]byte(chain))
	if err == nil {
		t.Fatalf("ParseSigningChainPEM() accepted a leaf without an organizationName")
	}
	if !strings.Contains(err.Error(), "organizationName") {
		t.Fatalf("error does not name the missing field: %v", err)
	}
}

// TestParseSigningChainAcceptsLeafWithOrganization is the positive control:
// the same certificate gains an O= and must parse.
func TestParseSigningChainAcceptsLeafWithOrganization(t *testing.T) {
	chain := mustX5ChainPEMWithSubject(t, pkix.Name{
		Organization: []string{"FACIS DCS"},
		CommonName:   "DCS Dev dcs-c2pa Signer",
	})

	parsed, err := ParseSigningChainPEM([]byte(chain))
	if err != nil {
		t.Fatalf("ParseSigningChainPEM() error = %v", err)
	}
	if len(parsed) != 1 {
		t.Fatalf("cert chain length = %d, want 1", len(parsed))
	}
}

// TestParseSigningChainChecksLeafNotAnchor guards against checking the wrong
// certificate: only the leaf signs claims, so a chain whose CA carries the
// organizationName but whose leaf does not must still be refused.
func TestParseSigningChainChecksLeafNotAnchor(t *testing.T) {
	leaf := mustX5ChainPEMWithSubject(t, pkix.Name{CommonName: "DCS Dev dcs-c2pa Signer"})
	anchor := mustX5ChainPEMWithSubject(t, pkix.Name{
		Organization: []string{"FACIS DCS"},
		CommonName:   "DCS Dev C2PA CA",
	})
	if _, err := ParseSigningChainPEM([]byte(leaf + anchor)); err == nil {
		t.Fatalf("ParseSigningChainPEM() accepted a chain whose leaf has no organizationName")
	}
}
