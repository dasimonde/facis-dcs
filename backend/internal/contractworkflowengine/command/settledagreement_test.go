package command

import (
	"context"
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/jades"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/contractworkflowengine/negotiationmerging"
	db2 "digital-contracting-service/internal/dcstodcs/db"
	"digital-contracting-service/internal/pdfgeneration/provenance"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
)

const (
	settledContractDID = "did:web:example:contract:1"
	thisInstance       = "did:web:dcs-a.example"
	theCounterparty    = "did:web:dcs-b.example"
	agreedDocument     = `{"dcs:documentStructure":{"dcs:clause":"the agreed terms"}}`
)

type settledContractRepoFake struct {
	db.ContractRepo
	pdfState db.ContractPDFState
	document string
}

func (r *settledContractRepoFake) ReadPDFState(context.Context, *sqlx.Tx, string) (*db.ContractPDFState, error) {
	state := r.pdfState
	return &state, nil
}

func (r *settledContractRepoFake) ReadDataByDID(_ context.Context, _ *sqlx.Tx, did string) (*db.Contract, error) {
	contract := &db.Contract{DID: did}
	if r.document != "" {
		document := datatype.JSON(r.document)
		contract.ContractData = &document
	}
	return contract, nil
}

// settlementStoreFake stands in for contract_settlements, keyed by the settling
// party the way the table is.
type settlementStoreFake struct {
	db2.SyncRepository
	rows map[string]*db2.Settlement
	// queued mirrors contract_settlement_withdrawals: what the peer holding a
	// settlement will be told, keyed by the audience it is addressed to.
	queued map[string]db2.SettlementWithdrawal
}

func (s *settlementStoreFake) GetSettlement(_ context.Context, _ *sqlx.Tx, _, fromPeerDID string) (*db2.Settlement, error) {
	return s.rows[fromPeerDID], nil
}

func (s *settlementStoreFake) GetSettlementsBy(_ context.Context, _ *sqlx.Tx, _, fromPeerDID string) ([]db2.Settlement, error) {
	if row := s.rows[fromPeerDID]; row != nil {
		return []db2.Settlement{*row}, nil
	}
	return nil, nil
}

func (s *settlementStoreFake) DeleteSettlementsBy(_ context.Context, _ *sqlx.Tx, _, fromPeerDID string) error {
	delete(s.rows, fromPeerDID)
	return nil
}

func (s *settlementStoreFake) UpsertSettlementWithdrawal(_ context.Context, _ *sqlx.Tx, w db2.SettlementWithdrawal) error {
	s.queued[w.ToPeerDID] = w
	return nil
}

// settledOn records the settlement a party ships on closing its negotiation
// round, over the same digest dcstodcs.BuildSettlement binds.
func settledOn(t *testing.T, store *settlementStoreFake, party, document string) {
	t.Helper()
	digest, err := jades.ContractDocumentDigest([]byte(document))
	require.NoError(t, err)
	store.rows[party] = &db2.Settlement{
		DID:            settledContractDID,
		FromPeerDID:    party,
		ToPeerDID:      theCounterparty,
		DocumentDigest: digest,
	}
}

func unsignedArtifactOf(document string) *settledContractRepoFake {
	return &settledContractRepoFake{
		pdfState: db.ContractPDFState{IPFSCID: "cid", C2PAState: "draft"},
		document: document,
	}
}

func emptyStore() *settlementStoreFake {
	return &settlementStoreFake{
		rows:   map[string]*db2.Settlement{},
		queued: map[string]db2.SettlementWithdrawal{},
	}
}

// The window this gate exists for: both parties agreed, nobody has signed yet,
// and this instance tries to rewrite the very document it agreed to.
func TestRenegotiationIsRefusedOnceThisInstanceSettledTheStoredDocument(t *testing.T) {
	repo := unsignedArtifactOf(agreedDocument)
	store := emptyStore()
	settledOn(t, store, thisInstance, agreedDocument)

	err := requireUnsettledAgreement(context.Background(), nil, repo, store, thisInstance, settledContractDID)

	require.ErrorIs(t, err, ErrOwnAgreementSettled)
	// Nothing is signed, so the refusal must not be the one that says it is.
	require.NotErrorIs(t, err, ErrAgreementSettled)
}

// Before this instance settles, the counteroffer ping-pong is the whole point
// of the negotiation round and must stay open.
func TestRenegotiationIsPermittedBeforeThisInstanceSettles(t *testing.T) {
	repo := unsignedArtifactOf(agreedDocument)

	require.NoError(t, requireUnsettledAgreement(context.Background(), nil, repo, emptyStore(), thisInstance, settledContractDID))
}

// Only this instance's own agreement binds it. A counterparty that settled while
// this one is still negotiating does not freeze this copy's document.
func TestRenegotiationIsPermittedWhileOnlyTheCounterpartyHasSettled(t *testing.T) {
	repo := unsignedArtifactOf(agreedDocument)
	store := emptyStore()
	settledOn(t, store, theCounterparty, agreedDocument)

	require.NoError(t, requireUnsettledAgreement(context.Background(), nil, repo, store, thisInstance, settledContractDID))
}

// The settlement binds one version. Once the counterparty redlined and shipped a
// different document, what this instance agreed to is not what it holds, and
// refusing here would wedge a contract the counterparty is still negotiating.
func TestRenegotiationIsPermittedWhenTheSettledVersionIsNoLongerTheStoredOne(t *testing.T) {
	repo := unsignedArtifactOf(`{"dcs:documentStructure":{"dcs:clause":"the counterparty's redline"}}`)
	store := emptyStore()
	settledOn(t, store, thisInstance, agreedDocument)

	require.NoError(t, requireUnsettledAgreement(context.Background(), nil, repo, store, thisInstance, settledContractDID))
}

// The way forward for a party that wants a further change: the rejection edges
// (SUBMITTED -> NEGOTIATION, REVIEWED -> REJECTED) withdraw the agreement, and
// the document is editable again.
func TestRenegotiationIsPermittedAfterWithdrawingTheAgreement(t *testing.T) {
	repo := unsignedArtifactOf(agreedDocument)
	store := emptyStore()
	settledOn(t, store, thisInstance, agreedDocument)
	settledOn(t, store, theCounterparty, agreedDocument)

	require.ErrorIs(t,
		requireUnsettledAgreement(context.Background(), nil, repo, store, thisInstance, settledContractDID),
		ErrOwnAgreementSettled)

	require.NoError(t, withdrawOwnSettlement(context.Background(), nil, store, thisInstance, settledContractDID))

	require.NoError(t, requireUnsettledAgreement(context.Background(), nil, repo, store, thisInstance, settledContractDID))
	// Withdrawing takes back this instance's own word only; the evidence the
	// signing gate holds about the counterparty is not ours to drop.
	require.NotNil(t, store.rows[theCounterparty])
}

// Dropping the local row only stops THIS instance signing. The counterparty
// holds the same settlement as the evidence its own gate reads, so withdrawing
// has to be told to it — and told as a statement about the version that was
// agreed, so it cannot land on a later one.
func TestWithdrawingQueuesTheWithdrawalTowardThePeerThatWasTold(t *testing.T) {
	store := emptyStore()
	settledOn(t, store, thisInstance, agreedDocument)

	require.NoError(t, withdrawOwnSettlement(context.Background(), nil, store, thisInstance, settledContractDID))

	queued, ok := store.queued[theCounterparty]
	require.True(t, ok, "the peer that holds this instance's settlement must be told it is withdrawn")
	require.Equal(t, settledContractDID, queued.DID)
	require.Equal(t, thisInstance, queued.FromPeerDID)

	digest, err := jades.ContractDocumentDigest([]byte(agreedDocument))
	require.NoError(t, err)
	require.Equal(t, digest, queued.DocumentDigest,
		"the withdrawal must name the version that was agreed, not the document as it now stands")
	require.False(t, queued.WithdrawnAt.IsZero())
}

// Nothing was given, so nothing is taken back: a rejection on a contract that
// never settled must not put a message on the wire.
func TestWithdrawingWithoutAnOwnSettlementQueuesNothing(t *testing.T) {
	store := emptyStore()
	settledOn(t, store, theCounterparty, agreedDocument)

	require.NoError(t, withdrawOwnSettlement(context.Background(), nil, store, thisInstance, settledContractDID))

	require.Empty(t, store.queued)
	require.NotNil(t, store.rows[theCounterparty])
}

// The defect the artifact condition closes: the counterparty's copy stays in
// OFFERED when the originator ships a contract it has already signed (only a
// revocation may override intrinsic state), so the transition table still offers
// it its designated way out of an offer — and renegotiating there moves
// contract_data off the signed PDF that copy must never re-render, wedging it out
// of the federation for good. That copy never settled locally, so only the
// artifact can answer.
func TestRenegotiationIsRefusedOnceTheArtifactCarriesThePeersSignature(t *testing.T) {
	repo := &settledContractRepoFake{
		pdfState: db.ContractPDFState{IPFSCID: "cid", C2PAState: "active"},
		document: agreedDocument,
	}

	err := requireUnsettledAgreement(context.Background(), nil, repo, emptyStore(), thisInstance, settledContractDID)

	require.ErrorIs(t, err, ErrAgreementSettled)
	require.NotErrorIs(t, err, ErrOwnAgreementSettled)
}

// A signed artifact answers first: telling a party to reopen the round is the
// wrong instruction once a signature exists, whatever the settlement rows say.
func TestASignedArtifactIsReportedAsSignedNotAsAnOwnAgreement(t *testing.T) {
	repo := &settledContractRepoFake{
		pdfState: db.ContractPDFState{IPFSCID: "cid", C2PAState: "active"},
		document: agreedDocument,
	}
	store := emptyStore()
	settledOn(t, store, thisInstance, agreedDocument)

	err := requireUnsettledAgreement(context.Background(), nil, repo, store, thisInstance, settledContractDID)

	require.ErrorIs(t, err, ErrAgreementSettled)
}

// A contract with no stored artifact yet (genesis, before the first render) and
// no settlement is not settled either.
func TestRenegotiationIsPermittedWithNoStoredArtifact(t *testing.T) {
	repo := &settledContractRepoFake{}

	require.NoError(t, requireUnsettledAgreement(context.Background(), nil, repo, emptyStore(), thisInstance, settledContractDID))
}

// The digest is taken over the JCS canonicalization on both sides, so the same
// document survives the reserialization each instance's jsonb column applies —
// without that, every settled contract would read as renegotiable.
func TestTheSettledVersionIsRecognizedAcrossAReserializationOfTheSameDocument(t *testing.T) {
	repo := unsignedArtifactOf(`{"dcs:documentStructure":{"dcs:clause":"the agreed terms"},"dcs:amount":1.50}`)
	store := emptyStore()
	settledOn(t, store, thisInstance, `{"dcs:amount":1.5,"dcs:documentStructure":{"dcs:clause":"the agreed terms"}}`)

	require.ErrorIs(t,
		requireUnsettledAgreement(context.Background(), nil, repo, store, thisInstance, settledContractDID),
		ErrOwnAgreementSettled)
}

// The gate reads the artifact, not this instance's workflow: a received copy
// sitting in OFFERED holds a signed artifact and is settled, while every
// pre-signing local state over a draft artifact is not.
func TestSettlementIsReadFromTheArtifactNotTheLocalWorkflowState(t *testing.T) {
	for _, state := range []string{
		contractstate.Offered.String(),
		contractstate.Negotiation.String(),
		contractstate.Submitted.String(),
		contractstate.Reviewed.String(),
		contractstate.Approved.String(),
	} {
		draft, err := provenance.ArtifactC2PAState(state, []byte("%PDF-1.7\n1 0 obj\n<< >>\nendobj\n"))
		require.NoError(t, err)
		require.False(t, provenance.ArtifactCarriesSignature(draft), "state %s over an unsigned artifact", state)

		signed, err := provenance.ArtifactC2PAState(state, []byte("<< /Type /Sig /ByteRange [0 1 2 3] >>"))
		require.NoError(t, err)
		require.True(t, provenance.ArtifactCarriesSignature(signed), "state %s over a PAdES-signed artifact", state)
	}
}

// negotiate applies a structured redline to contract_data straight away and
// re-ships the result, so that is the change the settled-agreement gate has to
// stand in front of.
func TestStructuredRedlineRewritesTheDocument(t *testing.T) {
	redline, err := datatype.NewJSON(map[string]any{
		"contract_data": map[string]any{"dcs:documentStructure": map[string]any{}},
	})
	require.NoError(t, err)

	var change negotiationmerging.ChangeRequest
	require.NoError(t, json.Unmarshal(redline, &change))
	require.NotNil(t, change.ContractData)
}

// A free-text negotiation note leaves the document alone, so it stays legal on a
// settled agreement: it is what carries a copy still in OFFERED into NEGOTIATION
// and on towards its own countersignature.
func TestFreeTextNegotiationLeavesTheDocumentAlone(t *testing.T) {
	note, err := datatype.NewJSON("please confirm the delivery window")
	require.NoError(t, err)

	var change negotiationmerging.ChangeRequest
	if err := json.Unmarshal(note, &change); err == nil {
		require.Nil(t, change.ContractData)
	}
}
