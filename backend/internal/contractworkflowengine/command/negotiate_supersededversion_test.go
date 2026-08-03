package command

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

func negotiator(t *testing.T, process *db.ContractProcessData, tasks *negotiationTaskRepoFake, negotiations *negotiationRepoFake) (*Negotiator, *taskContractRepoFake) {
	t.Helper()
	contracts := &taskContractRepoFake{process: process, stored: taskStoredContract()}
	return &Negotiator{
		DB:          taskTestDB(t),
		CRepo:       contracts,
		NRepo:       negotiations,
		NTRepo:      tasks,
		SRepo:       emptyStore(),
		DIDDocument: *taskTestDIDDocument(t),
	}, contracts
}

// redlinedDocument is what a proposing client posts as
// change_request.contract_data: a complete contract, not a patch.
func redlinedDocument(t *testing.T) datatype.JSON {
	t.Helper()
	return minimalCanonicalContractData(t, taskContractDID)
}

func negotiationCmd(t *testing.T, changeRequest map[string]any) NegotiationCmd {
	t.Helper()
	encoded, err := datatype.NewJSON(changeRequest)
	require.NoError(t, err)
	return NegotiationCmd{
		DID:           taskContractDID,
		NegotiatedBy:  "Counterparty Org",
		ChangeRequest: &encoded,
		UpdatedAt:     time.Now().UTC().Add(time.Hour),
		HolderDID:     "did:key:user",
		UserRoles:     userrole.UserRoles{userrole.ContractNegotiator},
		CauserDID:     localPeerDID,
	}
}

// The defect: a redline is applied to contract_data the moment it is PROPOSED,
// so the contract a reviewer opens the proposal against already carries the
// proposed value — the comparison rendered the proposal on both sides and the
// version it asks to change FROM was gone for good.
func TestProposingARedlineSnapshotsTheSupersededVersionBeforeApplyingIt(t *testing.T) {
	redline := redlinedDocument(t)
	handler, contracts := negotiator(t, inboundOffer(1), &negotiationTaskRepoFake{}, &negotiationRepoFake{})

	require.NoError(t, handler.Handle(context.Background(), negotiationCmd(t, map[string]any{"contract_data": redline})))

	require.Equal(t, 1, contracts.historyEntries, "the replaced document must stay retrievable")
	require.Equal(t, []int{2}, contracts.versionBumps())
	require.Equal(t, []string{"history", "update"}, contracts.writes,
		"a snapshot taken after the apply would snapshot the proposal, not what it supersedes")
}

// The negotiation row keys on the version it was proposed against, which is the
// one just snapshotted: that pairing is what lets a reviewer resolve the
// proposal's "from" side.
func TestTheSnapshottedVersionIsTheOneTheNegotiationRowKeysOn(t *testing.T) {
	redline := redlinedDocument(t)
	negotiations := &negotiationRepoFake{}
	handler, contracts := negotiator(t, inboundOffer(4), &negotiationTaskRepoFake{}, negotiations)

	require.NoError(t, handler.Handle(context.Background(), negotiationCmd(t, map[string]any{"contract_data": redline})))

	require.Len(t, negotiations.proposed, 1)
	require.Equal(t, 4, negotiations.proposed[0].data.ContractVersion)
	require.Equal(t, []int{5}, contracts.versionBumps(),
		"the round the proposal opens is the one after the round it was proposed against")
}

// A free-text note carries no document, so it changes neither the contract nor
// the version — there is nothing superseded to snapshot, and the live contract
// is still the proposal's "from" side.
func TestProposingAFreeTextNoteSupersedesNothing(t *testing.T) {
	handler, contracts := negotiator(t, inboundOffer(1), &negotiationTaskRepoFake{}, &negotiationRepoFake{})

	require.NoError(t, handler.Handle(context.Background(), negotiationCmd(t, map[string]any{"comment": "please reconsider the amount"})))

	require.Zero(t, contracts.historyEntries)
	require.Empty(t, contracts.versionBumps())
}

// The proposal is applied at propose time and the negotiation row stays on the
// version it superseded, so the acceptance decides a round the merge no longer
// finds anything to fold in for. Proposing and then accepting must therefore
// move the contract forward exactly one version — a second bump would re-apply
// the same redline and open a round no task belongs to.
func TestAProposalFollowedByItsAcceptanceBumpsTheVersionOnce(t *testing.T) {
	redline := redlinedDocument(t)
	negotiations := &negotiationRepoFake{}
	tasks := &negotiationTaskRepoFake{}
	proposer, contracts := negotiator(t, inboundOffer(1), tasks, negotiations)

	require.NoError(t, proposer.Handle(context.Background(), negotiationCmd(t, map[string]any{"contract_data": redline})))
	require.Equal(t, []int{2}, contracts.versionBumps())
	require.Equal(t, 2, tasks.rows[0].ContractVersion, "the redline opened a new round and the task moved with it")

	acceptor := &NegotiationAcceptor{
		DB:          taskTestDB(t),
		CRepo:       contracts,
		NRepo:       negotiations,
		NTRepo:      tasks,
		DIDDocument: *taskTestDIDDocument(t),
	}
	require.NoError(t, acceptor.Handle(context.Background(), AcceptNegotiationCmd{
		ID:         negotiations.proposed[0].id,
		DID:        taskContractDID,
		AcceptedBy: "Second Negotiator Org",
		HolderDID:  "did:key:other-user",
		UserRoles:  userrole.UserRoles{userrole.ContractNegotiator},
		CauserDID:  localPeerDID,
	}))

	submitter := &Submitter{
		DB:          taskTestDB(t),
		CRepo:       contracts,
		NRepo:       negotiations,
		NTRepo:      tasks,
		DIDDocument: *taskTestDIDDocument(t),
	}
	require.NoError(t, submitter.Handle(context.Background(), negotiationSubmitCmd()))

	require.Equal(t, []int{2}, contracts.versionBumps(), "the accepted proposal is already in the document")
	require.Equal(t, 1, contracts.historyEntries)
	require.Equal(t, contractstate.Submitted.String(), contracts.process.State,
		"the round the proposal opened settles once every task in it is accepted")
}

// Every handler that replaces the contract document with a locally authored one
// and bumps contract_version must snapshot the version it replaces first, or
// that document is unrecoverable: GET /contract/history is the only way back to
// it. receivepdf.go is not one of these — it adopts the counterparty's document
// rather than authoring one, and the versions it supersedes are the peer's own
// ships, retained in the stored PDF's C2PA provenance chain (ADR-13).
func TestEveryLocallyAuthoredContractVersionBumpSnapshotsWhatItSupersedes(t *testing.T) {
	for _, file := range []string{"submit.go", "negotiate.go"} {
		require.Contains(t, callsIn(t, file, "Handle"), "CreateHistoryEntryForDID",
			"%s replaces the contract document without snapshotting the superseded version", file)
	}
}
