package envelope

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"math/big"
	"os"
	"testing"

	"digital-contracting-service/internal/base/hsm"
)

// hsmPublicKey loads the dcs-ecdh public key from the SoftHSM2 token the
// environment points at, or skips the test when no token is configured.
func hsmPublicKey(t *testing.T) *ecdsa.PublicKey {
	t.Helper()
	if os.Getenv("SOFTHSM2_CONF") == "" {
		t.Skip("SOFTHSM2_CONF not set; skipping SoftHSM2 integration test (provision a token via scripts/hsm-provision.sh and export SOFTHSM2_CONF, PKCS11_MODULE_PATH, PKCS11_TOKEN_LABEL, PKCS11_PIN)")
	}
	h, err := hsm.Open(hsm.ConfigFromEnv())
	if err != nil {
		t.Fatalf("open hsm: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	signer, err := h.Signer(hsm.KeyLabelECDH())
	if err != nil {
		t.Fatalf("load %s key: %v", hsm.KeyLabelECDH(), err)
	}
	pub, ok := signer.Public().(*ecdsa.PublicKey)
	if !ok {
		t.Fatalf("%s key is not ECDSA", hsm.KeyLabelECDH())
	}
	return pub
}

func TestHSMDeriveECDHMatchesSoftware(t *testing.T) {
	hsmPub := hsmPublicKey(t)

	eph, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate ephemeral key: %v", err)
	}
	point := eph.PublicKey().Bytes()
	ephECDSA := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     new(big.Int).SetBytes(point[1:33]),
		Y:     new(big.Int).SetBytes(point[33:65]),
	}

	zHSM, err := hsm.DeriveECDH(hsm.KeyLabelECDH(), ephECDSA)
	if err != nil {
		t.Fatalf("DeriveECDH via hsm: %v", err)
	}

	hsmPubECDH, err := hsmPub.ECDH()
	if err != nil {
		t.Fatalf("hsm public key: %v", err)
	}
	zSoft, err := eph.ECDH(hsmPubECDH)
	if err != nil {
		t.Fatalf("software ecdh: %v", err)
	}

	if !bytes.Equal(zHSM, zSoft) {
		t.Fatal("hsm-derived shared secret differs from software-side ecdh")
	}
}

func TestHSMWrapUnwrapRoundtrip(t *testing.T) {
	hsmPub := hsmPublicKey(t)

	cek := mustNewCEK(t)
	w, err := Wrap(cek, "did:web:test.local#dcs-ecdh", hsmPub)
	if err != nil {
		t.Fatalf("Wrap to hsm key: %v", err)
	}

	got, err := Unwrap(w, HSMKeyAgreement(hsm.KeyLabelECDH()))
	if err != nil {
		t.Fatalf("Unwrap via hsm: %v", err)
	}
	if !bytes.Equal(got, cek) {
		t.Fatal("hsm-unwrapped cek differs from original")
	}

	// Full envelope: content sealed under the CEK survives the HSM unwrap.
	const scope = "urn:dcs:contract:hsm-it"
	blob, err := Encrypt(cek, []byte("artifact bytes"), scope)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	plaintext, err := Decrypt(got, blob, scope)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if string(plaintext) != "artifact bytes" {
		t.Fatal("decrypted content differs from original")
	}
}
