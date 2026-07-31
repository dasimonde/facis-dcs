package compiler

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"os"
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
// chain must therefore be refused at load time rather than producing manifests
// no verifier accepts.
func TestLoadSigningMaterialRejectsLeafWithoutOrganization(t *testing.T) {
	chain := mustX5ChainPEMWithSubject(t, pkix.Name{CommonName: "DCS Dev dcs-c2pa Signer"})
	env := map[string]string{envX5ChainPEM: chain}

	_, err := loadSigningMaterialFromEnv(func(k string) string { return env[k] }, os.ReadFile)
	if err == nil {
		t.Fatalf("loadSigningMaterialFromEnv() accepted a leaf without an organizationName")
	}
	if !strings.Contains(err.Error(), "organizationName") {
		t.Fatalf("error does not name the missing field: %v", err)
	}
}

// TestLoadSigningMaterialAcceptsLeafWithOrganization is the positive control:
// the same certificate gains an O= and must load.
func TestLoadSigningMaterialAcceptsLeafWithOrganization(t *testing.T) {
	chain := mustX5ChainPEMWithSubject(t, pkix.Name{
		Organization: []string{"FACIS DCS"},
		CommonName:   "DCS Dev dcs-c2pa Signer",
	})
	env := map[string]string{envX5ChainPEM: chain}

	material, err := loadSigningMaterialFromEnv(func(k string) string { return env[k] }, os.ReadFile)
	if err != nil {
		t.Fatalf("loadSigningMaterialFromEnv() error = %v", err)
	}
	if len(material.certChainDER) != 1 {
		t.Fatalf("cert chain length = %d, want 1", len(material.certChainDER))
	}
}

// TestLoadSigningMaterialChecksLeafNotAnchor guards against checking the wrong
// certificate: only the leaf signs claims, so a chain whose CA carries the
// organizationName but whose leaf does not must still be refused.
func TestLoadSigningMaterialChecksLeafNotAnchor(t *testing.T) {
	leaf := mustX5ChainPEMWithSubject(t, pkix.Name{CommonName: "DCS Dev dcs-c2pa Signer"})
	anchor := mustX5ChainPEMWithSubject(t, pkix.Name{
		Organization: []string{"FACIS DCS"},
		CommonName:   "DCS Dev C2PA CA",
	})
	env := map[string]string{envX5ChainPEM: leaf + anchor}

	if _, err := loadSigningMaterialFromEnv(func(k string) string { return env[k] }, os.ReadFile); err == nil {
		t.Fatalf("loadSigningMaterialFromEnv() accepted a chain whose leaf has no organizationName")
	}
}
