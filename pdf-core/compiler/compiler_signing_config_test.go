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
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mustTestX5ChainPEM returns a self-signed P-256 leaf certificate as an x5chain
// PEM, standing in for the dev CA leaf whose public key matches the dcs-c2pa
// token key in production.
func mustTestX5ChainPEM(t *testing.T) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{Organization: []string{"FACIS DCS"}, CommonName: "DCS-PDF-CORE test signer"},
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

func TestLoadSigningMaterialFromEnv_InlineX5Chain(t *testing.T) {
	env := map[string]string{envX5ChainPEM: mustTestX5ChainPEM(t)}
	material, err := loadSigningMaterialFromEnv(func(k string) string { return env[k] }, os.ReadFile)
	if err != nil {
		t.Fatalf("loadSigningMaterialFromEnv() error = %v", err)
	}
	if len(material.certChainDER) != 1 {
		t.Fatalf("cert chain length = %d, want 1", len(material.certChainDER))
	}
}

func TestLoadSigningMaterialFromEnv_FileX5Chain(t *testing.T) {
	dir := t.TempDir()
	chainPath := filepath.Join(dir, "x5chain.pem")
	if err := os.WriteFile(chainPath, []byte(mustTestX5ChainPEM(t)), 0o644); err != nil {
		t.Fatalf("write chain: %v", err)
	}
	env := map[string]string{envX5ChainPEMFile: chainPath}
	material, err := loadSigningMaterialFromEnv(func(k string) string { return env[k] }, os.ReadFile)
	if err != nil {
		t.Fatalf("loadSigningMaterialFromEnv() error = %v", err)
	}
	if len(material.certChainDER) != 1 {
		t.Fatalf("cert chain length = %d, want 1", len(material.certChainDER))
	}
}

func TestLoadSigningMaterialFromEnv_MissingX5Chain(t *testing.T) {
	if _, err := loadSigningMaterialFromEnv(func(string) string { return "" }, os.ReadFile); err == nil {
		t.Fatalf("expected error when x5chain is missing")
	}
}

// TestSignClaimSigStructure_RequiresSignerInContext proves pdf-core holds no key:
// a compile step that reaches a COSE signature without a Signer injected fails
// loudly rather than falling back to any built-in key.
func TestSignClaimSigStructure_RequiresSignerInContext(t *testing.T) {
	if _, err := signClaimSigStructure(context.Background(), []byte("protected"), []byte("claim")); err == nil {
		t.Fatalf("expected error when no signer is present in context")
	}
}

// TestCapturingSignerRecordsSigStructuresAndZeroes proves the prepare-step signer
// returns a zeroed 64-byte placeholder and records the exact Sig_structure bytes
// the DCS backend must sign.
func TestCapturingSignerRecordsSigStructuresAndZeroes(t *testing.T) {
	signer := NewCapturingSigner()
	sig, err := signer.Sign(context.Background(), []byte("sig-structure-A"))
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	if len(sig) != 64 || !isAllZero(sig) {
		t.Fatalf("placeholder must be 64 zero bytes, got % x", sig)
	}
	if _, err := signer.Sign(context.Background(), []byte("sig-structure-B")); err != nil {
		t.Fatalf("Sign() error = %v", err)
	}
	captured := signer.Captured()
	if len(captured) != 2 || !bytes.Equal(captured[0], []byte("sig-structure-A")) || !bytes.Equal(captured[1], []byte("sig-structure-B")) {
		t.Fatalf("captured Sig_structures not recorded in order: %q", captured)
	}
}

// TestInjectCOSESignaturesFillsOnlyZeroedSlots proves the stateless embed step
// fills a prepared PDF's zeroed 64-byte slot in place, leaving a pre-existing
// (non-zero) signature untouched.
func TestInjectCOSESignaturesFillsOnlyZeroedSlots(t *testing.T) {
	existing := bytes.Repeat([]byte{0xAB}, 64)
	zeroed := make([]byte, 64)
	prepared := bytes.Join([][]byte{
		[]byte("....manifestA...."), coseDetachedSig64Marker, existing,
		[]byte("....manifestB...."), coseDetachedSig64Marker, zeroed,
		[]byte("....tail...."),
	}, nil)

	newSig := bytes.Repeat([]byte{0x11}, 64)
	out, err := InjectCOSESignatures(prepared, [][]byte{newSig})
	if err != nil {
		t.Fatalf("InjectCOSESignatures() error = %v", err)
	}
	if !bytes.Contains(out, append(append([]byte(nil), coseDetachedSig64Marker...), existing...)) {
		t.Fatalf("pre-existing signature was modified")
	}
	if !bytes.Contains(out, append(append([]byte(nil), coseDetachedSig64Marker...), newSig...)) {
		t.Fatalf("zeroed slot was not filled with the provided signature")
	}
}

func TestInjectCOSESignaturesRejectsCountMismatch(t *testing.T) {
	zeroed := make([]byte, 64)
	prepared := bytes.Join([][]byte{coseDetachedSig64Marker, zeroed}, nil)
	if _, err := InjectCOSESignatures(prepared, [][]byte{bytes.Repeat([]byte{1}, 64), bytes.Repeat([]byte{2}, 64)}); err == nil {
		t.Fatalf("expected error when signature count exceeds zeroed slots")
	}
	if _, err := InjectCOSESignatures(prepared, nil); err == nil {
		t.Fatalf("expected error when a zeroed slot is left unfilled")
	}
}
