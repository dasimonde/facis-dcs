package command

import (
	"context"
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"

	"github.com/jmoiron/sqlx"
)

func TestSubmitterContractDataForSemanticValidationPersistsSubmittedData(t *testing.T) {
	ctx := context.Background()
	did := "did:web:facis.example:contract:submit"
	submitted := minimalCanonicalContractData(t, "did:web:facis.example:contract:stale")
	storedInvalid := datatype.JSON(`{
        "semanticConditionValues":[{"parameterName":"provider.country","parameterValue":"USA"}],
        "dcs:effectiveShapes":[{"@id":"https://dcs.test/semantic/shapes/facis-dcs?version=4"}]}`)

	repo := &submitContractRepoFake{
		stored: &db.Contract{
			DID:          did,
			ContractData: &storedInvalid,
		},
	}
	submitter := Submitter{CRepo: repo}

	contractData, err := submitter.contractDataForSemanticValidation(ctx, nil, SubmitCmd{
		DID:          did,
		ContractData: &submitted,
	})
	if err != nil {
		t.Fatalf("contractDataForSemanticValidation returned error: %v", err)
	}
	if repo.updated == nil || repo.updated.ContractData == nil {
		t.Fatalf("submitted contract data was not persisted")
	}
	if contractData != repo.updated.ContractData {
		t.Fatalf("returned contract data is not the persisted normalized contract data")
	}
	if err := validation.ValidateContractSemantics(contractData); err != nil {
		t.Fatalf("persisted submitted contract data is not semantically valid: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(*repo.updated.ContractData, &decoded); err != nil {
		t.Fatalf("could not decode persisted contract data: %v", err)
	}
	// The submitted document is what is stored — the stale stored one is only
	// consulted for the Semantic Hub bundle the contract is pinned to, which a
	// client-assembled document never carries.
	if decoded["@id"] != did {
		t.Fatalf("persisted contract data @id = %v, want %s", decoded["@id"], did)
	}
	if _, ok := decoded["semanticConditionValues"]; ok {
		t.Fatalf("stored contract data leaked into the submitted document: %v", decoded)
	}
	effectiveShapes, _ := decoded["dcs:effectiveShapes"].([]any)
	if len(effectiveShapes) != 1 {
		t.Fatalf("pinned effective shapes were not carried forward: %v", decoded["dcs:effectiveShapes"])
	}
}

func TestCanSubmitUpdatedContractDataOnlyAllowsCreatorSubmitStates(t *testing.T) {
	allowed := []string{
		contractstate.Draft.String(),
		contractstate.Rejected.String(),
	}
	for _, state := range allowed {
		if !canSubmitUpdatedContractData(state) {
			t.Fatalf("state %s should allow submitted contract data", state)
		}
	}

	rejected := []string{
		contractstate.Negotiation.String(),
		contractstate.Submitted.String(),
		contractstate.Reviewed.String(),
		contractstate.Approved.String(),
	}
	for _, state := range rejected {
		if canSubmitUpdatedContractData(state) {
			t.Fatalf("state %s should reject submitted contract data", state)
		}
	}
}

func minimalCanonicalContractData(t *testing.T, id string) datatype.JSON {
	t.Helper()
	data := map[string]any{
		"@context": map[string]any{
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#",
			"xsd": "http://www.w3.org/2001/XMLSchema#",
		},
		"@id":   id,
		"@type": "dcs:Contract",
		"dcs:metadata": map[string]any{
			"@id":   id + "#metadata",
			"@type": "dcs:ContractMetadata",
		},
		"dcs:documentStructure": map[string]any{
			"@id":   id + "#document-structure",
			"@type": "dcs:DocumentStructure",
			"dcs:blocks": map[string]any{"@list": []any{
				map[string]any{
					"@id":   id + "#clause-1",
					"@type": "dcs:Clause",
					"dcs:content": map[string]any{
						"@list": []any{"Contract content."},
					},
				},
			}},
			"dcs:layout": []any{
				map[string]any{
					"@id":        id + "#root",
					"dcs:isRoot": true,
					"dcs:children": map[string]any{
						"@list": []any{
							map[string]any{"@id": id + "#clause-1"},
						},
					},
				},
				map[string]any{
					"@id": id + "#clause-1",
					"dcs:children": map[string]any{
						"@list": []any{},
					},
				},
			},
		},
	}
	result, err := datatype.NewJSON(data)
	if err != nil {
		t.Fatalf("could not create contract data: %v", err)
	}
	return result
}

type submitContractRepoFake struct {
	db.ContractRepo
	stored  *db.Contract
	updated *db.ContractUpdateData
}

func (r *submitContractRepoFake) ReadDataByDID(context.Context, *sqlx.Tx, string) (*db.Contract, error) {
	return r.stored, nil
}

func (r *submitContractRepoFake) Update(_ context.Context, _ *sqlx.Tx, data db.ContractUpdateData) error {
	r.updated = &data
	return nil
}

func (r *submitContractRepoFake) Create(context.Context, *sqlx.Tx, db.Contract) error {
	panic("not implemented")
}
