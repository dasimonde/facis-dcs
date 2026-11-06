package command

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"errors"
	"testing"
)

// DCS-NFR-SEC-02: a signature made with an RSA key below 3000 bits is refused.
func TestAssertSigningKeyStrengthRefusesAWeakRSAKey(t *testing.T) {
	for bits, wantRefused := range map[int]bool{2048: true, 3072: false} {
		key, err := rsa.GenerateKey(rand.Reader, bits)
		if err != nil {
			t.Fatal(err)
		}
		err = assertSigningKeyStrength(&x509.Certificate{PublicKey: &key.PublicKey})
		if refused := errors.Is(err, ErrSignatureInvalid); refused != wantRefused {
			t.Errorf("%d-bit RSA: refused=%v, want %v (%v)", bits, refused, wantRefused, err)
		}
	}
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := assertSigningKeyStrength(&x509.Certificate{PublicKey: &ecKey.PublicKey}); err != nil {
		t.Errorf("a P-256 key must pass, got %v", err)
	}
}
