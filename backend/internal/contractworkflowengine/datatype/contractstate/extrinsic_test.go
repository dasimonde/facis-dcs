package contractstate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	peerA = "did:web:dcs-a.localhost"
	peerB = "did:web:dcs-b.localhost"
)

func federated(signedLocally, peerSigners []string, localPeer string) SignatureEvidence {
	return SignatureEvidence{
		Declared:      []string{peerA, peerB},
		SignedLocally: signedLocally,
		Parties:       []string{peerA, peerB},
		LocalPeer:     localPeer,
		PeerSigners:   peerSigners,
	}
}

// The defect this closes: the first signatory's own instance reported the
// agreement executed the moment it signed, before the counterparty had
// countersigned anything (DCS-FR-SM-10, ADR-13). Nor is it agreed — our own
// signature is one party committing, and "agreed" claims both did.
func TestFirstOfTwoSignaturesIsNotExecuted(t *testing.T) {
	extrinsic := InferExtrinsic(Signed.String(), federated([]string{peerA}, nil, peerA))

	require.Equal(t, Proposed, extrinsic)
}

// The counterparty signs on ITS instance, so the evidence here is the
// cross-instance signature that peer shipped with its own signed copy.
func TestCountersignedContractIsExecuted(t *testing.T) {
	extrinsic := InferExtrinsic(Signed.String(), federated([]string{peerA}, []string{peerB}, peerA))

	require.Equal(t, Executed, extrinsic)
}

// The same holds on the counterparty's copy, where it signs its own slot and
// holds the originator's shipped signature.
func TestCountersignerAlsoReadsExecuted(t *testing.T) {
	extrinsic := InferExtrinsic(Signed.String(), federated([]string{peerB}, []string{peerA}, peerB))

	require.Equal(t, Executed, extrinsic)
}

// A shipped signature from a peer that is not the party whose slot is open
// says nothing about that slot — and so is no evidence the counterparty
// settled either.
func TestSignatureFromAnotherPeerDoesNotExecute(t *testing.T) {
	extrinsic := InferExtrinsic(Signed.String(),
		federated([]string{peerA}, []string{"did:web:dcs-c.localhost"}, peerA))

	require.Equal(t, Proposed, extrinsic)
}

// The defect this closes: a contract naming a counterparty could run
// Draft -> Submit -> Negotiation -> Submitted -> Reviewed -> Approved entirely
// inside the originating organization (transition.go; command/submit.go starts
// the round identically from DRAFT and OFFERED), and this projection then told
// the peer-facing world a bilateral agreement existed — on a contract signed by
// nobody that the counterparty had never received.
func TestApprovalAloneIsNotAgreement(t *testing.T) {
	extrinsic := InferExtrinsic(Approved.String(), federated(nil, nil, peerA))

	require.Equal(t, Proposed, extrinsic)
}

// Once the counterparty has committed, approval does say the parties settled.
func TestApprovalWithACounterpartySignatureIsAgreed(t *testing.T) {
	extrinsic := InferExtrinsic(Approved.String(), federated(nil, []string{peerB}, peerA))

	require.Equal(t, Agreed, extrinsic)
}

// A contract that declares no remote party has no counterparty to hear from,
// so its own approval is the whole settlement.
func TestApprovalOfANonFederatedContractIsAgreed(t *testing.T) {
	extrinsic := InferExtrinsic(Approved.String(), SignatureEvidence{
		Declared:  []string{"Signatory A", "Signatory B"},
		LocalPeer: peerA,
	})

	require.Equal(t, Agreed, extrinsic)
}

// A peer signature never stands in for this instance's OWN slot.
func TestOwnUnsignedSlotIsNotExecutedByAPeerSignature(t *testing.T) {
	extrinsic := InferExtrinsic(Signed.String(), federated(nil, []string{peerB}, peerA))

	require.Equal(t, Agreed, extrinsic)
}

// A contract with a single declared signatory is executed on that one
// signature — the single-signer flow must not be held open waiting for a
// second party that does not exist.
func TestSingleSignerIsExecutedOnItsOnlySignature(t *testing.T) {
	extrinsic := InferExtrinsic(Signed.String(), SignatureEvidence{
		Declared:      []string{"SignerOne"},
		SignedLocally: []string{"SignerOne"},
		LocalPeer:     peerA,
	})

	require.Equal(t, Executed, extrinsic)
}

// The single-instance multi-signer flow names its fields per signatory, and
// every one of them is signed here.
func TestSingleInstanceMultiSignerNeedsEveryLocalSignature(t *testing.T) {
	partial := SignatureEvidence{
		Declared:      []string{"SignerOne", "SignerTwo"},
		SignedLocally: []string{"SignerOne"},
		Parties:       []string{peerA, peerB},
		LocalPeer:     peerA,
		PeerSigners:   []string{peerB},
	}
	require.Equal(t, Agreed, InferExtrinsic(Signed.String(), partial))

	partial.SignedLocally = []string{"SignerOne", "SignerTwo"}
	require.Equal(t, Executed, InferExtrinsic(Signed.String(), partial))
}

// Three declared parties cannot be told apart by a store that keeps one
// cross-instance signature per contract, so the third party's slot stays
// unsatisfied: fail closed rather than claim an execution nobody evidenced.
func TestThirdPartySlotFailsClosed(t *testing.T) {
	extrinsic := InferExtrinsic(Signed.String(), SignatureEvidence{
		Declared:      []string{peerA, peerB, "did:web:dcs-c.localhost"},
		SignedLocally: []string{peerA},
		Parties:       []string{peerA, peerB},
		LocalPeer:     peerA,
		PeerSigners:   []string{peerB},
	})

	require.Equal(t, Agreed, extrinsic)
}

// ACTIVE is reachable only through the deployment gate, which already refuses
// a contract with an unsigned declared field.
func TestActiveIsExecuted(t *testing.T) {
	extrinsic := InferExtrinsic(Active.String(), federated([]string{peerA}, nil, peerA))

	require.Equal(t, Executed, extrinsic)
}

// A contract declaring no signature fields skips the deployment gate; the
// projection agrees with it rather than inventing a second rule.
func TestSignedWithoutDeclaredFieldsIsExecuted(t *testing.T) {
	extrinsic := InferExtrinsic(Signed.String(), SignatureEvidence{LocalPeer: peerA})

	require.Equal(t, Executed, extrinsic)
}

func TestPreSettlementStatesAreProposed(t *testing.T) {
	for _, state := range []ContractState{Draft, Offered, Negotiation, Submitted, Reviewed} {
		require.Equal(t, Proposed, InferExtrinsic(state.String(), SignatureEvidence{}), state)
	}
}

func TestApprovedIsAgreed(t *testing.T) {
	require.Equal(t, Agreed, InferExtrinsic(Approved.String(), SignatureEvidence{}))
}

// Off-ramps pass through so a caller can still tell why a contract left the
// happy path.
func TestOffRampsPassThrough(t *testing.T) {
	require.Equal(t, ExtrinsicLifecycle("revoked"), InferExtrinsic(Revoked.String(), SignatureEvidence{}))
	require.Equal(t, ExtrinsicLifecycle("withdrawn"), InferExtrinsic(Withdrawn.String(), SignatureEvidence{}))
}
