package envelope

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
)

// testMethodID stands for the recipient's keyAgreement verification method a
// wrap names.
const testMethodID = "did:web:recipient.local#dcs-ecdh"

// softwareKeyAgreement is the test-only counterpart of the HSM: it derives
// the ECDH shared secret from an in-memory static private key.
type softwareKeyAgreement struct{ priv *ecdsa.PrivateKey }

func (a softwareKeyAgreement) DeriveECDH(peerPub *ecdsa.PublicKey) ([]byte, error) {
	priv, err := a.priv.ECDH()
	if err != nil {
		return nil, err
	}
	pub, err := peerPub.ECDH()
	if err != nil {
		return nil, err
	}
	return priv.ECDH(pub)
}

func newRecipient(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate recipient key: %v", err)
	}
	return priv
}

func mustNewCEK(t *testing.T) []byte {
	t.Helper()
	cek, err := NewCEK()
	if err != nil {
		t.Fatalf("NewCEK: %v", err)
	}
	return cek
}

func TestWrapUnwrapRoundtrip(t *testing.T) {
	recipient := newRecipient(t)
	cek := mustNewCEK(t)

	w, err := Wrap(cek, testMethodID, &recipient.PublicKey)
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	if w.Alg != Alg {
		t.Fatalf("alg = %q, want %q", w.Alg, Alg)
	}
	if len(w.Wrapped) != 40 {
		t.Fatalf("wrapped length = %d, want 40", len(w.Wrapped))
	}
	if bytes.Contains(w.Wrapped, cek) {
		t.Fatal("wrapped blob contains the plaintext cek")
	}

	got, err := Unwrap(w, softwareKeyAgreement{priv: recipient})
	if err != nil {
		t.Fatalf("Unwrap: %v", err)
	}
	if !bytes.Equal(got, cek) {
		t.Fatal("unwrapped cek differs from original")
	}
}

func TestUnwrapWithWrongKeyFails(t *testing.T) {
	recipient := newRecipient(t)
	other := newRecipient(t)
	cek := mustNewCEK(t)

	w, err := Wrap(cek, testMethodID, &recipient.PublicKey)
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	if _, err := Unwrap(w, softwareKeyAgreement{priv: other}); err == nil {
		t.Fatal("Unwrap with wrong static key succeeded")
	}
}

func TestUnwrapTamperedWrappedFails(t *testing.T) {
	recipient := newRecipient(t)
	cek := mustNewCEK(t)

	w, err := Wrap(cek, testMethodID, &recipient.PublicKey)
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	w.Wrapped[17] ^= 0x01
	if _, err := Unwrap(w, softwareKeyAgreement{priv: recipient}); err == nil {
		t.Fatal("Unwrap of tampered blob succeeded")
	}
}

func TestUnwrapRejectsForeignAlg(t *testing.T) {
	recipient := newRecipient(t)
	w, err := Wrap(mustNewCEK(t), testMethodID, &recipient.PublicKey)
	if err != nil {
		t.Fatalf("Wrap: %v", err)
	}
	w.Alg = "ECDH-ES"
	if _, err := Unwrap(w, softwareKeyAgreement{priv: recipient}); err == nil {
		t.Fatal("Unwrap accepted a foreign alg")
	}
}

func TestEncryptDecryptRoundtrip(t *testing.T) {
	cek := mustNewCEK(t)
	plaintext := []byte("%PDF-1.7 contract artifact bytes")
	const scope = "urn:dcs:contract:test-1"

	blob, err := Encrypt(cek, plaintext, scope)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Contains(blob, plaintext[:8]) {
		t.Fatal("ciphertext leaks plaintext prefix")
	}

	got, err := Decrypt(cek, blob, scope)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatal("decrypted content differs from original")
	}

	// Nonce is random per call: encrypting twice must not repeat the blob.
	blob2, err := Encrypt(cek, plaintext, scope)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if bytes.Equal(blob, blob2) {
		t.Fatal("two encryptions produced identical blobs")
	}
}

func TestDecryptAADMismatchFails(t *testing.T) {
	cek := mustNewCEK(t)
	blob, err := Encrypt(cek, []byte("body"), "urn:dcs:contract:a")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if _, err := Decrypt(cek, blob, "urn:dcs:contract:b"); err == nil {
		t.Fatal("Decrypt with wrong scope succeeded")
	}
}

func TestDecryptTamperedCiphertextFails(t *testing.T) {
	cek := mustNewCEK(t)
	const scope = "urn:dcs:contract:a"
	blob, err := Encrypt(cek, []byte("body"), scope)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	blob[len(blob)-1] ^= 0x01
	if _, err := Decrypt(cek, blob, scope); err == nil {
		t.Fatal("Decrypt of tampered ciphertext succeeded")
	}
}

func TestDecryptTruncatedBlobFails(t *testing.T) {
	cek := mustNewCEK(t)
	if _, err := Decrypt(cek, []byte{0x01, 0x02, 0x03}, "scope"); err == nil {
		t.Fatal("Decrypt of truncated blob succeeded")
	}
}

func TestWrapRejectsBadInputs(t *testing.T) {
	recipient := newRecipient(t)
	if _, err := Wrap([]byte("short"), testMethodID, &recipient.PublicKey); err == nil {
		t.Fatal("Wrap accepted a short cek")
	}
	if _, err := Wrap(mustNewCEK(t), testMethodID, nil); err == nil {
		t.Fatal("Wrap accepted a nil recipient key")
	}
}
