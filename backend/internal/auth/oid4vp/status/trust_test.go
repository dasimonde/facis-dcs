package status_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/base64"
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status"

	"github.com/stretchr/testify/require"
)

func TestTrustConfig_ResolveECDSAPublicKeyByKID_ScopedByURI(t *testing.T) {
	keyA, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	keyB, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	const kid = "12"
	trust := &status.TrustConfig{
		Issuers: map[string]status.TrustIssuerEntry{
			"http://provider-a.example": {JWKS: status.TrustJWKS{Keys: []map[string]any{
				ecJWK(t, keyA, kid),
			}}},
			"http://provider-b.example": {JWKS: status.TrustJWKS{Keys: []map[string]any{
				ecJWK(t, keyB, kid),
			}}},
		},
	}

	pub, err := trust.ResolveECDSAPublicKeyByKID("http://provider-a.example/statuslists/1", kid)
	require.NoError(t, err)
	require.Equal(t, &keyA.PublicKey, pub)

	pub, err = trust.ResolveECDSAPublicKeyByKID("http://provider-b.example/statuslists/9", kid)
	require.NoError(t, err)
	require.Equal(t, &keyB.PublicKey, pub)

	_, err = trust.ResolveECDSAPublicKeyByKID("", kid)
	require.Error(t, err)

	_, err = trust.ResolveECDSAPublicKeyByKID("http://unknown.example/statuslists/1", kid)
	require.Error(t, err)
}

func TestTrustConfig_ResolveECDSAPublicKeyByKID_RejectsUnrelatedURIWithUniqueKID(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	const kid = "12"
	trust := &status.TrustConfig{
		Issuers: map[string]status.TrustIssuerEntry{
			"http://provider-a.example": {JWKS: status.TrustJWKS{Keys: []map[string]any{
				ecJWK(t, key, kid),
			}}},
		},
	}

	_, err = trust.ResolveECDSAPublicKeyByKID("http://unrelated.example/statuslists/1", kid)
	require.Error(t, err)
	require.ErrorContains(t, err, "not trusted for status list URI")
}

func ecJWK(t *testing.T, key *ecdsa.PrivateKey, kid string) map[string]any {
	t.Helper()
	return map[string]any{
		"kty": "EC",
		"crv": "P-256",
		"kid": kid,
		"x":   base64.RawURLEncoding.EncodeToString(key.X.Bytes()),
		"y":   base64.RawURLEncoding.EncodeToString(key.Y.Bytes()),
	}
}
