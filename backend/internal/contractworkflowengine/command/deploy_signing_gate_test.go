package command

import (
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/contractworkflowengine/db"
	db2 "digital-contracting-service/internal/dcstodcs/db"
)

// The gate's rule — which declared signature fields are still unsigned — is
// contractstate.SignatureEvidence.Unsigned, shared with the extrinsic lifecycle
// projection. What these cases pin is that the deployment path reaches it with
// the right inputs: the contract's parties, this instance's own peer identity,
// and the one cross-instance signature it holds.
const (
	peerA = "did:web:dcs-a.localhost"
	peerB = "did:web:dcs-b.localhost"
)

func federatedParties() *db.Responsible {
	return &db.Responsible{Creator: peerA, Counterparty: peerB}
}

func peerSignature(fromPeer string) *db2.SyncSignature {
	return &db2.SyncSignature{DID: "did:web:example#contract", FromPeerDID: fromPeer, ContractVersion: 1}
}

// The defect this closes: one party could deploy — and so activate — a
// two-party contract the counterparty had never even received, because a slot
// naming the counterparty was skipped without asking whether that counterparty
// had signed anything (DCS-NFR-BR-03).
func TestHalfSignedFederatedContractHasAnUnsignedField(t *testing.T) {
	missing := signatureEvidence(
		[]string{peerA, peerB}, []string{peerA}, federatedParties(), peerA, nil).Unsigned()

	require.Equal(t, []string{peerB}, missing)
}

// The counterparty's signature row lives in the counterparty's database, so the
// evidence this instance gates on is the JAdES that peer shipped with its own
// signed copy.
func TestCountersignedFederatedContractHasNoUnsignedField(t *testing.T) {
	missing := signatureEvidence(
		[]string{peerA, peerB}, []string{peerA}, federatedParties(), peerA, peerSignature(peerB)).Unsigned()

	require.Empty(t, missing)
}

// The same holds on the counterparty's own copy, where the parties are recorded
// the other way round: it signs its own slot and holds the originator's JAdES.
func TestCountersignerDeploysOnTheOriginatorsShippedSignature(t *testing.T) {
	missing := signatureEvidence(
		[]string{peerA, peerB}, []string{peerB}, federatedParties(), peerB, peerSignature(peerA)).Unsigned()

	require.Empty(t, missing)
}

// A stored signature from someone else says nothing about the party whose slot
// is open.
func TestASignatureFromAnotherPeerDoesNotSatisfyThisPartysField(t *testing.T) {
	missing := signatureEvidence(
		[]string{peerA, peerB}, []string{peerA}, federatedParties(), peerA, peerSignature("did:web:dcs-c.localhost")).Unsigned()

	require.Equal(t, []string{peerB}, missing)
}

// The exemption is for the OTHER party's slot only: this instance's own
// signature is never stood in for by anything a peer shipped.
func TestOwnFieldIsNeverSatisfiedByAPeerSignature(t *testing.T) {
	missing := signatureEvidence(
		[]string{peerA, peerB}, nil, federatedParties(), peerA, peerSignature(peerB)).Unsigned()

	require.Equal(t, []string{peerA}, missing)
}

// The single-instance multi-signer flow names its fields per signatory, not per
// party, and every one of them is signed here. A peer signature must not stand
// in for a signatory nobody else can sign for.
func TestSingleInstanceMultiSignerStillRequiresEveryLocalSignature(t *testing.T) {
	missing := signatureEvidence(
		[]string{"SignerOne", "SignerTwo"}, []string{"SignerOne"}, federatedParties(), peerA, peerSignature(peerB)).Unsigned()

	require.Equal(t, []string{"SignerTwo"}, missing)
}

func TestFullySignedMultiSignerContractHasNoUnsignedField(t *testing.T) {
	missing := signatureEvidence(
		[]string{"SignerOne", "SignerTwo"}, []string{"SignerOne", "SignerTwo"}, federatedParties(), peerA, nil).Unsigned()

	require.Empty(t, missing)
}

// A peer's own spelling of its DID need not match the field name character for
// character — DNS is case-insensitive — so party identity is compared as
// did:web rather than as text.
func TestPeerIdentityIsComparedAsDIDWebNotAsText(t *testing.T) {
	missing := signatureEvidence(
		[]string{peerA, peerB}, []string{peerA}, federatedParties(), peerA,
		peerSignature("did:web:DCS-B.localhost")).Unsigned()

	require.Empty(t, missing)
}

// A contract with no recorded parties has no remote slot to exempt.
func TestWithoutRecordedPartiesEveryFieldIsLocal(t *testing.T) {
	missing := signatureEvidence(
		[]string{peerA, peerB}, []string{peerA}, nil, peerA, peerSignature(peerB)).Unsigned()

	require.Equal(t, []string{peerB}, missing)
}

// An instance that does not know its own DID cannot tell its slot from the
// counterparty's, so it treats every slot as its own and refuses: the gate
// fails closed rather than exempting a slot it cannot attribute. This is why
// the auto-deploy subscriber must pass LocalPeer — left empty there, a fully
// countersigned federated contract could never auto-deploy.
func TestWithoutThisInstancesOwnIdentityTheGateFailsClosed(t *testing.T) {
	missing := signatureEvidence(
		[]string{peerA, peerB}, []string{peerA}, federatedParties(), "", peerSignature(peerB)).Unsigned()

	require.Equal(t, []string{peerB}, missing)
}
