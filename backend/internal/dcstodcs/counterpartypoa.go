package dcstodcs

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/auth/oid4vp"
	"digital-contracting-service/internal/base/identity"
	smdb "digital-contracting-service/internal/signingmanagement/db"

	dcstodcs "digital-contracting-service/gen/dcs_to_dcs"
)

// SignatoryPoA is the Power-of-Attorney evidence behind one applied signature,
// on the wire between two instances: the party the credential authorizes and
// the presentation the signatory's wallet delivered at the ceremony.
type SignatoryPoA struct {
	Party        string
	Presentation string
	// Summary is the signing summary credential the shipping instance issued
	// for this signature, its attestation of who signed for that party.
	Summary string
}

// SignatoryPoAs reads the evidence retained for the signatures an instance
// applied to a contract.
type SignatoryPoAs interface {
	ForContract(ctx context.Context, contractIRI string) ([]SignatoryPoA, error)
}

// CeremonyPoAs is the production SignatoryPoAs: the credential is retained on
// the signing ceremony that consumed it, next to the PID presentation.
type CeremonyPoAs struct {
	DB           *sqlx.DB
	CeremonyRepo smdb.CeremonyRepo
}

func (c *CeremonyPoAs) ForContract(ctx context.Context, contractIRI string) ([]SignatoryPoA, error) {
	tx, err := c.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	applied, err := c.CeremonyRepo.ListAppliedPoAs(ctx, tx, contractIRI)
	if err != nil {
		return nil, err
	}

	evidence := make([]SignatoryPoA, 0, len(applied))
	for _, poa := range applied {
		evidence = append(evidence, SignatoryPoA{Party: poa.FieldName, Presentation: poa.Presentation, Summary: poa.SummaryVC})
	}
	return evidence, nil
}

// WireSignatoryPoAs converts retained evidence to the ship payload.
func WireSignatoryPoAs(evidence []SignatoryPoA) []*dcstodcs.DCSToDCSSignatoryPoA {
	if len(evidence) == 0 {
		return nil
	}
	wire := make([]*dcstodcs.DCSToDCSSignatoryPoA, 0, len(evidence))
	for _, poa := range evidence {
		wire = append(wire, &dcstodcs.DCSToDCSSignatoryPoA{Party: poa.Party, Presentation: poa.Presentation, Summary: summaryOrNil(poa.Summary)})
	}
	return wire
}

// ReceivedSignatoryPoAs converts a ship payload back to evidence.
func ReceivedSignatoryPoAs(wire []*dcstodcs.DCSToDCSSignatoryPoA) []SignatoryPoA {
	if len(wire) == 0 {
		return nil
	}
	evidence := make([]SignatoryPoA, 0, len(wire))
	for _, poa := range wire {
		if poa == nil {
			continue
		}
		evidence = append(evidence, SignatoryPoA{Party: poa.Party, Presentation: poa.Presentation, Summary: derefSummary(poa.Summary)})
	}
	return evidence
}

func summaryOrNil(summary string) *string {
	if summary == "" {
		return nil
	}
	return &summary
}

func derefSummary(summary *string) string {
	if summary == nil {
		return ""
	}
	return *summary
}

// CounterpartyPoAGate verifies the Power-of-Attorney evidence a peer ships with
// a contract (ADR-31): the counterparty's side of the mutual binding.
//
// Evidence that is present and does not verify refuses the exchange, like any
// other trust-gate denial. Evidence that is ABSENT does not: a peer that
// retains none still federates, and a party that signed without a Power of
// Attorney is raised by the compliance viewer from the contract itself
// (signingmanagement/command/compliance.go), which is where that finding has
// always come from.
type CounterpartyPoAGate struct {
	Trust *oid4vp.TrustConfig
	// Verify is the credential check, defaulting to oid4vp.VerifyCounterpartyPoA.
	// Held as a field so the party-matching rules below can be exercised for
	// what they accept as well as what they refuse: with the real verifier they
	// are only reachable by minting a genuine credential, and the acceptance
	// path went untested long enough to ship a join that never matched.
	Verify func(presentation string, trust *oid4vp.TrustConfig, expected oid4vp.CounterpartyPoAExpectation) (*oid4vp.CounterpartyPoA, error)
}

func (g *CounterpartyPoAGate) verify() func(string, *oid4vp.TrustConfig, oid4vp.CounterpartyPoAExpectation) (*oid4vp.CounterpartyPoA, error) {
	if g.Verify != nil {
		return g.Verify
	}
	return oid4vp.VerifyCounterpartyPoA
}

// ShippedSignatures is what the peer's own signing evidence says about the
// signatures on the contract it shipped: the ContractSigningSummaryCredential(s)
// embedded in the PDF (DCS-FR-SM-08), and the means to verify them against the
// peer's key.
type ShippedSignatures struct {
	// VerifyVC checks one summary against the peer's VC key. Required: without
	// it the summary is the peer telling us who signed, which is what the
	// contract payload already was. Absent, the gate denies rather than
	// accepting unverified claims — the same way a missing trust configuration
	// denies rather than waving credentials through.
	VerifyVC func(vc json.RawMessage, key *ecdsa.PublicKey) error
	// ResolveKey turns the verification method a proof names into the key to
	// check it with, refusing one the peer does not publish for assertions.
	ResolveKey func(verificationMethodID string) (*ecdsa.PublicKey, error)
}

// Check verifies each shipped Power of Attorney against the peer's own signing
// evidence.
//
// The attribution is read from the signing summary VC, not from dcs:parties in
// the contract payload. The payload embedded in a signed PDF is pinned before
// the wallet signs it, so the signatory and the authority — recorded when the
// signature is applied — are not in it and cannot be: writing them there would
// change the bytes the signature covers. DCS-FR-SM-08 already requires the
// summary as a VC embedded in the PDF/A-3, which is issuer-signed by the
// shipping instance rather than being a bare assertion beside the credential.
func (g *CounterpartyPoAGate) Check(peerDID, contractIRI string, signed ShippedSignatures, evidence []SignatoryPoA) error {
	if len(evidence) == 0 {
		return nil
	}

	deny := func(err error) error {
		return &GateError{Kind: PoAFailure, PeerDID: peerDID, Err: err}
	}

	if g.Trust == nil {
		return deny(fmt.Errorf("counterparty Power of Attorney: no issuer trust is configured, so nothing shipped can be verified"))
	}

	verified := make(map[string]bool, len(evidence))
	for _, poa := range evidence {
		// The credential authorizes an organization, and the contract records
		// which party that organization authorized. Joining on that recorded
		// authorization rather than on the party's own IRI is what makes this
		// work for both contract shapes: an auto-seeded signature field is named
		// for the signing instance's DID, so organization and party IRI coincide,
		// while an authored multi-signatory contract names its fields freely and
		// the two differ.
		organization := strings.TrimSpace(poa.Party)
		// Each credential arrives with the shipper's own attestation of the
		// signature it stands behind. Reading it from the PDF instead meant
		// reading whichever attachment happened to be last, which after a
		// countersignature is the other party's.
		node, err := signedPartyOf(signed, poa, organization, contractIRI)
		if err != nil {
			return deny(fmt.Errorf("counterparty Power of Attorney: %w", err))
		}
		party := organization
		// A peer ships the evidence behind the signatures IT applied. Evidence
		// for anyone else is a credential obtained in some other exchange being
		// replayed here: the presentation carries no audience or nonce this
		// instance could check, so without this it would verify on its own merits
		// and vouch for a party the shipper has nothing to do with.
		//
		// Only a did:web organization can be held against the peer's identity. An
		// authored contract may name its parties anything, and there the issuer's
		// entitlement to attest that organization is the only bound there is.
		if strings.HasPrefix(organization, "did:web:") && !identity.SameDIDWeb(organization, strings.TrimSpace(peerDID)) {
			return deny(fmt.Errorf("counterparty Power of Attorney: peer %q shipped evidence for %q, which is not its own",
				peerDID, organization))
		}
		if _, err := g.verify()(poa.Presentation, g.Trust, oid4vp.CounterpartyPoAExpectation{
			Organization: organization,
			SignatoryDID: node.Signatory,
		}); err != nil {
			return deny(fmt.Errorf("counterparty Power of Attorney for party %q: %w", party, err))
		}
		verified[party] = true
	}

	// Deliberately NOT required: evidence for every party the contract records as
	// authorized. A contract signed on both instances carries two such parties,
	// each authorized by a different peer, and neither peer holds the other's
	// presentation — the receive path verifies inbound evidence without retaining
	// it. Demanding all of it makes the return leg of every two-instance signing
	// unshippable, while a peer that wants nothing checked still just sends an
	// empty list. It would only ever fire against an honest peer.
	return nil
}

// signedParty is a party node of a shipped contract that carries a signature.
type signedParty struct {
	Signatory       string
	PoAOrganization string
}

// signedPartyOf reads what the shipper attests about ONE signature: which party
// it was made for, and by whom.
//
// A ceremony refuses unless the Power of Attorney authorizes exactly the party
// the signature field names (signingmanagement/command/ceremony.go), so the
// summary's field_name IS the organization its Power of Attorney must
// authorize, and credentialSubject.id is the signatory it must be held by.
func signedPartyOf(signed ShippedSignatures, poa SignatoryPoA, organization, contractIRI string) (signedParty, error) {
	if signed.VerifyVC == nil || signed.ResolveKey == nil {
		return signedParty{}, fmt.Errorf("no means to verify the peer's signing evidence, so nothing it claims can be believed")
	}
	raw := json.RawMessage(strings.TrimSpace(poa.Summary))
	if len(raw) == 0 {
		return signedParty{}, fmt.Errorf("the ship carries a Power of Attorney for %q with no signing summary attesting the signature it stands behind", organization)
	}

	var vc struct {
		Type              []string `json:"type"`
		CredentialSubject struct {
			ID         string `json:"id"`
			FieldName  string `json:"field_name"`
			ContractID string `json:"contract_id"`
		} `json:"credentialSubject"`
	}
	if err := json.Unmarshal(raw, &vc); err != nil {
		return signedParty{}, fmt.Errorf("decode signing evidence for %q: %w", organization, err)
	}
	if !slices.Contains(vc.Type, "ContractSigningSummaryCredential") {
		return signedParty{}, fmt.Errorf("the evidence shipped for %q is not a signing summary", organization)
	}
	if err := verifySummary(signed, raw, organization); err != nil {
		return signedParty{}, err
	}
	// Without this the evidence is not bound to this exchange at all. The
	// presentation carries no audience or nonce we can check, so the summary's
	// own contract_id is what stops a genuine (summary, Power of Attorney) pair
	// from another contract being replayed onto this one.
	if subject := strings.TrimSpace(vc.CredentialSubject.ContractID); subject != strings.TrimSpace(contractIRI) {
		return signedParty{}, fmt.Errorf("the signing summary shipped for %q attests a signature on contract %q, not %q",
			organization, subject, contractIRI)
	}
	if strings.TrimSpace(vc.CredentialSubject.FieldName) != organization {
		return signedParty{}, fmt.Errorf("the signing summary shipped for %q attests a signature for %q instead",
			organization, vc.CredentialSubject.FieldName)
	}
	signatory := strings.TrimSpace(vc.CredentialSubject.ID)
	if signatory == "" {
		return signedParty{}, fmt.Errorf("the signing summary for %q names no signatory", organization)
	}
	return signedParty{Signatory: signatory, PoAOrganization: organization}, nil
}

// verifySummary checks one summary credential before anything it says is used.
//
// The proof must name the verification method this peer publishes for VC
// signing. Verifying against a key without checking which key the proof claims
// lets a peer present a proof made with another of its published keys and have
// it checked against the one we happened to resolve.
func verifySummary(signed ShippedSignatures, raw json.RawMessage, organization string) error {
	var envelope struct {
		Proof struct {
			VerificationMethod string `json:"verificationMethod"`
			ProofPurpose       string `json:"proofPurpose"`
		} `json:"proof"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("decode signing evidence proof for %q: %w", organization, err)
	}
	// The proof says which key made it; the peer's document says whether that key
	// may make assertions. Guessing the id from our own key label instead worked
	// only while every peer ran this software — DID Core puts no meaning in the
	// fragment, so an interoperable peer names its keys whatever it likes.
	key, err := signed.ResolveKey(envelope.Proof.VerificationMethod)
	if err != nil {
		return fmt.Errorf("signing evidence for %q: %w", organization, err)
	}
	// A credential is an assertion; a proof made for any other purpose does not
	// establish one, and proofPurpose is mandatory (W3C VC Data Integrity §2.1),
	// so an omitted one is a malformed proof rather than a permissive default —
	// which is what it was, and it let a proof made to authenticate or to agree a
	// key pass as an assertion by simply leaving the field out.
	if purpose := strings.TrimSpace(envelope.Proof.ProofPurpose); purpose != string(identity.PurposeAssertion) {
		return fmt.Errorf("signing evidence for %q carries a proof for %q, not %s", organization, purpose, identity.PurposeAssertion)
	}
	if err := signed.VerifyVC(raw, key); err != nil {
		return fmt.Errorf("signing evidence for %q does not verify against the peer's key: %w", organization, err)
	}
	return nil
}
