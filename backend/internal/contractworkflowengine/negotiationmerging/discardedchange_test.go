package negotiationmerging

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

// The defect: a round whose second accepted request also carries a document
// keeps only that document — mergeContractDataChange returns the change, it
// does not blend it into the base — while the first request's decision row
// still reads ACCEPTED. Asserting only that the later document won would pass
// against that; what has to hold is that the loss is on the record, naming the
// request that caused it.
func TestMergeReportsTheAcceptedDocumentThatDidNotSurviveTheFold(t *testing.T) {
	proposed := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)
	older := acceptedChangeRequest(t, "negotiation-a", proposed, "Older redline", "Clause as the first negotiator worded it.")
	newer := acceptedChangeRequest(t, "negotiation-b", proposed.Add(time.Minute), "Newer redline", "Clause as the second negotiator worded it.")

	merged, superseded := mergeRoundReportingDiscards(t, []db.NegotiationChangeData{older, newer})

	require.Contains(t, string(*merged.ContractData), "as the second negotiator worded it",
		"last-accepted-wins is the merge semantics and stays that way")
	require.Equal(t, []db.NegotiationSupersession{{
		NegotiationID:  "negotiation-a",
		SupersededByID: "negotiation-b",
		Fields:         []string{"name", "contract_data"},
	}}, superseded)
}

// A request loses only what a later one also set. Reporting the whole request
// as discarded would misstate the record just as badly as reporting nothing.
func TestMergeReportsOnlyTheFieldsALaterRequestOverwrote(t *testing.T) {
	proposed := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)
	document := acceptedChangeRequest(t, "negotiation-a", proposed, "Agreed title", "Clause as the first negotiator worded it.")
	renameOnly := changeRequestRow(t, "negotiation-b", proposed.Add(time.Minute), map[string]any{"name": "Retitled by the second negotiator"})

	merged, superseded := mergeRoundReportingDiscards(t, []db.NegotiationChangeData{document, renameOnly})

	require.Contains(t, string(*merged.ContractData), "as the first negotiator worded it",
		"a request that set only the name cannot displace the document")
	require.Equal(t, []db.NegotiationSupersession{{
		NegotiationID:  "negotiation-a",
		SupersededByID: "negotiation-b",
		Fields:         []string{"name"},
	}}, superseded)
}

// Each loss names the request that actually took the field, not the last
// request of the round: the record has to attribute the discard to the edit
// that caused it.
func TestMergeAttributesEachDiscardToTheRequestThatTookTheField(t *testing.T) {
	proposed := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)
	both := acceptedChangeRequest(t, "negotiation-a", proposed, "Agreed title", "Clause as the first negotiator worded it.")
	renameOnly := changeRequestRow(t, "negotiation-b", proposed.Add(time.Minute), map[string]any{"name": "Retitled by the second negotiator"})
	documentOnly := changeRequestRow(t, "negotiation-c", proposed.Add(2*time.Minute), map[string]any{
		"contract_data": json.RawMessage(canonicalContractData(t, "Clause as the third negotiator worded it.")),
	})

	_, superseded := mergeRoundReportingDiscards(t, []db.NegotiationChangeData{both, renameOnly, documentOnly})

	require.Equal(t, []db.NegotiationSupersession{
		{NegotiationID: "negotiation-a", SupersededByID: "negotiation-b", Fields: []string{"name"}},
		{NegotiationID: "negotiation-a", SupersededByID: "negotiation-c", Fields: []string{"contract_data"}},
	}, superseded)
}

// Nothing to report when the accepted requests do not contend: a supersession
// record on a request whose content stands would be a false accusation.
func TestMergeReportsNoDiscardWhenTheAcceptedRequestsDoNotContend(t *testing.T) {
	proposed := time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)
	renameOnly := changeRequestRow(t, "negotiation-a", proposed, map[string]any{"name": "Agreed title"})
	documentOnly := changeRequestRow(t, "negotiation-b", proposed.Add(time.Minute), map[string]any{
		"contract_data": json.RawMessage(canonicalContractData(t, "Clause as the second negotiator worded it.")),
	})

	_, superseded := mergeRoundReportingDiscards(t, []db.NegotiationChangeData{renameOnly, documentOnly})

	require.Empty(t, superseded)
}

func mergeRoundReportingDiscards(t *testing.T, accepted []db.NegotiationChangeData) (*db.ContractUpdateData, []db.NegotiationSupersession) {
	t.Helper()
	stored := canonicalContractData(t, "Clause as the round started.")
	cRepo := &mergeOrderContractRepoFake{stored: &db.Contract{DID: mergeOrderDID, ContractData: &stored}}
	nRepo := &mergeOrderNegotiationRepoFake{accepted: accepted}

	updated, superseded, err := MergeChangeRequests(context.Background(), nil, cRepo, nRepo, mergeOrderDID, 1)
	require.NoError(t, err)
	return updated, superseded
}

// changeRequestRow builds one contract_negotiations row from the exact set of
// change request keys the negotiator submitted.
func changeRequestRow(t *testing.T, id string, createdAt time.Time, fields map[string]any) db.NegotiationChangeData {
	t.Helper()
	changeRequest, err := datatype.NewJSON(fields)
	require.NoError(t, err)
	return db.NegotiationChangeData{ID: id, ChangeRequest: &changeRequest, CreatedAt: createdAt}
}
