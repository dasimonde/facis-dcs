package oid4vp

import (
	"encoding/json"
	"os"
	"testing"

	"digital-contracting-service/internal/auth/oid4vp/status"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	_ = ConfigureStatusListVerification(nil)
	os.Exit(m.Run())
}

// The status-list verifier is built from the trust config already parsed for
// OID4VP, not from a second read of the same file. Only issuers carrying a
// bundled JWKS can sign a status list: an issuer whose key is resolved by x5c,
// did:web or ORCE has no key to check a list signature against, so it must be
// absent from the projection rather than present and unusable.
func TestConfigureStatusListVerificationTakesOnlyIssuersWithABundledKey(t *testing.T) {
	trustCfg := &TrustConfig{Issuers: map[string]TrustedIssuer{
		"did:web:example:issuer:bundled": {
			Mechanism: MechanismJWKS,
			JWKS:      json.RawMessage(`{"keys":[{"kty":"EC","crv":"P-256","x":"f83OJ3D2xF1Bg8vub9tLe1gHMzV76e8Tus9uPHvRVEU","y":"x_FEzRu9m36HLN_tue659LNpXW6pCyStikYjKIWI5a0","kid":"bundled"}]}`),
		},
		"did:web:example:issuer:x5c": {
			Mechanism: MechanismX5C,
		},
	}}

	require.NoError(t, ConfigureStatusListVerification(trustCfg))
	t.Cleanup(func() { _ = ConfigureStatusListVerification(nil) })

	projected, err := status.NewTrustConfig(map[string]json.RawMessage{
		"did:web:example:issuer:bundled": trustCfg.Issuers["did:web:example:issuer:bundled"].JWKS,
		"did:web:example:issuer:x5c":     trustCfg.Issuers["did:web:example:issuer:x5c"].JWKS,
	})
	require.NoError(t, err)
	assert.Contains(t, projected.Issuers, "did:web:example:issuer:bundled")
	assert.NotContains(t, projected.Issuers, "did:web:example:issuer:x5c")
}

// Every status-list issuer publishing by certificate is the deployed
// arrangement, not a broken one — our own issuers bundle no key at all, and
// TestCheckStatusList_IETFStatusList_Active runs on exactly that. What is broken
// is neither a bundled key nor an anchor: every status list then resolves to no
// key and every credential is refused, which belongs at startup rather than at
// the first login.
func TestConfigureStatusListVerificationRefusesAConfigThatCanVerifyNothing(t *testing.T) {
	err := ConfigureStatusListVerification(&TrustConfig{
		Issuers: map[string]TrustedIssuer{"did:web:example:issuer:x5c": {Mechanism: MechanismX5C}},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no x5c trust anchors are configured")
}

// The deployed shape end to end: a statuslist+jwt carrying an x5c chain, which
// SelectMechanismFromResponse routes to handler.IETFToken. Nothing else in this
// package covers that combination, and it is the one a login credential's status
// list takes on every instance (ADR-34).
func TestCheckStatusList_IETFStatusList_Active(t *testing.T) {
	const index = 62073

	list := newIETFStatusList(t, 125000)
	require.NotEmpty(t, list.chain, "the default fixture must sign by chain, not by bundled key")
	trustIETFStatusList(t, list)

	claims, err := json.Marshal(map[string]any{"status": list.Entry(index)})
	require.NoError(t, err)
	require.NoError(t, checkStatusList(claims))
}

func TestCheckStatusList_IETFStatusList_Revoked(t *testing.T) {
	const index = 3

	list := newIETFStatusList(t, 16)
	trustIETFStatusList(t, list)

	claims, err := json.Marshal(map[string]any{"status": list.Entry(index)})
	require.NoError(t, err)

	// The same credential against the same list is accepted before the
	// revocation, so what the refusal below reports is the bit and not a list
	// that failed to load — with no unsigned fallback both look alike.
	require.NoError(t, checkStatusList(claims))

	list.Revoke(index)

	err = checkStatusList(claims)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "revoked")
}

// A chain proves the anchor vouched for the certificate, not whose certificate
// it is. The issuer's boot path used to mint its leaf before the public URL was
// knowable, so the leaf carried no SAN at all — it chained perfectly and
// identified nobody, and every status list it signed was refused on an instance
// that had the right anchor mounted. The leaf is now minted from the request's
// own URL; this is what would fail if it went back to being minted at boot.
func TestCheckStatusList_IETFStatusList_RefusesALeafThatNamesNoIssuer(t *testing.T) {
	const index = 3

	list := newIETFStatusList(t, 16, leafNamingNoIssuer(t))
	trustIETFStatusList(t, list)

	claims, err := json.Marshal(map[string]any{"status": list.Entry(index)})
	require.NoError(t, err)

	err = checkStatusList(claims)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "revoked",
		"an unusable list must not read as a revocation: the bit was never set")
}

// The other branch of the same handler. An issuer that publishes no certificate
// is verified against the key bundled in the trust document, and the anchors are
// absent — so this cannot be passing on the chain the case above supplies.
func TestCheckStatusList_IETFStatusList_BundledKeyIssuer(t *testing.T) {
	const index = 9

	list := newIETFStatusList(t, 16, keyByJWKS)
	require.Empty(t, list.chain)
	trustIETFStatusList(t, list)

	claims, err := json.Marshal(map[string]any{"status": list.Entry(index)})
	require.NoError(t, err)
	require.NoError(t, checkStatusList(claims))

	list.Revoke(index)
	err = checkStatusList(claims)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "revoked")
}
