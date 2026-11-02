package command

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/negotiationtaskstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/contractworkflowengine/negotiationmerging"
)

// A change request that is a JSON string is a free-text note for the
// negotiation audit trail. When the round is merged, the note changes nothing
// and the merge succeeds.
func TestMergingARoundWithAnAcceptedFreeTextNote(t *testing.T) {
	note := datatype.JSON(`"Redline on Liability: proposed replacement text"`)
	title := datatype.JSON(`{"name":"Title from the structured request"}`)
	negotiations := &negotiationRepoFake{
		negotiations: true,
		accepted: map[int][]db.NegotiationChangeData{2: {
			{ID: "negotiation-note", ChangeRequest: &note},
			{ID: "negotiation-title", ChangeRequest: &title},
		}},
	}
	tasks := &negotiationTaskRepoFake{rows: []db.NegotiationTaskData{{
		DID:             taskContractDID,
		ContractVersion: 2,
		Negotiator:      localPeerDID,
		State:           negotiationtaskstate.Open.String(),
	}}}
	contracts := &taskContractRepoFake{process: inNegotiation(2), stored: taskStoredContract()}
	handler := &Submitter{
		DB:          outboxRecordingDB(t, &recordingOutbox{}),
		CRepo:       contracts,
		NRepo:       negotiations,
		NTRepo:      tasks,
		DIDDocument: *taskTestDIDDocument(t),
	}

	require.NoError(t, handler.Handle(context.Background(), negotiationSubmitCmd()))

	require.Equal(t, "Title from the structured request", *contracts.updates[0].Name)
	require.Empty(t, negotiations.superseded, "a note sets no field, so it loses none")
}

// A change request that is neither a note nor a ChangeRequest object is
// refused when it is proposed, so it never reaches the merge.
func TestProposingAMalformedChangeRequestIsRefused(t *testing.T) {
	for name, raw := range map[string]string{
		"number":         `42`,
		"array":          `["a"]`,
		"wrong type":     `{"name":7}`,
		"not a date":     `{"start_date":"tomorrow"}`,
		"not an RFC3339": `{"exp_date":"2027-01-01"}`,
	} {
		t.Run(name, func(t *testing.T) {
			negotiations := &negotiationRepoFake{}
			handler, _ := negotiator(t, inboundOffer(1), &negotiationTaskRepoFake{}, negotiations)
			cmd := negotiationCmd(t, nil)
			changeRequest := datatype.JSON(raw)
			cmd.ChangeRequest = &changeRequest

			err := handler.Handle(context.Background(), cmd)

			require.ErrorIs(t, err, negotiationmerging.ErrInvalidChangeRequest)
			require.Empty(t, negotiations.proposed)
		})
	}
}

// The free-text note is the documented string form and is accepted.
func TestProposingAFreeTextNoteIsAccepted(t *testing.T) {
	negotiations := &negotiationRepoFake{}
	handler, contracts := negotiator(t, inboundOffer(1), &negotiationTaskRepoFake{}, negotiations)
	cmd := negotiationCmd(t, nil)
	note := datatype.JSON(`"Redline on Liability: proposed replacement text"`)
	cmd.ChangeRequest = &note

	require.NoError(t, handler.Handle(context.Background(), cmd))

	require.Len(t, negotiations.proposed, 1)
	require.Empty(t, contracts.writes)
}
