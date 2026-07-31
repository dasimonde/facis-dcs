package validation

import (
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"

	"github.com/stretchr/testify/require"
)

func TestNormalizeContractDataForPersistencePreservesTemplateProvenance(t *testing.T) {
	const (
		templateDID = "did:web:facis.example:template:source"
		contractDID = "did:web:facis.example:contract:derived"
	)

	template, err := NormalizeTemplateDataForPersistence(canonicalTemplateData(t), templateDID)
	require.NoError(t, err)

	var contract map[string]any
	require.NoError(t, json.Unmarshal(*template, &contract))
	contract["@type"] = "dcs:Contract"
	contract["dcs:metadata"].(map[string]any)["@type"] = "dcs:ContractMetadata"
	contract["derivedFromTemplate"] = map[string]any{"@id": templateDID, "version": 3}
	raw, err := datatype.NewJSON(contract)
	require.NoError(t, err)

	persisted, err := NormalizeContractDataForPersistence(&raw, contractDID, false)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(*persisted, &contract))

	require.Equal(t, contractDID, contract["@id"])
	require.Equal(t, templateDID, contract["derivedFromTemplate"].(map[string]any)["@id"])
	structure := contract["dcs:documentStructure"].(map[string]any)
	block := structure["dcs:blocks"].(map[string]any)["@list"].([]any)[0].(map[string]any)
	require.Equal(t, contractDID+"#block-clause-1", block["@id"])
}
