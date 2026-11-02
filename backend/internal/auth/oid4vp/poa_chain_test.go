package oid4vp

import (
	"context"
	"crypto/ecdsa"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/auth/oid4vp/sdjwt"
	"digital-contracting-service/internal/auth/oid4vp/status"
)

// delegationLink is one credential of a delegation chain: the presentation a
// holder makes of it, and the key it binds.
type delegationLink struct {
	poaFixture
	key *ecdsa.PrivateKey
}

// carried is the credential as a delegated one carries it: without KB-JWT.
func (l delegationLink) carried() string {
	return l.Presentation[:strings.LastIndex(l.Presentation, "~")+1]
}

// delegationExpiry is the exp of every delegated link, so no link of a chain
// outlives the one above it.
var delegationExpiry = time.Now().Add(30 * time.Minute).Unix()

// delegate issues a Power of Attorney from the holder of parent to a new
// holder, signed with the parent's holder key. claims override the defaults.
func delegate(t *testing.T, parent delegationLink, statusList *xfscStatusList, index uint64, claims map[string]any) delegationLink {
	t.Helper()
	holderKey := newECKey(t)
	holderJWK := publicJWK(holderKey)
	signatory, err := sdjwt.DIDJWKFromPublicJWK(holderJWK)
	require.NoError(t, err)

	disclosure := encodeDisclosure(t, "organization", testPoAParty)
	payload := jwt.MapClaims{
		"iss":            parent.SignatoryDID,
		"sub":            signatory,
		"vct":            PoAVCT,
		"iat":            time.Now().Add(-time.Minute).Unix(),
		"exp":            delegationExpiry,
		"roles":          []any{"Contract Signer"},
		"_sd":            []any{disclosureDigest(disclosure)},
		"_sd_alg":        "sha-256",
		"cnf":            map[string]any{"jwk": jwkMap(holderJWK)},
		"status":         statusList.Entry(index),
		DelegatedByClaim: parent.carried(),
	}
	signingKey := parent.key
	for name, value := range claims {
		if name == "signing_key" {
			signingKey = value.(*ecdsa.PrivateKey)
			continue
		}
		if value == nil {
			delete(payload, name)
			continue
		}
		payload[name] = value
	}
	if disclosed, ok := claims["organization"]; ok {
		disclosure = encodeDisclosure(t, "organization", disclosed)
		payload["_sd"] = []any{disclosureDigest(disclosure)}
		delete(payload, "organization")
	}

	token := jwt.NewWithClaims(jwt.SigningMethodES256, payload)
	token.Header["typ"] = sdjwt.CredentialTyp
	issuerJWT, err := token.SignedString(signingKey)
	require.NoError(t, err)

	sdHash := sdjwt.SDHash(issuerJWT, []string{disclosure})
	kb := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{"iat": time.Now().Unix(), "nonce": "n", "aud": "a", "sd_hash": sdHash})
	kb.Header["typ"] = sdjwt.KBJWTTyp
	kbJWT, err := kb.SignedString(holderKey)
	require.NoError(t, err)

	return delegationLink{
		poaFixture: poaFixture{Presentation: issuerJWT + "~" + disclosure + "~" + kbJWT, SignatoryDID: signatory},
		key:        holderKey,
	}
}

type chainFixture struct {
	list   *xfscStatusList
	trust  *TrustConfig
	anchor delegationLink
}

// newChainFixture sets up an issuer trusted for `peer` and the Power of
// Attorney it issued to the organization's first delegator, at status index 1.
func newChainFixture(t *testing.T, anchorClaims map[string]any) chainFixture {
	t.Helper()
	list := newXFSCStatusList(t, 16, testPoAIssuer)
	issuerKey, holderKey := newECKey(t), newECKey(t)
	claims := map[string]any{MayDelegateClaim: true}
	for name, value := range anchorClaims {
		claims[name] = value
	}
	root := mintPoA(t, issuerKey, holderKey, testPoAIssuer, testPoAParty, list, claims)
	trust := peerTrust(t, issuerKey, []Purpose{PurposePeer}, []string{testPoAParty}, list)
	return chainFixture{list: list, trust: trust, anchor: delegationLink{poaFixture: root, key: holderKey}}
}

func (f chainFixture) verify(link delegationLink) (*CounterpartyPoA, error) {
	return VerifyCounterpartyPoA(link.Presentation, f.trust, CounterpartyPoAExpectation{
		Organization: testPoAParty,
		SignatoryDID: link.SignatoryDID,
	})
}

// DCS-FR-SM-04: a counterparty's Power of Attorney delegated by the holder of
// one the organization's issuer issued verifies link by link to that issuer.
func TestVerifyCounterpartyPoA_DelegationChainVerifiesToTheTrustedIssuer(t *testing.T) {
	f := newChainFixture(t, nil)
	middle := delegate(t, f.anchor, f.list, 2, map[string]any{MayDelegateClaim: true})
	leaf := delegate(t, middle, f.list, 3, nil)

	verified, err := f.verify(leaf)
	require.NoError(t, err)
	assert.Equal(t, leaf.SignatoryDID, verified.SignatoryDID)
	assert.Equal(t, middle.SignatoryDID, verified.IssuerID)
	assert.Equal(t, testPoAParty, verified.Organization)
	assert.Equal(t, []string{"Contract Signer"}, verified.Roles)
}

func TestVerifyCounterpartyPoA_DelegationChainIsRefusedPerLink(t *testing.T) {
	stranger := newECKey(t)
	for name, tc := range map[string]struct {
		anchor map[string]any
		claims map[string]any
		want   string
	}{
		"delegator may not delegate":  {anchor: map[string]any{MayDelegateClaim: nil}, want: "does not permit delegation"},
		"signed by another key":       {claims: map[string]any{"signing_key": stranger}, want: "delegated power of attorney"},
		"issuer is not the delegator": {claims: map[string]any{"iss": "did:jwk:someone-else"}, want: "not its delegator"},
		"wider organization":          {claims: map[string]any{"organization": "did:web:other.example"}, want: "its delegator only"},
		"wider roles":                 {claims: map[string]any{"roles": []any{"Contract Signer", "Contract Manager"}}, want: "does not hold"},
		"outlives the delegator":      {claims: map[string]any{"exp": time.Now().Add(2 * time.Hour).Unix()}, want: "expires"},
		"expired link":                {claims: map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}, want: "expired"},
		"no status":                   {claims: map[string]any{"status": nil}, want: "status"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newChainFixture(t, tc.anchor)
			leaf := delegate(t, f.anchor, f.list, 2, tc.claims)

			_, err := f.verify(leaf)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
		})
	}
}

func TestVerifyCounterpartyPoA_DelegationChainDepthIsLimited(t *testing.T) {
	f := newChainFixture(t, nil)
	link := f.anchor
	for i := range MaxDelegationDepth {
		link = delegate(t, link, f.list, uint64(2+i), map[string]any{MayDelegateClaim: true})
	}
	_, err := f.verify(link)
	require.NoError(t, err)

	_, err = f.verify(delegate(t, link, f.list, 15, nil))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "longer than")
}

// Revoking any link revokes everything delegated below it: the anchor
// credential on its issuer's list, a delegated link on the list of the issuer
// that anchors its chain.
func TestVerifyCounterpartyPoA_RevokedLinkRefusesTheChain(t *testing.T) {
	for name, index := range map[string]uint64{"anchor": testPoAStatusIndex, "delegated link": 2} {
		t.Run(name, func(t *testing.T) {
			f := newChainFixture(t, nil)
			middle := delegate(t, f.anchor, f.list, 2, map[string]any{MayDelegateClaim: true})
			leaf := delegate(t, middle, f.list, 3, nil)
			_, err := f.verify(leaf)
			require.NoError(t, err)

			f.list.Revoke(index)

			_, err = f.verify(leaf)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "revoked")

			state, err := PresentedCredentialState(context.Background(), leaf.Presentation, f.trust, PurposePeer)
			require.NoError(t, err)
			assert.Equal(t, status.StateInvalid, state)
		})
	}
}
