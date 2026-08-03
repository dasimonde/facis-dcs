package negotiationmerging

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/contractworkflowengine/db"
)

const mergeOrderDID = "did:web:facis.example:contract:round"

// mergeOrderContractRepoFake answers the one read the merge makes: the
// document the round started from.
type mergeOrderContractRepoFake struct {
	db.ContractRepo
	stored *db.Contract
}

func (r *mergeOrderContractRepoFake) ReadDataByDID(context.Context, *sqlx.Tx, string) (*db.Contract, error) {
	return r.stored, nil
}

// mergeOrderNegotiationRepoFake hands the accepted rows back in whatever order
// it was given, standing in for a query whose row order is a planner detail.
type mergeOrderNegotiationRepoFake struct {
	db.NegotiationRepo
	accepted []db.NegotiationChangeData
}

func (r *mergeOrderNegotiationRepoFake) ReadAllAcceptedByContractDIDAndVersion(context.Context, *sqlx.Tx, string, int) ([]db.NegotiationChangeData, error) {
	return r.accepted, nil
}

// TestMergeChangeRequestsIsIndependentOfRowOrder pins the merge to the
// proposal order of the accepted requests rather than the order the repository
// returns them in: the merged document is bound by digest into the settlement
// artifact, so a document that depends on the planner is one the counterparty
// cannot be held to.
func TestMergeChangeRequestsIsIndependentOfRowOrder(t *testing.T) {
	proposed := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	older := acceptedChangeRequest(t, "negotiation-a", proposed, "Older redline", "Clause as the first negotiator worded it.")
	newer := acceptedChangeRequest(t, "negotiation-b", proposed.Add(time.Minute), "Newer redline", "Clause as the second negotiator worded it.")

	inReadOrder := mergeRound(t, []db.NegotiationChangeData{older, newer})
	reversed := mergeRound(t, []db.NegotiationChangeData{newer, older})

	require.Equal(t, string(*inReadOrder.ContractData), string(*reversed.ContractData))
	require.Equal(t, *inReadOrder.Name, *reversed.Name)

	// Both orders must settle on the LATER request, not merely agree: an
	// implementation that sorted the other way round would satisfy the
	// equality above while inverting last-write-wins.
	require.Equal(t, "Newer redline", *inReadOrder.Name)
	require.Contains(t, string(*inReadOrder.ContractData), "as the second negotiator worded it")
}

// TestMergeChangeRequestsOrdersRequestsOfOneTransactionByID covers the tie the
// clock cannot break: CURRENT_TIMESTAMP is the transaction clock, so requests
// written in one transaction share a created_at and only the primary key
// separates them.
func TestMergeChangeRequestsOrdersRequestsOfOneTransactionByID(t *testing.T) {
	sameInstant := time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC)
	first := acceptedChangeRequest(t, "negotiation-a", sameInstant, "First by id", "Clause as the first negotiator worded it.")
	second := acceptedChangeRequest(t, "negotiation-b", sameInstant, "Second by id", "Clause as the second negotiator worded it.")

	inReadOrder := mergeRound(t, []db.NegotiationChangeData{first, second})
	reversed := mergeRound(t, []db.NegotiationChangeData{second, first})

	require.Equal(t, string(*inReadOrder.ContractData), string(*reversed.ContractData))
	require.Equal(t, "Second by id", *inReadOrder.Name)
}

func mergeRound(t *testing.T, accepted []db.NegotiationChangeData) *db.ContractUpdateData {
	t.Helper()
	stored := canonicalContractData(t, "Clause as the round started.")
	cRepo := &mergeOrderContractRepoFake{stored: &db.Contract{DID: mergeOrderDID, ContractData: &stored}}
	nRepo := &mergeOrderNegotiationRepoFake{accepted: accepted}

	updated, _, err := MergeChangeRequests(context.Background(), nil, cRepo, nRepo, mergeOrderDID, 1)
	require.NoError(t, err)
	require.NotNil(t, updated.ContractData)
	require.NotNil(t, updated.Name)
	return updated
}

// acceptedChangeRequest builds one contract_negotiations row: a redline that
// renames the contract and rewords its only clause.
func acceptedChangeRequest(t *testing.T, id string, createdAt time.Time, name string, clause string) db.NegotiationChangeData {
	t.Helper()
	document := canonicalContractData(t, clause)
	changeRequest, err := datatype.NewJSON(map[string]any{
		"name":          name,
		"contract_data": json.RawMessage(document),
	})
	require.NoError(t, err)
	return db.NegotiationChangeData{ID: id, ChangeRequest: &changeRequest, CreatedAt: createdAt}
}

func canonicalContractData(t *testing.T, clause string) datatype.JSON {
	t.Helper()
	data := map[string]any{
		"@context": map[string]any{
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#",
			"xsd": "http://www.w3.org/2001/XMLSchema#",
		},
		"@id":   mergeOrderDID,
		"@type": "dcs:Contract",
		"dcs:metadata": map[string]any{
			"@id":   mergeOrderDID + "#metadata",
			"@type": "dcs:ContractMetadata",
		},
		"dcs:documentStructure": map[string]any{
			"@id":   mergeOrderDID + "#document-structure",
			"@type": "dcs:DocumentStructure",
			"dcs:blocks": map[string]any{"@list": []any{
				map[string]any{
					"@id":         mergeOrderDID + "#clause-1",
					"@type":       "dcs:Clause",
					"dcs:content": map[string]any{"@list": []any{clause}},
				},
			}},
			"dcs:layout": []any{
				map[string]any{
					"@id":        mergeOrderDID + "#root",
					"dcs:isRoot": true,
					"dcs:children": map[string]any{"@list": []any{
						map[string]any{"@id": mergeOrderDID + "#clause-1"},
					}},
				},
				map[string]any{
					"@id":          mergeOrderDID + "#clause-1",
					"dcs:children": map[string]any{"@list": []any{}},
				},
			},
		},
	}
	result, err := datatype.NewJSON(data)
	require.NoError(t, err)
	return result
}
