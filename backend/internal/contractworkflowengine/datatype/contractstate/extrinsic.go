package contractstate

import (
	"strings"

	"digital-contracting-service/internal/base/identity"
)

// ExtrinsicLifecycle is the peer-facing, INFERRED negotiation lifecycle a
// contract presents across the DCS-to-DCS boundary (ADR-13). It is never
// stored: each instance derives it from its own intrinsic state plus the
// signature evidence it holds — its own SIGNED signature rows and the
// cross-instance signature the counterparty shipped with its signed copy.
//
// The two instances therefore agree on the value that matters — "executed"
// requires evidence for EVERY declared signature field, which neither side has
// until both have signed — but not on the pre-signing phases, which are
// projections of local workflow facts (a contract offered by A is OFFERED on
// both copies, but A may already have it approved internally while B has not
// looked at it). ADR-13 states the derivation is possible from the shared
// artifact alone; this projection does not read the artifact.
//
// It is distinct from both the intrinsic (local RBAC) state and the C2PA banner
// (which stays SRS draft/active): the negotiation phases live here.
type ExtrinsicLifecycle string

const (
	// Proposed: a version is on the table and being negotiated (offer +
	// counteroffers). This is the whole pre-settlement negotiation.
	Proposed ExtrinsicLifecycle = "proposed"
	// Agreed: both parties settled/consolidated on the same version. The
	// signing gate opens here, and the contract stays agreed while signatures
	// are being collected.
	Agreed ExtrinsicLifecycle = "agreed"
	// Executed: every signature field the contract declares is satisfied by
	// signature evidence this instance holds (DCS-FR-SM-10; ADR-13: "two
	// signatures over the settled content is the executed agreement").
	Executed ExtrinsicLifecycle = "executed"
)

// SignatureEvidence is what one instance holds about who has signed a
// contract. It is the single place the question "have all declared signatures
// been collected" is answered: the extrinsic projection below reads it to tell
// a half-signed contract from an executed one, and the deployment gate
// (contractworkflowengine/command/deploy.go) reads it to decide whether the
// contract may deploy — so a contract reads "executed" exactly when it would be
// allowed to deploy, because it is one predicate and not two.
//
// A declared field is satisfied by a local SIGNED signature row, or — for a
// field naming a counterparty — by the cross-instance signature that peer
// shipped with its own signed copy, which it ships only once that copy is
// SIGNED and which is verified against the peer's published assertion key on
// receipt.
//
// Only one cross-instance signature is stored per contract
// (contract_sync_signatures is keyed by contract, with no field column), so
// this satisfies at most one remote party and leaves any further remote party
// unsigned for want of evidence: sound for two parties, deliberately
// fail-closed beyond them.
type SignatureEvidence struct {
	// Declared are the contract document's signature field names
	// (validation.RequiredSignatureFields). Slots of a federated contract are
	// named by the signing party's DID; the single-instance multi-signer flow
	// names them per signatory instead.
	Declared []string
	// SignedLocally are the fields carrying a SIGNED signature row here.
	SignedLocally []string
	// Parties are the contract's two party DIDs (creator and counterparty). A
	// declared field naming neither is not a remote party's slot.
	Parties []string
	// LocalPeer is this instance's own did:web, which distinguishes our slot
	// from the counterparty's.
	LocalPeer string
	// PeerSigners are the peer DIDs this instance holds a verified
	// cross-instance signature from.
	PeerSigners []string
}

// Unsigned returns the declared signature fields no held evidence satisfies.
func (e SignatureEvidence) Unsigned() []string {
	signed := make(map[string]bool, len(e.SignedLocally))
	for _, f := range e.SignedLocally {
		signed[f] = true
	}

	var missing []string
	for _, f := range e.Declared {
		if signed[f] {
			continue
		}
		if e.isRemotePartyField(f) && e.peerSigned(f) {
			continue
		}
		missing = append(missing, f)
	}
	return missing
}

// AllSignaturesCollected reports whether every declared signature field is
// satisfied. A contract declaring no signature fields collects nothing and so
// is trivially complete, matching the deployment gate, which it skips.
func (e SignatureEvidence) AllSignaturesCollected() bool {
	return len(e.Unsigned()) == 0
}

func (e SignatureEvidence) isRemotePartyField(field string) bool {
	return IsRemotePartyField(e.Parties, e.LocalPeer, field)
}

// IsRemotePartyField reports whether a declared signature field belongs to a
// contract party other than this instance. Slots are named by the signing
// party's DID (dcs:signatoryName), so a slot naming the counterparty is one
// whose signature is produced and recorded in that peer's own deployment and
// never reaches ours. A field naming no party of this contract — the
// single-instance multi-signer flow names its fields per signatory — is never
// remote, and neither is a contract that records no parties at all.
//
// This answers only WHOSE slot a field is. What each caller owes a remote slot
// differs and must not be folded together: the deployment gate still demands
// the peer's shipped signature for it (Unsigned above), while the signing
// ceremony gate (signingmanagement/command/apply.go) exempts it outright,
// because a peer ships that signature only once its own copy is SIGNED and
// demanding it at first-signature time would deadlock federated signing.
func IsRemotePartyField(parties []string, localPeer, field string) bool {
	if localPeer == "" || field == "" || identity.SameDIDWeb(field, localPeer) {
		return false
	}
	for _, party := range parties {
		if party != "" && party == field {
			return true
		}
	}
	return false
}

// peerSigned reports whether a cross-instance signature from the party this
// field names is held.
func (e SignatureEvidence) peerSigned(field string) bool {
	for _, signer := range e.PeerSigners {
		if signer != "" && identity.SameDIDWeb(signer, field) {
			return true
		}
	}
	return false
}

// counterpartyCommitted reports whether this instance holds evidence that the
// OTHER party settled on this version: a verified cross-instance signature for
// the slot naming it. A contract that declares no remote party slot has no
// counterparty to hear from, so its own workflow is the whole story.
//
// Local workflow progress is never such evidence. Approval, review and our own
// signature are all things this organization does alone, and a contract can
// reach every one of them without the peer having received the document —
// Draft -> Submit -> Negotiation needs no counterparty (transition.go), and
// command/submit.go starts the negotiation round identically from DRAFT and
// OFFERED.
func (e SignatureEvidence) counterpartyCommitted() bool {
	remote := false
	for _, field := range e.Declared {
		if !e.isRemotePartyField(field) {
			continue
		}
		remote = true
		if e.peerSigned(field) {
			return true
		}
	}
	return !remote
}

// InferExtrinsic projects the extrinsic lifecycle from a contract's intrinsic
// state and the signature evidence this instance holds. The pre-settlement
// formation states all read as "proposed". Off-ramps pass through as their
// lowercase state so a caller can still tell why a contract left the happy
// path.
//
// "Agreed" asserts that BOTH parties settled on this version, so it is claimed
// only where this instance holds evidence of the counterparty's commitment.
// APPROVED alone is not that evidence: it is one organization's internal
// decision, reachable without the counterparty ever receiving the contract, and
// projecting it as agreement told a peer a bilateral agreement existed on the
// strength of a unilateral act. The same applies to our own first signature.
//
// SIGNED is written on the FIRST local signature, so it does not by itself say
// the agreement is executed. ACTIVE is only ever reached through the deployment
// gate, which already refuses a contract with an unsigned declared field.
func InferExtrinsic(intrinsicState string, signatures SignatureEvidence) ExtrinsicLifecycle {
	switch strings.ToUpper(intrinsicState) {
	case Draft.String(), Offered.String(), Negotiation.String(), Submitted.String(), Reviewed.String():
		return Proposed
	case Approved.String():
		if signatures.counterpartyCommitted() {
			return Agreed
		}
		return Proposed
	case Signed.String():
		if signatures.AllSignaturesCollected() {
			return Executed
		}
		if signatures.counterpartyCommitted() {
			return Agreed
		}
		return Proposed
	case Active.String():
		return Executed
	default:
		return ExtrinsicLifecycle(strings.ToLower(intrinsicState))
	}
}
