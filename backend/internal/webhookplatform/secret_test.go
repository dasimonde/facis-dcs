package webhookplatform

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"testing"
)

// Test vector of the Standard Webhooks reference libraries
// (standard-webhooks/standard-webhooks, libraries/python/tests/test_webhooks.py,
// test_sign_function).
func TestSignatureMatchesStandardWebhooksVector(t *testing.T) {
	const (
		secret    = "whsec_MfKQ9r8GKYqrTwjUPD8ILPZIo2LaLaSw"
		msgID     = "msg_p5jXN8AQM9LWM0D4loKWxJek"
		timestamp = "1614265330"
		body      = `{"test": 2432232314}`
		want      = "v1,g0hM9SsE+OTPJTGt/tmIKtSyZlE3uFJELVlNIOLJ1OE="
	)
	got, err := Signature(secret, msgID, timestamp, []byte(body))
	if err != nil || got != want {
		t.Fatalf("Signature = %q, %v; want %q", got, err, want)
	}
	for name, tampered := range map[string][3]string{
		"body":      {msgID, timestamp, `{"test": 2432232315}`},
		"msg id":    {msgID + "x", timestamp, body},
		"timestamp": {msgID, "1614265331", body},
	} {
		if got, _ := Signature(secret, tampered[0], tampered[1], []byte(tampered[2])); got == want {
			t.Errorf("tampered %s gives the same signature", name)
		}
	}
}

// softwareKeyAgreement is the test-only counterpart of the HSM.
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

func testSecretBox(t *testing.T) SecretBox {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return SecretBox{Agreement: softwareKeyAgreement{priv: priv}, MethodID: "did:web:test.local#dcs-ecdh", Pub: &priv.PublicKey}
}

func TestSecretBoxRoundTrip(t *testing.T) {
	box := testSecretBox(t)
	secret, err := NewSecret()
	if err != nil {
		t.Fatal(err)
	}
	const id = "6f1c1b39-9f0e-4d8e-9a43-1b2a3c4d5e6f"

	ciphertext, wrappedCEK, err := box.Seal(id, secret)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if bytes.Contains(ciphertext, []byte(secret)) || bytes.Contains(wrappedCEK, []byte(secret)) {
		t.Fatalf("stored values contain the plaintext secret")
	}
	got, err := box.Open(id, ciphertext, wrappedCEK)
	if err != nil || got != secret {
		t.Fatalf("Open = %q, %v; want the sealed secret", got, err)
	}

	if _, err := box.Open("another-subscription", ciphertext, wrappedCEK); err == nil {
		t.Fatalf("Open with another subscription id succeeded")
	}
	if _, err := testSecretBox(t).Open(id, ciphertext, wrappedCEK); err == nil {
		t.Fatalf("Open with another key-agreement key succeeded")
	}
}
