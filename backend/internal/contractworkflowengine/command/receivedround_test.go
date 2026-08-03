package command

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

// receivedContractRepoFake models the receiver's own contracts row across
// several ships: RemoteCreate/RemoteUpdate write the row the way the handlers
// compute it, and the reads answer from that row rather than from a fixture, so
// contract_version is whatever the sequence under test actually produced.
type receivedContractRepoFake struct {
	db.ContractRepo
	row *db.Contract
}

func (r *receivedContractRepoFake) ReadProcessDataByDIDOrNil(context.Context, *sqlx.Tx, string) (*db.ContractProcessData, error) {
	if r.row == nil {
		return nil, nil
	}
	return r.processData(), nil
}

func (r *receivedContractRepoFake) ReadProcessDataByDID(context.Context, *sqlx.Tx, string) (*db.ContractProcessData, error) {
	return r.processData(), nil
}

func (r *receivedContractRepoFake) processData() *db.ContractProcessData {
	return &db.ContractProcessData{
		DID:             r.row.DID,
		Origin:          r.row.Origin,
		State:           r.row.State,
		ContractVersion: r.row.ContractVersion,
		CreatedBy:       r.row.CreatedBy,
	}
}

func (r *receivedContractRepoFake) RemoteCreate(_ context.Context, _ *sqlx.Tx, data db.Contract) error {
	copied := data
	r.row = &copied
	return nil
}

func (r *receivedContractRepoFake) RemoteUpdate(_ context.Context, _ *sqlx.Tx, data db.Contract) error {
	copied := data
	r.row = &copied
	return nil
}

func (r *receivedContractRepoFake) ReadDataByDID(context.Context, *sqlx.Tx, string) (*db.Contract, error) {
	stored := taskStoredContract()
	stored.ContractVersion = r.row.ContractVersion
	return stored, nil
}

func (r *receivedContractRepoFake) Update(_ context.Context, _ *sqlx.Tx, data db.ContractUpdateData) error {
	if data.ContractVersion != 0 {
		r.row.ContractVersion = data.ContractVersion
	}
	return nil
}

func (r *receivedContractRepoFake) UpdateState(_ context.Context, _ *sqlx.Tx, _ string, state string) error {
	r.row.State = state
	return nil
}

// localTaskRepoFake records the review and approval tasks a first receipt opens.
type localReviewTaskRepoFake struct {
	db.ReviewTaskRepo
	rows []db.ReviewTaskData
}

func (r *localReviewTaskRepoFake) Create(_ context.Context, _ *sqlx.Tx, data db.ReviewTaskData) (*time.Time, error) {
	r.rows = append(r.rows, data)
	now := time.Now().UTC()
	return &now, nil
}

type localApprovalTaskRepoFake struct {
	db.ApprovalTaskRepo
	rows []db.ApprovalTaskData
}

func (r *localApprovalTaskRepoFake) Create(_ context.Context, _ *sqlx.Tx, data db.ApprovalTaskData) (*time.Time, error) {
	r.rows = append(r.rows, data)
	now := time.Now().UTC()
	return &now, nil
}

// shippedContract is the JSON-LD a peer ships in the contract PDF.
func shippedContract() []byte {
	return []byte(`{"@id":"` + taskContractDID + `","@type":"dcs:Contract","dcs:metadata":{"dcs:title":"Shipped Agreement"}}`)
}

func peerShipCmd() PeerPdfReceiveCmd {
	return PeerPdfReceiveCmd{
		ContractIRI:  taskContractDID,
		Counterparty: originPeerDID,
		LocalPeer:    localPeerDID,
		Payload:      shippedContract(),
	}
}

// receivingSide wires the two handlers a responder runs against one contract:
// the peer-ship receiver and the offer acceptor, sharing the contracts row and
// the negotiation tasks so a sequence of ships and acts reads as one instance.
func receivingSide(t *testing.T) (*PeerPdfReceiver, *OfferAcceptor, *receivedContractRepoFake, *negotiationTaskRepoFake) {
	t.Helper()
	contracts := &receivedContractRepoFake{}
	tasks := &negotiationTaskRepoFake{}
	testDB := taskTestDB(t)
	didDocument := taskTestDIDDocument(t)
	receiver := &PeerPdfReceiver{
		DB:     testDB,
		CRepo:  contracts,
		RTRepo: &localReviewTaskRepoFake{},
		ATRepo: &localApprovalTaskRepoFake{},
		NTRepo: tasks,
	}
	acceptor := &OfferAcceptor{
		DB:          testDB,
		CRepo:       contracts,
		NTRepo:      tasks,
		DIDDocument: *didDocument,
	}
	return receiver, acceptor, contracts, tasks
}

// currentRoundTask is the responder's negotiation task as the round predicates
// see it: a task on a superseded round answers none of them, and a Negotiations
// tab row backed by one is a row that cannot be acted on.
func currentRoundTask(t *testing.T, tasks *negotiationTaskRepoFake, contracts *receivedContractRepoFake) *db.NegotiationTaskData {
	t.Helper()
	for i := range tasks.rows {
		if tasks.rows[i].Negotiator != localPeerDID {
			continue
		}
		if tasks.rows[i].ContractVersion == contracts.row.ContractVersion {
			return &tasks.rows[i]
		}
	}
	return nil
}

// One offer ships twice (the OFFER_CONTRACT ship and the PDF_REGENERATED ship
// of the same document), so the responder's contract_version moves without the
// responder doing anything. Its task must move with it: asserting only that a
// task EXISTS passes while it sits on a round the contract has left behind.
func TestATwiceReceivedOfferKeepsItsAcceptedTaskOnTheContractsCurrentRound(t *testing.T) {
	receiver, acceptor, contracts, tasks := receivingSide(t)
	ctx := context.Background()

	require.NoError(t, receiver.Handle(ctx, peerShipCmd()))
	require.Equal(t, 1, contracts.row.ContractVersion)
	require.Equal(t, contractstate.Offered.String(), contracts.row.State)
	require.Empty(t, tasks.rows, "receiving an offer is not engaging with it")

	require.NoError(t, acceptor.Handle(ctx, acceptOfferCmd()))
	require.Len(t, tasks.rows, 1)
	require.Equal(t, 1, tasks.rows[0].ContractVersion)

	// The second ship of the same offer.
	require.NoError(t, receiver.Handle(ctx, peerShipCmd()))

	require.Equal(t, 2, contracts.row.ContractVersion)
	task := currentRoundTask(t, tasks, contracts)
	require.NotNil(t, task, "the responder's Negotiations tab row is stranded on round %d while the contract is on round %d",
		tasks.rows[0].ContractVersion, contracts.row.ContractVersion)
	require.Equal(t, negotiationtaskstate.Open.String(), task.State,
		"a new document is a new round to answer")
	require.Len(t, tasks.rows, 1, "carrying a task forward moves it, it does not mint a second one")
}

// The ships can also both land before the responder acts, which is the ordering
// the two-instance vertical produces: the accept then has to mint for the round
// the contract actually stands on, not for the one it arrived on.
func TestAcceptingAfterBothShipsMintsForTheRoundTheContractStandsOn(t *testing.T) {
	receiver, acceptor, contracts, tasks := receivingSide(t)
	ctx := context.Background()

	require.NoError(t, receiver.Handle(ctx, peerShipCmd()))
	require.NoError(t, receiver.Handle(ctx, peerShipCmd()))
	require.Equal(t, 2, contracts.row.ContractVersion)
	require.Empty(t, tasks.rows)

	require.NoError(t, acceptor.Handle(ctx, acceptOfferCmd()))

	require.Len(t, tasks.rows, 1)
	require.NotNil(t, currentRoundTask(t, tasks, contracts))
	require.Equal(t, contractstate.Negotiation.String(), contracts.row.State)
}

// A revocation ship carries the local copy to REVOKED and still bumps the
// version, so the carry-forward has to hold for every kind of ship, not only
// for the negotiation ones.
func TestARevocationShipCarriesTheTaskForwardToo(t *testing.T) {
	receiver, acceptor, contracts, tasks := receivingSide(t)
	ctx := context.Background()

	require.NoError(t, receiver.Handle(ctx, peerShipCmd()))
	require.NoError(t, acceptor.Handle(ctx, acceptOfferCmd()))

	revoked := peerShipCmd()
	revoked.AdoptRevoked = true
	require.NoError(t, receiver.Handle(ctx, revoked))

	require.Equal(t, contractstate.Revoked.String(), contracts.row.State)
	require.NotNil(t, currentRoundTask(t, tasks, contracts))
}

// The shipped document is what the local copy carries: a re-ship refreshes the
// content but must not reset the responder's own intrinsic state.
func TestAReshipRefreshesTheContentWithoutResettingLocalState(t *testing.T) {
	receiver, acceptor, contracts, _ := receivingSide(t)
	ctx := context.Background()

	require.NoError(t, receiver.Handle(ctx, peerShipCmd()))
	require.NoError(t, acceptor.Handle(ctx, acceptOfferCmd()))
	require.Equal(t, contractstate.Negotiation.String(), contracts.row.State)

	require.NoError(t, receiver.Handle(ctx, peerShipCmd()))

	require.Equal(t, contractstate.Negotiation.String(), contracts.row.State)
	require.Equal(t, datatype.JSON(shippedContract()), *contracts.row.ContractData)
}
