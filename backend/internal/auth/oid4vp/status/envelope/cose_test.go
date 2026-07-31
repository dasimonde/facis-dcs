package envelope_test

import (
	"crypto/ecdsa"
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status/envelope"

	"github.com/stretchr/testify/require"
)

func TestCOSEVC_RoundTrip(t *testing.T) {
	privateKey := testECPrivateKey(t)
	uri := "http://127.0.0.1:28080/status/w3c/bitstring/signed-cose"
	document := sampleW3CDocument(uri, "uH4sIAAAAAAAAAAD6PwpGwSgYsQAQAAD//9T_OrgABAAA")

	signed, err := envelope.SignCOSEVC(document, privateKey, "application/vc+cose")
	require.NoError(t, err)

	_, err = envelope.VerifyCOSEVC(signed, envelope.COSEVerifier{
		ResolveECDSA: func(issuer string) (*ecdsa.PublicKey, error) {
			require.Equal(t, "did:web:dev.example:issuer:poa", issuer)
			return &privateKey.PublicKey, nil
		},
	})
	require.NoError(t, err)
}
