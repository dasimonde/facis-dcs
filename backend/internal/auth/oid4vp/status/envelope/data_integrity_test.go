package envelope_test

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status/envelope"

	"github.com/stretchr/testify/require"
)

func testECPrivateKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return key
}

func sampleW3CDocument(uri string, encodedList string) map[string]any {
	return map[string]any{
		"@context":  []any{"https://www.w3.org/ns/credentials/v2"},
		"id":        uri + "#credential",
		"type":      []any{"VerifiableCredential", "BitstringStatusListCredential"},
		"issuer":    "did:web:dev.example:issuer:poa",
		"validFrom": "2024-06-23T00:00:00Z",
		"credentialSubject": map[string]any{
			"id":            uri + "#list",
			"type":          "BitstringStatusList",
			"statusPurpose": "revocation",
			"encodedList":   encodedList,
		},
	}
}

func TestDataIntegrity_ECDSA_RoundTrip(t *testing.T) {
	privateKey := testECPrivateKey(t)
	uri := "http://127.0.0.1:28080/status/w3c/bitstring/di-ecdsa"
	document := sampleW3CDocument(uri, "uH4sIAAAAAAAAAAD6PwpGwSgYsQAQAAD//9T_OrgABAAA")

	signed, err := envelope.SignDataIntegrityCredential(document, envelope.ECDSASigner{
		PrivateKey: privateKey,
	}, "2024-06-23T00:00:00Z")
	require.NoError(t, err)

	raw, err := json.Marshal(signed)
	require.NoError(t, err)

	_, err = envelope.VerifyDataIntegrityCredential(raw, envelope.DataIntegrityVerifier{
		ResolveECDSA: func(issuer string) (*ecdsa.PublicKey, error) {
			require.Equal(t, "did:web:dev.example:issuer:poa", issuer)
			return &privateKey.PublicKey, nil
		},
	})
	require.NoError(t, err)
}

func TestEd25519Signer_Available(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	signer := envelope.Ed25519Signer{PrivateKey: priv}
	require.Equal(t, envelope.CryptosuiteEdDSARDFC2022, signer.Cryptosuite())
}
