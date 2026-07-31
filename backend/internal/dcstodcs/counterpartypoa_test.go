package dcstodcs

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/auth/oid4vp"
)

const (
	testSignedParty     = "did:web:peer.example"
	testSignedSignatory = "did:jwk:eyJrdHkiOiJFQyJ9"
	testContract        = "urn:contract:1"
)

// summaryVC is a ContractSigningSummaryCredential as the shipping instance
// embeds it (DCS-FR-SM-08): field_name is the party the signature was made for,
// credentialSubject.id the signatory that made it.
func summaryVC(organization, signatory string) string {
	return `{
	  "@context": ["https://www.w3.org/ns/credentials/v2"],
	  "type": ["VerifiableCredential", "ContractSigningSummaryCredential"],
	  "issuer": "did:web:peer.example",
	  "credentialSubject": {
	    "id": "` + signatory + `",
	    "field_name": "` + organization + `",
	    "contract_id": "` + testContract + `"
	  },
	  "proof": {
	    "type": "DataIntegrityProof",
	    "verificationMethod": "` + testSignedParty + `#dcs-vc",
	    "proofPurpose": "assertionMethod"
	  }
	}`
}

// verifier accepts any summary, so the party-matching rules can be exercised
// for what they admit as well as what they refuse.
func verifier() ShippedSignatures {
	return ShippedSignatures{
		ResolveKey: func(string) (*ecdsa.PublicKey, error) { return nil, nil },
		VerifyVC:   func(json.RawMessage, *ecdsa.PublicKey) error { return nil },
	}
}

// poaFor is one shipped credential with the shipper's attestation of the
// signature it stands behind.
func poaFor(organization string) SignatoryPoA {
	signatory := testSignedSignatory
	if organization != testSignedParty {
		signatory = "did:jwk:" + organization
	}
	return SignatoryPoA{Party: organization, Presentation: "a-genuine-presentation", Summary: summaryVC(organization, signatory)}
}

func gateError(t *testing.T, err error) *GateError {
	t.Helper()
	require.Error(t, err)
	var gateErr *GateError
	require.True(t, errors.As(err, &gateErr), "a refusal must arrive as a GateError so it is recorded like any other trust denial")
	assert.Equal(t, PoAFailure, gateErr.Kind)
	assert.Equal(t, "did:web:peer.example", gateErr.PeerDID)
	return gateErr
}

// A peer that retains no Power-of-Attorney evidence still federates: absence is
// left to the compliance viewer, which reports a party that signed without one
// from the contract itself.
func TestCounterpartyPoAGate_AbsentEvidenceIsAccepted(t *testing.T) {
	gate := CounterpartyPoAGate{Trust: nil}
	require.NoError(t, gate.Check(testSignedParty, testContract, verifier(), nil))
}

// Present evidence that cannot be verified refuses the exchange rather than
// being ignored — including when this instance has no trust configuration to
// verify it against.
func TestCounterpartyPoAGate_EvidenceWithoutTrustConfigIsRefused(t *testing.T) {
	gate := CounterpartyPoAGate{Trust: nil}
	err := gate.Check(testSignedParty, testContract, verifier(), []SignatoryPoA{poaFor(testSignedParty)})
	assert.Contains(t, gateError(t, err).Error(), "no issuer trust is configured")
}

// A peer ships evidence for its OWN signatures; one naming another party is a
// credential from some other exchange.
func TestCounterpartyPoAGate_EvidenceForAnotherPartyIsRefusedEarly(t *testing.T) {
	gate := CounterpartyPoAGate{Trust: &oid4vp.TrustConfig{}}
	err := gate.Check(testSignedParty, testContract, verifier(), []SignatoryPoA{poaFor("did:web:the-other-party.example")})
	assert.Contains(t, gateError(t, err).Error(), "which is not its own")
}

// A summary attesting a different party's signature does not stand behind the
// credential shipped with it.
func TestCounterpartyPoAGate_SummaryForAnotherPartyIsRefused(t *testing.T) {
	gate := CounterpartyPoAGate{Trust: &oid4vp.TrustConfig{}}
	poa := poaFor(testSignedParty)
	poa.Summary = summaryVC("did:web:impostor.example", testSignedSignatory)
	err := gate.Check(testSignedParty, testContract, verifier(), []SignatoryPoA{poa})
	assert.Contains(t, gateError(t, err).Error(), "attests a signature for")
}

func TestCounterpartyPoAGate_UnreadableSigningEvidenceIsRefused(t *testing.T) {
	gate := CounterpartyPoAGate{Trust: &oid4vp.TrustConfig{}}
	poa := poaFor(testSignedParty)
	poa.Summary = "not json"
	err := gate.Check(testSignedParty, testContract, verifier(), []SignatoryPoA{poa})
	assert.Contains(t, gateError(t, err).Error(), "decode signing evidence")
}

func TestSignedPartyOf_ReadsTheShippersAttestation(t *testing.T) {
	party, err := signedPartyOf(verifier(), poaFor(testSignedParty), testSignedParty, testContract)
	require.NoError(t, err)
	assert.Equal(t, testSignedSignatory, party.Signatory)
	assert.Equal(t, testSignedParty, party.PoAOrganization)
}

// A Power of Attorney with no attestation of the signature it stands behind is
// refused: the shipper would be asserting the binding rather than proving it.
func TestPoAWithoutASummaryIsRefused(t *testing.T) {
	_, err := signedPartyOf(verifier(), SignatoryPoA{Party: testSignedParty, Presentation: "p"}, testSignedParty, testContract)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no signing summary")
}

// A summary attesting some other party's signature does not stand behind this
// credential.
func TestSummaryForAnotherPartyIsRefused(t *testing.T) {
	poa := poaFor(testSignedParty)
	poa.Summary = summaryVC("did:web:elsewhere.example", testSignedSignatory)
	_, err := signedPartyOf(verifier(), poa, testSignedParty, testContract)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "attests a signature for")
}

// acceptingGate stands in for the credential check so the party-matching rules
// can be exercised for what they ACCEPT. It records what the gate asked to be
// verified, which is the part that has to line up with the contract.
func acceptingGate(seen *[]oid4vp.CounterpartyPoAExpectation) CounterpartyPoAGate {
	return CounterpartyPoAGate{
		Trust: &oid4vp.TrustConfig{},
		Verify: func(_ string, _ *oid4vp.TrustConfig, expected oid4vp.CounterpartyPoAExpectation) (*oid4vp.CounterpartyPoA, error) {
			*seen = append(*seen, expected)
			return &oid4vp.CounterpartyPoA{Organization: expected.Organization, SignatoryDID: expected.SignatoryDID}, nil
		},
	}
}

// The join has to match the ordinary two-instance ship, where the signing
// instance's DID is both the organization the credential authorizes and the
// party IRI the contract carries. Nothing asserted this before, and a join that
// could never match shipped as a result.
func TestCounterpartyPoAGate_VerifiedEvidenceIsAccepted(t *testing.T) {
	var seen []oid4vp.CounterpartyPoAExpectation
	gate := acceptingGate(&seen)

	err := gate.Check(testSignedParty, testContract, verifier(), []SignatoryPoA{poaFor(testSignedParty)})
	require.NoError(t, err)

	require.Len(t, seen, 1, "the shipped credential must actually be verified, not skipped")
	assert.Equal(t, testSignedParty, seen[0].Organization)
	assert.Equal(t, testSignedSignatory, seen[0].SignatoryDID,
		"the credential is bound to the signatory the shipped contract records, not to one the peer names")
}

// A peer ships the evidence behind its OWN signatures. A credential for another
// party is one obtained in a different exchange: it would verify on its own
// merits, because nothing in the presentation names this contract.
func TestCounterpartyPoAGate_EvidenceForAnotherPartyIsRefused(t *testing.T) {
	var seen []oid4vp.CounterpartyPoAExpectation
	gate := acceptingGate(&seen)

	err := gate.Check("did:web:someone-else.example", testContract, verifier(), []SignatoryPoA{poaFor(testSignedParty)})

	require.Error(t, err)
	var gateErr *GateError
	require.True(t, errors.As(err, &gateErr))
	assert.Contains(t, gateErr.Error(), "which is not its own")
	assert.Empty(t, seen, "a credential for another party must be refused before it is verified")
}

// The return leg of a two-instance signing: A signs and ships, B signs on top
// and ships the double-signed contract back. It records BOTH parties as
// authorized, but B holds only its own presentation — the receive path verifies
// inbound evidence without retaining it — so B can only ever ship one.
//
// Requiring evidence for every authorized party made this ship impossible while
// leaving a peer that wants nothing verified free to send an empty list, so the
// requirement only ever refused honest peers.
func TestCounterpartyPoAGate_DoubleSignedReturnLegIsAccepted(t *testing.T) {
	var seen []oid4vp.CounterpartyPoAExpectation
	gate := acceptingGate(&seen)

	doubleSigned := verifier()

	require.NoError(t, gate.Check(testSignedParty, testContract, doubleSigned, []SignatoryPoA{poaFor(testSignedParty)}))

	require.Len(t, seen, 1, "the shipper's own evidence is still verified")
	assert.Equal(t, testSignedParty, seen[0].Organization)
	assert.Equal(t, testSignedSignatory, seen[0].SignatoryDID)
}

// An authored multi-signatory contract names its signature fields freely, so
// the organization a credential authorizes is not the party's own IRI. The join
// follows the authorization the contract records rather than the IRI.
func TestCounterpartyPoAGate_AuthoredFieldNamesStillJoin(t *testing.T) {
	var seen []oid4vp.CounterpartyPoAExpectation
	gate := acceptingGate(&seen)

	payload := verifier()

	require.NoError(t, gate.Check(testSignedParty, testContract, payload, []SignatoryPoA{poaFor("Acme Corp")}))
	require.Len(t, seen, 1)
	assert.Equal(t, "Acme Corp", seen[0].Organization)
}
