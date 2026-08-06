package dcstodcs

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"strings"

	"digital-contracting-service/internal/auth/oid4vp"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/pdfgeneration/provenance"
)

// CounterpartyPoAGate verifies the Power-of-Attorney evidence embedded in the
// PDF a peer ships (ADR-31, ADR-35): the counterparty's side of the mutual
// binding.
//
// Evidence that is present and does not verify refuses the exchange, like any
// other trust-gate denial. Evidence that is ABSENT does not: a peer whose PDF
// carries none still federates, and a party that signed without a Power of
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

// ShippedSignatures is the means to check the signing summaries embedded in a
// shipped PDF against the key of whichever instance issued each one.
type ShippedSignatures struct {
	// VerifyVC checks one summary against an issuer's VC key. Required: without
	// it the summary is somebody telling us who signed, which is what the
	// contract payload already was. Absent, the gate denies rather than
	// accepting unverified claims — the same way a missing trust configuration
	// denies rather than waving credentials through.
	VerifyVC func(vc json.RawMessage, key *ecdsa.PublicKey) error
	// ResolveKey turns (the instance that owns the signed field, the
	// verification method a proof names) into the key to check it with,
	// refusing one that instance does not publish for assertions.
	ResolveKey func(ownerDID, verificationMethodID string) (*ecdsa.PublicKey, error)
}

// Check verifies every signing evidence attachment the shipped PDF carries.
//
// A countersigned PDF carries BOTH parties' evidence — each party embeds its own
// before applying its own signature — so "the shipper must own it" is not the
// rule. What binds each attachment instead is the key it verifies against: the
// summary must be issued by the instance that OWNS the field it attests, and
// nothing a shipper can forge reaches another instance's assertion key.
//
// localDID is this instance's own identity. Evidence for a field this instance
// owns is verified against our own key and its Power of Attorney is NOT
// re-checked as peer evidence: that ceremony ran here, where the credential is a
// `login` question and was answered as one (ADR-35).
//
// EVERY attachment is checked. A field may legitimately appear more than once —
// each embed appends rather than replacing — and taking only the newest would
// let an appended attachment for an already-verified field decide nothing while
// still being carried by the contract.
func (g *CounterpartyPoAGate) Check(peerDID, localDID, contractIRI string, signed ShippedSignatures, evidence []json.RawMessage) error {
	if len(evidence) == 0 {
		return nil
	}

	deny := func(err error) error {
		return &GateError{Kind: PoAFailure, PeerDID: peerDID, Err: err}
	}

	if g.Trust == nil {
		return deny(fmt.Errorf("counterparty Power of Attorney: no issuer trust is configured, so nothing shipped can be verified"))
	}
	if signed.VerifyVC == nil || signed.ResolveKey == nil {
		return deny(fmt.Errorf("counterparty Power of Attorney: no means to verify the embedded signing evidence, so nothing it claims can be believed"))
	}

	for _, raw := range evidence {
		attachment, subject, err := provenance.ReadSigningEvidence(raw)
		if err != nil {
			return deny(fmt.Errorf("counterparty Power of Attorney: %w", err))
		}
		// The credential authorizes an organization, and the summary names the
		// field the signature was made for. A ceremony refuses unless the Power
		// of Attorney authorizes exactly that field
		// (signingmanagement/command/ceremony.go), so the field IS the
		// organization the credential must authorize. That holds for both
		// contract shapes: an auto-seeded field is named for the signing
		// instance's DID, so field and party IRI coincide, while an authored
		// multi-signatory contract names its fields freely.
		organization := subject.FieldName
		if organization == "" {
			return deny(fmt.Errorf("counterparty Power of Attorney: the shipped PDF carries a signing summary naming no signature field"))
		}
		// The owner of a did:web field is the instance that DID names; anything
		// else is a field of an authored contract, which only the shipper can
		// have summarised.
		owner := strings.TrimSpace(peerDID)
		if strings.HasPrefix(organization, "did:web:") {
			owner = organization
		}
		if err := verifySummary(signed, attachment.Summary, owner, organization); err != nil {
			return deny(fmt.Errorf("counterparty Power of Attorney: %w", err))
		}
		// Without this the evidence is not bound to this exchange at all. The
		// presentation carries no audience or nonce we can check, so the
		// summary's own contract_id is what stops a genuine (summary, Power of
		// Attorney) pair from another contract being replayed onto this one.
		if subject.ContractID != strings.TrimSpace(contractIRI) {
			return deny(fmt.Errorf("counterparty Power of Attorney: the signing summary embedded for %q attests a signature on contract %q, not %q",
				organization, subject.ContractID, contractIRI))
		}
		if subject.Signatory == "" {
			return deny(fmt.Errorf("counterparty Power of Attorney: the signing summary embedded for %q names no signatory", organization))
		}
		if identity.SameDIDWeb(organization, strings.TrimSpace(localDID)) {
			continue
		}
		if attachment.PoAPresentation == "" {
			continue
		}
		if _, err := g.verify()(attachment.PoAPresentation, g.Trust, oid4vp.CounterpartyPoAExpectation{
			Organization: organization,
			SignatoryDID: subject.Signatory,
		}); err != nil {
			return deny(fmt.Errorf("counterparty Power of Attorney for party %q: %w", organization, err))
		}
	}

	return nil
}

// verifySummary checks one summary credential before anything it says is used.
//
// The proof must name a verification method the OWNER of the signed field
// publishes for VC signing. Verifying against a key without checking which key
// the proof claims lets an issuer present a proof made with another of its
// published keys and have it checked against the one we happened to resolve.
func verifySummary(signed ShippedSignatures, summary json.RawMessage, owner, organization string) error {
	var envelope struct {
		Proof struct {
			VerificationMethod string `json:"verificationMethod"`
			ProofPurpose       string `json:"proofPurpose"`
		} `json:"proof"`
	}
	if err := json.Unmarshal(summary, &envelope); err != nil {
		return fmt.Errorf("decode signing evidence proof for %q: %w", organization, err)
	}
	key, err := signed.ResolveKey(owner, envelope.Proof.VerificationMethod)
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
	if err := signed.VerifyVC(summary, key); err != nil {
		return fmt.Errorf("signing evidence for %q does not verify against the key of %q: %w", organization, owner, err)
	}
	return nil
}
