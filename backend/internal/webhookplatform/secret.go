package webhookplatform

import (
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"digital-contracting-service/internal/base/envelope"
)

// secretPrefix marks a Standard Webhooks symmetric secret
// (https://www.standardwebhooks.com/): "whsec_" and the base64 key.
const secretPrefix = "whsec_"

// NewSecret returns a subscription secret with a fresh 256-bit key.
func NewSecret() (string, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return "", fmt.Errorf("generate webhook secret: %w", err)
	}
	return secretPrefix + base64.StdEncoding.EncodeToString(key), nil
}

// Signature returns the Standard Webhooks v1 "webhook-signature" value:
// "v1," and the base64 HMAC-SHA256 over msgID "." timestamp "." body, keyed
// with the decoded part of secret after "whsec_".
func Signature(secret, msgID, timestamp string, body []byte) (string, error) {
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, secretPrefix))
	if err != nil {
		return "", fmt.Errorf("decode webhook secret: %w", err)
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(msgID + "." + timestamp + "."))
	mac.Write(body)
	return "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil)), nil
}

// SecretBox encrypts subscription secrets at rest (DCS-NFR-SEC-06,
// DCS-NFR-SEC-14). Each secret has its own CEK, wrapped to the instance's
// keyAgreement key; only the HSM can unwrap it.
type SecretBox struct {
	Agreement envelope.KeyAgreement
	MethodID  string
	Pub       *ecdsa.PublicKey
}

// secretScope binds a ciphertext to its subscription, so that a ciphertext
// copied to another row does not decrypt.
func secretScope(subscriptionID string) string {
	return "webhook-subscription:" + subscriptionID
}

// Seal returns the encrypted secret and the JSON-encoded wrapped CEK.
func (b SecretBox) Seal(subscriptionID, secret string) (ciphertext, wrappedCEK []byte, err error) {
	cek, err := envelope.NewCEK()
	if err != nil {
		return nil, nil, err
	}
	ciphertext, err = envelope.Encrypt(cek, []byte(secret), secretScope(subscriptionID))
	if err != nil {
		return nil, nil, fmt.Errorf("encrypt webhook secret: %w", err)
	}
	wrapped, err := envelope.Wrap(cek, b.MethodID, b.Pub)
	if err != nil {
		return nil, nil, fmt.Errorf("wrap webhook secret cek: %w", err)
	}
	wrappedCEK, err = json.Marshal(wrapped)
	if err != nil {
		return nil, nil, fmt.Errorf("encode wrapped webhook secret cek: %w", err)
	}
	return ciphertext, wrappedCEK, nil
}

// Open reverses Seal.
func (b SecretBox) Open(subscriptionID string, ciphertext, wrappedCEK []byte) (string, error) {
	var wrapped envelope.WrappedCEK
	if err := json.Unmarshal(wrappedCEK, &wrapped); err != nil {
		return "", fmt.Errorf("decode wrapped webhook secret cek: %w", err)
	}
	cek, err := envelope.Unwrap(&wrapped, b.Agreement)
	if err != nil {
		return "", fmt.Errorf("unwrap webhook secret cek: %w", err)
	}
	secret, err := envelope.Decrypt(cek, ciphertext, secretScope(subscriptionID))
	if err != nil {
		return "", fmt.Errorf("decrypt webhook secret: %w", err)
	}
	return string(secret), nil
}
