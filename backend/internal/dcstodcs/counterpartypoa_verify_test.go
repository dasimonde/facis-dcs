package dcstodcs

import (
	"crypto/ecdsa"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/auth/oid4vp"
)

func proofedPoA(proofMethod, proofPurpose string) SignatoryPoA {
	return SignatoryPoA{
		Party:        testSignedParty,
		Presentation: "p",
		Summary: `{
	  "type": ["VerifiableCredential", "ContractSigningSummaryCredential"],
	  "credentialSubject": {"id": "` + testSignedSignatory + `", "field_name": "` + testSignedParty + `", "contract_id": "` + testContract + `"},
	  "proof": {"type": "DataIntegrityProof", "verificationMethod": "` + proofMethod + `", "proofPurpose": "` + proofPurpose + `"}
	}`,
	}
}

// The one security control this gate rests on had no test at all: every case
// ran the branch where no verifier is configured.
func TestSigningEvidenceMustBeVerifiable(t *testing.T) {
	gate := CounterpartyPoAGate{Trust: &oid4vp.TrustConfig{}}
	err := gate.Check(testSignedParty, testContract, ShippedSignatures{}, []SignatoryPoA{poaFor(testSignedParty)})
	require.Error(t, err, "evidence with no means to verify it must be refused, not believed")
	assert.Contains(t, gateError(t, err).Error(), "no means to verify")
}

// The key is resolved from the method the PROOF names, and a peer that does not
// publish that method as one which may make assertions is refused. Deriving the
// id from our own key label instead only worked while every peer ran this
// software: DID Core puts no meaning in the fragment.
func TestSigningEvidenceKeyComesFromTheProofAndMustBeAuthorized(t *testing.T) {
	var seen []oid4vp.CounterpartyPoAExpectation
	gate := acceptingGate(&seen)

	shipped := verifier()
	var asked string
	shipped.ResolveKey = func(id string) (*ecdsa.PublicKey, error) {
		asked = id
		return nil, assertErr("not listed as an assertionMethod")
	}

	err := gate.Check(testSignedParty, testContract, shipped, []SignatoryPoA{proofedPoA(testSignedParty+"#whatever-this-peer-calls-it", "assertionMethod")})

	require.Error(t, err)
	assert.Equal(t, testSignedParty+"#whatever-this-peer-calls-it", asked,
		"the method to resolve must come from the proof, not from our own key label")
	assert.Contains(t, gateError(t, err).Error(), "not listed as an assertionMethod")
	assert.Empty(t, seen)
}

// A credential is an assertion; a proof made for another purpose does not
// establish one.
func TestSigningEvidenceMustProveAnAssertion(t *testing.T) {
	var seen []oid4vp.CounterpartyPoAExpectation
	gate := acceptingGate(&seen)

	err := gate.Check(testSignedParty, testContract, verifier(), []SignatoryPoA{proofedPoA(testSignedParty+"#dcs-vc", "authentication")})

	require.Error(t, err)
	assert.Contains(t, gateError(t, err).Error(), "not assertionMethod")
	assert.Empty(t, seen)
}

// A summary whose proof does not verify must stop the exchange before any of
// its claims are used.
func TestUnverifiableSigningEvidenceIsRefusedBeforeItsClaimsAreUsed(t *testing.T) {
	var seen []oid4vp.CounterpartyPoAExpectation
	gate := acceptingGate(&seen)

	shipped := verifier()
	shipped.VerifyVC = func(json.RawMessage, *ecdsa.PublicKey) error { return assertErr("bad signature") }

	err := gate.Check(testSignedParty, testContract, shipped, []SignatoryPoA{proofedPoA(testSignedParty+"#whatever-this-peer-calls-it", "assertionMethod")})

	require.Error(t, err)
	assert.True(t, strings.Contains(gateError(t, err).Error(), "does not verify"))
	assert.Empty(t, seen, "an unverified summary must never reach the Power-of-Attorney check")
}

type assertErr string

func (e assertErr) Error() string { return string(e) }

// The ordinary case still passes once the proof names the right key and purpose.
func TestVerifiedSigningEvidenceIsAccepted(t *testing.T) {
	var seen []oid4vp.CounterpartyPoAExpectation
	gate := acceptingGate(&seen)
	gate.Verify = func(_ string, _ *oid4vp.TrustConfig, e oid4vp.CounterpartyPoAExpectation) (*oid4vp.CounterpartyPoA, error) {
		seen = append(seen, e)
		return &oid4vp.CounterpartyPoA{}, nil
	}

	require.NoError(t, gate.Check(testSignedParty, testContract, verifier(), []SignatoryPoA{proofedPoA(testSignedParty+"#dcs-vc", "assertionMethod")}))
	require.Len(t, seen, 1)
	assert.Equal(t, testSignedSignatory, seen[0].SignatoryDID)
}
