package mapper

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"digital-contracting-service/internal/base/datatype"
	contractdb "digital-contracting-service/internal/contractworkflowengine/db"
	templatedb "digital-contracting-service/internal/templaterepository/db"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

func newJSON(t *testing.T, value any) *datatype.JSON {
	t.Helper()
	raw, err := datatype.NewJSON(value)
	require.NoError(t, err)
	return &raw
}

func fixedTime() time.Time {
	return time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
}

func canonicalTemplateData() map[string]any {
	return map[string]any{
		"@context": map[string]any{
			"dcs":  "https://w3id.org/facis/dcs/ontology/v1#",
			"odrl": "http://www.w3.org/ns/odrl/2/",
			"xsd":  "http://www.w3.org/2001/XMLSchema#",
		},
		"@id":   "did:web:example:template:1",
		"@type": "dcs:ContractTemplate",
		"dcs:metadata": map[string]any{
			"@type":    "dcs:TemplateMetadata",
			"dcs:name": "Test Template",
		},
		"dcs:documentStructure": map[string]any{
			"@type":      "dcs:DocumentStructure",
			"dcs:blocks": []any{},
			"dcs:layout": []any{},
		},
		"dcs:contractData": []any{},
		"dcs:policies":     []any{},
	}
}

func canonicalContractData() map[string]any {
	return map[string]any{
		"@context": map[string]any{
			"dcs":  "https://w3id.org/facis/dcs/ontology/v1#",
			"odrl": "http://www.w3.org/ns/odrl/2/",
			"xsd":  "http://www.w3.org/2001/XMLSchema#",
		},
		"@id":   "did:web:example:contract:1",
		"@type": "dcs:Contract",
		"dcs:metadata": map[string]any{
			"@type": "dcs:ContractMetadata",
		},
		"dcs:documentStructure": map[string]any{
			"@type":      "dcs:DocumentStructure",
			"dcs:blocks": []any{},
			"dcs:layout": []any{},
		},
		"dcs:contractData": []any{},
		"dcs:policies":     []any{},
	}
}

func TestBuildTemplateJSONLDPassesThrough(t *testing.T) {
	input := canonicalTemplateData()
	name := "Test Template"
	template := templatedb.ContractTemplate{
		DID:          "did:web:example:template:1",
		Version:      1,
		State:        "APPROVED",
		TemplateType: "COMPONENT",
		Name:         &name,
		CreatedBy:    "user-1",
		CreatedAt:    fixedTime(),
		UpdatedAt:    fixedTime(),
		TemplateData: newJSON(t, input),
	}

	env, err := BuildTemplateJSONLD(template)
	require.NoError(t, err)

	var got, want map[string]any
	raw, _ := json.Marshal(env)
	_ = json.Unmarshal(raw, &got)
	rawIn, _ := json.Marshal(input)
	_ = json.Unmarshal(rawIn, &want)
	require.Equal(t, want, got)
}

func TestBuildTemplateJSONLDRejectsNonCanonicalFormat(t *testing.T) {
	template := templatedb.ContractTemplate{
		DID:       "did:web:example:template:1",
		CreatedAt: fixedTime(),
		UpdatedAt: fixedTime(),
		TemplateData: newJSON(t, map[string]any{
			"document":     map[string]any{"outline": []any{}, "blocks": []any{}},
			"requirements": []any{},
		}),
	}

	_, err := BuildTemplateJSONLD(template)
	require.Error(t, err)
	require.Contains(t, err.Error(), "canonical JSON-LD")
}

func TestBuildContractJSONLDPassesThrough(t *testing.T) {
	input := canonicalContractData()
	name := "Test Contract"
	contract := contractdb.Contract{
		DID:             "did:web:example:contract:1",
		ContractVersion: 1,
		State:           "DRAFT",
		CreatedBy:       "user-1",
		CreatedAt:       fixedTime(),
		UpdatedAt:       fixedTime(),
		Name:            &name,
		ContractData:    newJSON(t, input),
	}

	env, err := BuildContractJSONLD(contract)
	require.NoError(t, err)

	var got, want map[string]any
	raw, _ := json.Marshal(env)
	_ = json.Unmarshal(raw, &got)
	rawIn, _ := json.Marshal(input)
	_ = json.Unmarshal(rawIn, &want)
	require.Equal(t, want, got)
}

func TestBuildContractJSONLDRejectsNonCanonicalFormat(t *testing.T) {
	contract := contractdb.Contract{
		DID:       "did:web:example:contract:1",
		CreatedAt: fixedTime(),
		UpdatedAt: fixedTime(),
		ContractData: newJSON(t, map[string]any{
			"document":     map[string]any{"outline": []any{}, "blocks": []any{}},
			"requirements": []any{},
		}),
	}

	_, err := BuildContractJSONLD(contract)
	require.Error(t, err)
	require.Contains(t, err.Error(), "canonical JSON-LD")
}
