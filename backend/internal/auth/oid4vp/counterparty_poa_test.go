package oid4vp

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
)

// poaFixture is a counterparty's Power of Attorney the way one arrives: a
// dc+sd-jwt credential with the organization selectively disclosed, key-bound
// to the signatory's own wallet key.
type poaFixture struct {
	Presentation string
	SignatoryDID string
}

func mintPoA(t *testing.T, issuerKey *ecdsa.PrivateKey, holderKey *ecdsa.PrivateKey, iss, organization string, extraClaims map[string]any) poaFixture {
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
		"status": map[string]any{
			"status_list": map[string]any{"uri": activeStatusList(t), "idx": 1},
		},
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

// activeStatusList serves a status list on which nothing is revoked.
func activeStatusList(t *testing.T) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(makeXFSCListBody(make([]byte, 16)))
	}))
	t.Cleanup(server.Close)
	return server.URL
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
// counterparty's issuer and what that issuer may speak for.
func peerTrust(t *testing.T, issuerKey *ecdsa.PrivateKey, purposes []Purpose, organizations []string) *TrustConfig {
	t.Helper()
	jwks, err := json.Marshal(map[string]any{"keys": []any{jwkMap(publicJWK(issuerKey))}})
	require.NoError(t, err)
	return &TrustConfig{
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
}

func TestVerifyCounterpartyPoA_Valid(t *testing.T) {
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{testPoAParty})

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
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, "did:web:stranger.example:issuer:poa", testPoAParty, nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{testPoAParty})

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
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposeLogin}, []string{testPoAParty})

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
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{"did:web:someone-else.example"})

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
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, "did:web:other-party.example", nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{OrganizationsAny})

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
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{testPoAParty})

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
	index := uint32(7)
	bitstring := make([]byte, 16)
	bitstring[index/8] |= 1 << (index % 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(makeXFSCListBody(bitstring))
	}))
	defer server.Close()

	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, map[string]any{
		"status": map[string]any{
			"status_list": map[string]any{"uri": server.URL, "idx": index},
		},
	})
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{testPoAParty})

	_, err := VerifyCounterpartyPoA(poa.Presentation, trust, CounterpartyPoAExpectation{
		Organization: testPoAParty,
		SignatoryDID: poa.SignatoryDID,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "revoked")
}

func TestVerifyCounterpartyPoA_ExpiredCredentialIsRefused(t *testing.T) {
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, map[string]any{
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{testPoAParty})

	_, err := VerifyCounterpartyPoA(poa.Presentation, trust, CounterpartyPoAExpectation{
		Organization: testPoAParty,
		SignatoryDID: poa.SignatoryDID,
	})
	require.Error(t, err)
	assert.Contains(t, strings.ToLower(err.Error()), "expired")
}

// Nothing to verify against is not an excuse to skip verification.
func TestVerifyCounterpartyPoA_WithoutTrustConfigOrExpectationIsRefused(t *testing.T) {
	issuerKey, holderKey := newECKey(t), newECKey(t)
	poa := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, nil)
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{testPoAParty})

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
