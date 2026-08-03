package oid4vp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/auth/oid4vp/sdjwt"
)

const (
	testPoAIssuer = "did:web:peer.example:issuer:poa"
	testPoAParty  = "did:web:peer.example"
	// testPoAStatusIndex is the index on the counterparty issuer's status list
	// that the fixture PoA is issued against.
	testPoAStatusIndex = 1
)

// poaFixture is a counterparty's Power of Attorney the way one arrives: a
// dc+sd-jwt credential with the organization selectively disclosed, key-bound
// to the signatory's own wallet key.
type poaFixture struct {
	Presentation string
	SignatoryDID string
}

func mintPoA(
	t *testing.T,
	issuerKey *ecdsa.PrivateKey,
	holderKey *ecdsa.PrivateKey,
	iss, organization string,
	statusList *xfscStatusList,
	extraClaims map[string]any,
) poaFixture {
	t.Helper()

	holderJWK := publicJWK(holderKey)
	signatory, err := sdjwt.DIDJWKFromPublicJWK(holderJWK)
	require.NoError(t, err)

	disclosure := encodeDisclosure(t, "organization", organization)

	claims := jwt.MapClaims{
		"iss":     iss,
		"sub":     signatory,
		"vct":     PoAVCT,
		"iat":     time.Now().Add(-time.Minute).Unix(),
		"exp":     time.Now().Add(time.Hour).Unix(),
		"roles":   []any{"Contract Signer"},
		"_sd":     []any{disclosureDigest(disclosure)},
		"_sd_alg": "sha-256",
		"cnf":     map[string]any{"jwk": jwkMap(holderJWK)},
		// A credential with no reachable status list is refused outright, so
		// every fixture carries one: revocation is not an optional step this
		// path can be exercised without.
		"status": statusList.Entry(testPoAStatusIndex),
	}
	for name, value := range extraClaims {
		claims[name] = value
	}

	issuerToken := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	issuerToken.Header["typ"] = sdjwt.CredentialTyp
	issuerJWT, err := issuerToken.SignedString(issuerKey)
	require.NoError(t, err)

	sdHash := sdjwt.SDHash(issuerJWT, []string{disclosure})
	kb := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iat":     time.Now().Unix(),
		"nonce":   "ceremony-nonce",
		"aud":     "https://the-signing-instance.example",
		"sd_hash": sdHash,
	})
	kb.Header["typ"] = sdjwt.KBJWTTyp
	kbJWT, err := kb.SignedString(holderKey)
	require.NoError(t, err)

	return poaFixture{
		Presentation: issuerJWT + "~" + disclosure + "~" + kbJWT,
		SignatoryDID: signatory,
	}
}

func publicJWK(key *ecdsa.PrivateKey) sdjwt.JWK {
	return sdjwt.JWK{
		Kty: "EC",
		Crv: "P-256",
		X:   base64.RawURLEncoding.EncodeToString(key.X.FillBytes(make([]byte, 32))),
		Y:   base64.RawURLEncoding.EncodeToString(key.Y.FillBytes(make([]byte, 32))),
	}
}

func jwkMap(key sdjwt.JWK) map[string]any {
	return map[string]any{"kty": key.Kty, "crv": key.Crv, "x": key.X, "y": key.Y}
}

func encodeDisclosure(t *testing.T, name string, value any) string {
	t.Helper()
	raw, err := json.Marshal([]any{"c2FsdA", name, value})
	require.NoError(t, err)
	return base64.RawURLEncoding.EncodeToString(raw)
}

func disclosureDigest(disclosure string) string {
	sum := sha256.Sum256([]byte(disclosure))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func newECKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	return key
}

// peerTrust is a receiving instance's trust configuration: it knows the
// counterparty's issuer, what that issuer may speak for, and the anchors the
// chain on that issuer's status list must verify against.
//
// It also wires the instance's status-list verifier off that same config, which
// is what ConfigureStatusListVerification does at startup — a config the
// verifier is not built from would leave the status list checked against
// nothing.
func peerTrust(
	t *testing.T,
	issuerKey *ecdsa.PrivateKey,
	purposes []Purpose,
	organizations []string,
	statusList *xfscStatusList,
) *TrustConfig {
	t.Helper()

	jwks, err := json.Marshal(map[string]any{"keys": []any{jwkMap(publicJWK(issuerKey))}})
	require.NoError(t, err)

	cfg := &TrustConfig{
		VCTs: []string{PoAVCT},
		Issuers: map[string]TrustedIssuer{
			testPoAIssuer: {
				Purposes:      purposes,
				Organizations: organizations,
				Mechanism:     MechanismJWKS,
				JWKS:          jwks,
			},
		},
	}
	cfg.SetX5CTrustRoots(statusList.Roots)

	require.NoError(t, ConfigureStatusListVerification(cfg))
	t.Cleanup(func() { _ = ConfigureStatusListVerification(nil) })

	return cfg
}

func TestVerifyCounterpartyPoA_Valid(t *testing.T) {
	list := newXFSCStatusList(t, 16)
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, list, nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{testPoAParty}, list)

	verified, err := VerifyCounterpartyPoA(poa.Presentation, trust, CounterpartyPoAExpectation{
		Organization: testPoAParty,
		SignatoryDID: poa.SignatoryDID,
	})
	require.NoError(t, err)
	assert.Equal(t, testPoAIssuer, verified.IssuerID)
	assert.Equal(t, testPoAParty, verified.Organization)
	assert.Equal(t, poa.SignatoryDID, verified.SignatoryDID)
	assert.Equal(t, []string{"Contract Signer"}, verified.Roles)
}

// A credential is only as good as the issuer behind it: one this instance never
// configured verifies against nothing, whatever its signature says.
func TestVerifyCounterpartyPoA_UnknownIssuerIsRefused(t *testing.T) {
	list := newXFSCStatusList(t, 16)
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, "did:web:stranger.example:issuer:poa", testPoAParty, list, nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{testPoAParty}, list)

	_, err := VerifyCounterpartyPoA(poa.Presentation, trust, CounterpartyPoAExpectation{
		Organization: testPoAParty,
		SignatoryDID: poa.SignatoryDID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not trusted")
}

// An issuer trusted to grant sessions here has not thereby been trusted to
// attest a counterparty's authority to sign: the purposes are separate grants.
func TestVerifyCounterpartyPoA_IssuerWithoutPeerPurposeIsRefused(t *testing.T) {
	list := newXFSCStatusList(t, 16)
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, list, nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposeLogin}, []string{testPoAParty}, list)

	_, err := VerifyCounterpartyPoA(poa.Presentation, trust, CounterpartyPoAExpectation{
		Organization: testPoAParty,
		SignatoryDID: poa.SignatoryDID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not trusted")
}

// An issuer may only speak for the organizations its own entry names, so a
// credential naming a party outside them is refused even though it verifies.
func TestVerifyCounterpartyPoA_OrganizationOutsideIssuerEntitlementIsRefused(t *testing.T) {
	list := newXFSCStatusList(t, 16)
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, list, nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{"did:web:someone-else.example"}, list)

	_, err := VerifyCounterpartyPoA(poa.Presentation, trust, CounterpartyPoAExpectation{
		Organization: testPoAParty,
		SignatoryDID: poa.SignatoryDID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not entitled to attest")
}

// The credential authorizes one party; a signature by another party is not
// covered by it.
func TestVerifyCounterpartyPoA_CredentialForAnotherPartyIsRefused(t *testing.T) {
	list := newXFSCStatusList(t, 16)
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, "did:web:other-party.example", list, nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{OrganizationsAny}, list)

	_, err := VerifyCounterpartyPoA(poa.Presentation, trust, CounterpartyPoAExpectation{
		Organization: testPoAParty,
		SignatoryDID: poa.SignatoryDID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not the signing party")
}

// Holder binding: the credential has to be held by the signatory the shipped
// contract records, or a peer could authorize its signature with somebody
// else's Power of Attorney.
func TestVerifyCounterpartyPoA_HolderIsNotTheRecordedSignatoryIsRefused(t *testing.T) {
	list := newXFSCStatusList(t, 16)
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, list, nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{testPoAParty}, list)

	_, err := VerifyCounterpartyPoA(poa.Presentation, trust, CounterpartyPoAExpectation{
		Organization: testPoAParty,
		SignatoryDID: "did:jwk:somebody-else",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is held by")
}

// A revoked Power of Attorney authorizes nothing, and the status list is
// checked on this path like on every other.
func TestVerifyCounterpartyPoA_RevokedCredentialIsRefused(t *testing.T) {
	list := newXFSCStatusList(t, 16)
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, list, nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{testPoAParty}, list)

	expectation := CounterpartyPoAExpectation{
		Organization: testPoAParty,
		SignatoryDID: poa.SignatoryDID,
	}

	// The same credential against the same list is accepted before the
	// revocation, so what the refusal below reports is the bit and not a list
	// that failed to load — with no unsigned fallback both look alike.
	_, err := VerifyCounterpartyPoA(poa.Presentation, trust, expectation)
	require.NoError(t, err)

	list.Revoke(testPoAStatusIndex)

	_, err = VerifyCounterpartyPoA(poa.Presentation, trust, expectation)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "revoked")
}

func TestVerifyCounterpartyPoA_ExpiredCredentialIsRefused(t *testing.T) {
	list := newXFSCStatusList(t, 16)
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, list, map[string]any{
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{testPoAParty}, list)

	_, err := VerifyCounterpartyPoA(poa.Presentation, trust, CounterpartyPoAExpectation{
		Organization: testPoAParty,
		SignatoryDID: poa.SignatoryDID,
	})
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "expired")
}

// Nothing to verify against is not an excuse to skip verification.
func TestVerifyCounterpartyPoA_WithoutTrustConfigOrExpectationIsRefused(t *testing.T) {
	list := newXFSCStatusList(t, 16)
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, list, nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{testPoAParty}, list)

	_, err := VerifyCounterpartyPoA(poa.Presentation, nil, CounterpartyPoAExpectation{
		Organization: testPoAParty,
		SignatoryDID: poa.SignatoryDID,
	})
	require.Error(t, err)

	_, err = VerifyCounterpartyPoA(poa.Presentation, trust, CounterpartyPoAExpectation{
		Organization: testPoAParty,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "records no signatory")

	_, err = VerifyCounterpartyPoA("", trust, CounterpartyPoAExpectation{
		Organization: testPoAParty,
		SignatoryDID: poa.SignatoryDID,
	})
	require.Error(t, err)
}
