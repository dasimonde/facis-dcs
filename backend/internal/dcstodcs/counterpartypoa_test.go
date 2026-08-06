package dcstodcs

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/auth/oid4vp"
	"digital-contracting-service/internal/pdfgeneration/provenance"
)

const (
	testSignedParty     = "did:web:peer.example"
	testLocalParty      = "did:web:us.example"
	testSignedSignatory = "did:jwk:eyJrdHkiOiJFQyJ9"
	testContract        = "urn:contract:1"
)

// summaryVC is a ContractSigningSummaryCredential as the signing instance embeds
// it (DCS-FR-SM-08): field_name is the party the signature was made for,
// credentialSubject.id the signatory that made it.
func summaryVC(organization, signatory string) string {
	return `{
	  "@context": ["https://www.w3.org/ns/credentials/v2"],
	  "type": ["VerifiableCredential", "ContractSigningSummaryCredential"],
	  "issuer": "` + organization + `",
	  "credentialSubject": {
	    "id": "` + signatory + `",
	    "field_name": "` + organization + `",
	    "contract_id": "` + testContract + `"
	  },
	  "proof": {
	    "type": "DataIntegrityProof",
	    "verificationMethod": "` + organization + `#dcs-vc",
	    "proofPurpose": "assertionMethod"
	  }
	}`
}

// verifier accepts any summary, so the field-ownership rules can be exercised
// for what they admit as well as what they refuse.
func verifier() ShippedSignatures {
	return ShippedSignatures{
		ResolveKey: func(string, string) (*ecdsa.PublicKey, error) { return nil, nil },
		VerifyVC:   func(json.RawMessage, *ecdsa.PublicKey) error { return nil },
	}
}

// attachment is one embedded evidence document as ExtractEvidence hands it back.
func attachment(summary, presentation string) json.RawMessage {
	raw, err := json.Marshal(provenance.SigningEvidenceAttachment{
		Summary:         json.RawMessage(summary),
		PoAPresentation: presentation,
	})
	if err != nil {
		panic(err)
	}
	return raw
}

// evidenceFor is the attachment a party embedded before its own signature.
func evidenceFor(organization string) json.RawMessage {
	signatory := testSignedSignatory
	if organization != testSignedParty {
		signatory = "did:jwk:" + organization
	}
	return attachment(summaryVC(organization, signatory), "a-genuine-presentation")
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

// A PDF whose signatures carry no Power-of-Attorney evidence still federates:
// absence is left to the compliance viewer, which reports a party that signed
// without one from the contract itself.
func TestCounterpartyPoAGate_AbsentEvidenceIsAccepted(t *testing.T) {
	gate := CounterpartyPoAGate{Trust: nil}
	require.NoError(t, gate.Check(testSignedParty, testLocalParty, testContract, verifier(), nil))
}

// Present evidence that cannot be verified refuses the exchange rather than
// being ignored — including when this instance has no trust configuration to
// verify it against.
func TestCounterpartyPoAGate_EvidenceWithoutTrustConfigIsRefused(t *testing.T) {
	gate := CounterpartyPoAGate{Trust: nil}
	err := gate.Check(testSignedParty, testLocalParty, testContract, verifier(), []json.RawMessage{evidenceFor(testSignedParty)})
	assert.Contains(t, gateError(t, err).Error(), "no issuer trust is configured")
}

// An attachment that is not a signing summary attests nothing, and its empty
// fields must not read as an unattested signature.
func TestCounterpartyPoAGate_NonSummaryEvidenceIsRefused(t *testing.T) {
	gate := CounterpartyPoAGate{Trust: &oid4vp.TrustConfig{}}
	err := gate.Check(testSignedParty, testLocalParty, testContract, verifier(),
		[]json.RawMessage{attachment(`{"type":["VerifiableCredential"]}`, "p")})
	assert.Contains(t, gateError(t, err).Error(), "not a signing summary")
}

func TestCounterpartyPoAGate_UnreadableSigningEvidenceIsRefused(t *testing.T) {
	gate := CounterpartyPoAGate{Trust: &oid4vp.TrustConfig{}}
	err := gate.Check(testSignedParty, testLocalParty, testContract, verifier(),
		[]json.RawMessage{attachment(`"not an object"`, "p")})
	assert.Contains(t, gateError(t, err).Error(), "decode signing summary")
}

// acceptingGate stands in for the credential check so the field-ownership rules
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
// field its summary names.
func TestCounterpartyPoAGate_VerifiedEvidenceIsAccepted(t *testing.T) {
	var seen []oid4vp.CounterpartyPoAExpectation
	gate := acceptingGate(&seen)

	err := gate.Check(testSignedParty, testLocalParty, testContract, verifier(), []json.RawMessage{evidenceFor(testSignedParty)})
	require.NoError(t, err)

	require.Len(t, seen, 1, "the embedded credential must actually be verified, not skipped")
	assert.Equal(t, testSignedParty, seen[0].Organization)
	assert.Equal(t, testSignedSignatory, seen[0].SignatoryDID,
		"the credential is bound to the signatory the embedded summary attests, not to one the peer names")
}

// The summary must verify against the key of the instance that OWNS the field,
// not against the shipper's. That is what stops a peer embedding evidence for a
// party it has nothing to do with: it cannot make a proof under that party's key.
func TestCounterpartyPoAGate_ASummaryIsCheckedAgainstItsFieldsOwner(t *testing.T) {
	var seen []oid4vp.CounterpartyPoAExpectation
	gate := acceptingGate(&seen)

	var asked []string
	shipped := verifier()
	shipped.ResolveKey = func(owner, _ string) (*ecdsa.PublicKey, error) {
		asked = append(asked, owner)
		return nil, nil
	}

	require.NoError(t, gate.Check(testSignedParty, testLocalParty, testContract, shipped,
		[]json.RawMessage{evidenceFor("did:web:third-party.example")}))
	assert.Equal(t, []string{"did:web:third-party.example"}, asked,
		"the key must be resolved from the instance the field names, not from the shipper")
}

// The return leg of a two-instance signing: A signs and ships, B countersigns
// and ships back the PDF carrying BOTH parties' evidence. A verifies B's
// credential, and re-checks its own summary against its own key without
// re-running a Power-of-Attorney check that was a `login` question here.
func TestCounterpartyPoAGate_DoubleSignedReturnLegVerifiesBothSides(t *testing.T) {
	var seen []oid4vp.CounterpartyPoAExpectation
	gate := acceptingGate(&seen)

	var asked []string
	shipped := verifier()
	shipped.ResolveKey = func(owner, _ string) (*ecdsa.PublicKey, error) {
		asked = append(asked, owner)
		return nil, nil
	}

	require.NoError(t, gate.Check(testSignedParty, testLocalParty, testContract, shipped,
		[]json.RawMessage{evidenceFor(testLocalParty), evidenceFor(testSignedParty)}))

	assert.Equal(t, []string{testLocalParty, testSignedParty}, asked,
		"both attachments are verified, each against the key of the instance that issued it")
	require.Len(t, seen, 1, "only the peer's Power of Attorney is peer evidence; our own ceremony answered login")
	assert.Equal(t, testSignedParty, seen[0].Organization)
}

// Our own field is still checked for authenticity: a peer that fabricates a
// summary under our DID cannot make a proof our own key verifies.
func TestCounterpartyPoAGate_OurOwnFieldIsStillVerifiedAgainstOurKey(t *testing.T) {
	var seen []oid4vp.CounterpartyPoAExpectation
	gate := acceptingGate(&seen)

	shipped := verifier()
	shipped.VerifyVC = func(json.RawMessage, *ecdsa.PublicKey) error { return assertErr("bad signature") }

	err := gate.Check(testSignedParty, testLocalParty, testContract, shipped, []json.RawMessage{evidenceFor(testLocalParty)})
	assert.Contains(t, gateError(t, err).Error(), "does not verify")
	assert.Empty(t, seen)
}

// An authored multi-signatory contract names its signature fields freely, so the
// organization a credential authorizes is not any party's did:web IRI. Such a
// field can only have been summarised by the shipper, so the shipper's key is
// what its summary is held to.
func TestCounterpartyPoAGate_AuthoredFieldNamesAreHeldToTheShipper(t *testing.T) {
	var seen []oid4vp.CounterpartyPoAExpectation
	gate := acceptingGate(&seen)

	var asked []string
	shipped := verifier()
	shipped.ResolveKey = func(owner, _ string) (*ecdsa.PublicKey, error) {
		asked = append(asked, owner)
		return nil, nil
	}

	require.NoError(t, gate.Check(testSignedParty, testLocalParty, testContract, shipped, []json.RawMessage{evidenceFor("Acme Corp")}))
	require.Len(t, seen, 1)
	assert.Equal(t, "Acme Corp", seen[0].Organization)
	assert.Equal(t, []string{testSignedParty}, asked)
}

// Every attachment is checked, not just the newest for a field: an embed appends
// rather than replacing, so a second attachment for an already-verified field
// would otherwise ride along unexamined.
func TestCounterpartyPoAGate_EveryAttachmentForAFieldIsVerified(t *testing.T) {
	var seen []oid4vp.CounterpartyPoAExpectation
	gate := acceptingGate(&seen)
	gate.Verify = func(presentation string, _ *oid4vp.TrustConfig, expected oid4vp.CounterpartyPoAExpectation) (*oid4vp.CounterpartyPoA, error) {
		seen = append(seen, expected)
		if presentation == "an-unverifiable-presentation" {
			return nil, assertErr("issuer is not trusted for peer")
		}
		return &oid4vp.CounterpartyPoA{}, nil
	}

	err := gate.Check(testSignedParty, testLocalParty, testContract, verifier(), []json.RawMessage{
		evidenceFor(testSignedParty),
		attachment(summaryVC(testSignedParty, testSignedSignatory), "an-unverifiable-presentation"),
	})

	assert.Contains(t, gateError(t, err).Error(), "issuer is not trusted for peer")
}

// A signature whose ceremony had no Power of Attorney still federates; the
// compliance viewer reports it.
func TestCounterpartyPoAGate_EvidenceWithoutAPresentationIsAccepted(t *testing.T) {
	var seen []oid4vp.CounterpartyPoAExpectation
	gate := acceptingGate(&seen)

	require.NoError(t, gate.Check(testSignedParty, testLocalParty, testContract, verifier(),
		[]json.RawMessage{attachment(summaryVC(testSignedParty, testSignedSignatory), "")}))
	assert.Empty(t, seen)
}
