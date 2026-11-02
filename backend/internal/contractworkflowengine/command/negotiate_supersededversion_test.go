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
// version it asks to change FROM was gone for good. Propose must leave the
// formal document and version alone. Submit is what applies accepted changes.
func TestProposingARedlineDoesNotChangeTheFormalContract(t *testing.T) {
	redline := redlinedDocument(t)
	handler, contracts := negotiator(t, inboundOffer(1), &negotiationTaskRepoFake{}, &negotiationRepoFake{})

	require.NoError(t, handler.Handle(context.Background(), negotiationCmd(t, map[string]any{"contract_data": redline})))

	require.Zero(t, contracts.historyEntries)
	require.Empty(t, contracts.versionBumps())
	require.Empty(t, contracts.writes, "propose must not write the formal contract row")
}

// The negotiation row keys on the version it was proposed against: that
// pairing is what lets a reviewer resolve the proposal's "from" side.
func TestTheNegotiationRowKeysOnTheCurrentFormalVersion(t *testing.T) {
	redline := redlinedDocument(t)
	negotiations := &negotiationRepoFake{}
	handler, contracts := negotiator(t, inboundOffer(4), &negotiationTaskRepoFake{}, negotiations)

	require.NoError(t, handler.Handle(context.Background(), negotiationCmd(t, map[string]any{"contract_data": redline})))

	require.Len(t, negotiations.proposed, 1)
	require.Equal(t, 4, negotiations.proposed[0].data.ContractVersion)
	require.Empty(t, contracts.versionBumps())
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

// The proposal is not applied at propose time, so the acceptance decides a
// round the merge still finds something to fold in for. Proposing and then
// accepting must therefore leave the version unchanged; Submit applies the
// accepted set once — a second bump at propose would open a round no task
// belongs to, and leaving apply to Submit alone moves the contract forward
// exactly one version.
func TestAProposalFollowedByItsAcceptanceBumpsTheVersionOnceOnSubmit(t *testing.T) {
	redline := redlinedDocument(t)
	negotiations := &negotiationRepoFake{}
	tasks := &negotiationTaskRepoFake{}
	proposer, contracts := negotiator(t, inboundOffer(1), tasks, negotiations)

	require.NoError(t, proposer.Handle(context.Background(), negotiationCmd(t, map[string]any{"contract_data": redline})))
	require.Empty(t, contracts.versionBumps())
	require.Equal(t, 1, tasks.rows[0].ContractVersion, "propose keeps the current round")
	require.Equal(t, contractstate.Negotiation.String(), contracts.process.State)

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
	require.Empty(t, contracts.versionBumps(), "accept decides; it does not apply")

	submitter := &Submitter{
		DB:          taskTestDB(t),
		CRepo:       contracts,
		NRepo:       negotiations,
		NTRepo:      tasks,
		SRepo:       emptyStore(),
		DIDDocument: *taskTestDIDDocument(t),
	}
	require.NoError(t, submitter.Handle(context.Background(), negotiationSubmitCmd()))

	require.Equal(t, []int{2}, contracts.versionBumps(), "Submit applies accepted proposals once")
	require.Equal(t, 1, contracts.historyEntries)
	require.Equal(t, []string{"history", "update"}, contracts.writes[:2],
		"snapshot the formal version before replacing it")
	require.Equal(t, contractstate.Negotiation.String(), contracts.process.State,
		"a merge opens the next round; it does not settle")
	require.NotContains(t, contracts.states, contractstate.Submitted.String())
	require.Equal(t, 2, tasks.rows[0].ContractVersion)
}

// An accepted change carries fields that live on the contracts row, not in the
// document. Propose, accept and submit must keep them all, and the snapshot of
// the replaced version must name the submitter whose save replaced it.
func TestAnAcceptedNoticePeriodChangeSurvivesSubmitAndTheSnapshotNamesTheActor(t *testing.T) {
	negotiations := &negotiationRepoFake{}
	tasks := &negotiationTaskRepoFake{}
	proposer, contracts := negotiator(t, inboundOffer(1), tasks, negotiations)

	require.NoError(t, proposer.Handle(context.Background(), negotiationCmd(t, map[string]any{
		"contract_data":     redlinedDocument(t),
		"exp_notice_period": 45,
	})))

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
		SRepo:       emptyStore(),
		DIDDocument: *taskTestDIDDocument(t),
	}
	submit := negotiationSubmitCmd()
	submit.SubmittedBy = "Submitting Negotiator"
	require.NoError(t, submitter.Handle(context.Background(), submit))

	require.Equal(t, []int{2}, contracts.versionBumps())
	require.NotNil(t, contracts.stored.ExpNoticePeriod, "the accepted notice period was dropped")
	require.Equal(t, 45, *contracts.stored.ExpNoticePeriod)
	require.Equal(t, []string{"Submitting Negotiator"}, contracts.historyActors)
}

// A Draft save keeps its row fields and snapshots the replaced version under
// the organization that saved it. Only the creating organization saves a
// draft (ErrNotCreator).
func TestADraftSaveSnapshotsTheReplacedVersionUnderTheEditor(t *testing.T) {
	contracts := &taskContractRepoFake{process: inboundOffer(1), stored: taskStoredContract()}
	contracts.stored.Origin = localPeerDID
	contracts.stored.CreatedBy = "Editing User"
	contracts.stored.State = contractstate.Draft.String()
	updater := &Updater{
		DB:          taskTestDB(t),
		CRepo:       contracts,
		DIDDocument: *taskTestDIDDocument(t),
	}
	notice := 30

	require.NoError(t, updater.Handle(context.Background(), UpdateCmd{
		DID:             taskContractDID,
		UpdatedAt:       time.Now().UTC(),
		UpdatedBy:       "Editing User",
		ExpNoticePeriod: &notice,
		CauserDID:       localPeerDID,
	}))

	require.Equal(t, []string{"Editing User"}, contracts.historyActors)
	require.NotNil(t, contracts.stored.ExpNoticePeriod)
	require.Equal(t, 30, *contracts.stored.ExpNoticePeriod)
}

// Every handler that replaces the contract document with a locally authored one
// and bumps contract_version must snapshot the version it replaces first, or
// that document is unrecoverable: GET /contract/history is the only way back to
// it. receivepdf.go is one of these too — it adopts the counterparty's document
// and bumps contract_version, so the held version must be snapshotted first
// (peer ships also remain in the stored PDF's C2PA provenance chain (ADR-13)).
// negotiate.go is not one of these either — it records proposals without
// replacing the document.
func TestEveryLocallyAuthoredContractVersionBumpSnapshotsWhatItSupersedes(t *testing.T) {
	require.Contains(t, callsIn(t, "submit.go", "Handle"), "CreateHistoryEntryForDID",
		"submit.go replaces the contract document without snapshotting the superseded version")
	require.Contains(t, callsIn(t, "receivepdf.go", "Handle"), "CreateHistoryEntryForDID",
		"receivepdf.go replaces the held document without snapshotting the superseded version")
	require.NotContains(t, callsIn(t, "negotiate.go", "Handle"), "CreateHistoryEntryForDID",
		"negotiate.go does not replace the contract document at propose time")
}
