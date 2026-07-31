package negotiationmerging

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMergeContractDataChangeAcceptsCanonicalContractData(t *testing.T) {
	stored := map[string]any{
		"semanticConditionValues": []any{
			map[string]any{
				"blockId":        "block-1",
				"conditionId":    "provider",
				"parameterName":  "country",
				"parameterValue": "DEU",
			},
		},
	}
	canonicalChange := json.RawMessage(`{
		"@type": "dcs:Contract",
		"dcs:documentStructure": {
			"dcs:blocks": { "@list": [] },
			"dcs:layout": []
		},
		"dcs:contractData": [],
		"dcs:policies": [],
		"semanticConditionValues": []
	}`)

	merged, err := mergeContractDataChange(stored, canonicalChange)

	require.NoError(t, err)
	require.Equal(t, "dcs:Contract", merged["@type"])
	require.Contains(t, merged, "dcs:documentStructure")
	require.Empty(t, merged["semanticConditionValues"])
}

func TestMergeContractDataChangeRejectsNonCanonicalChange(t *testing.T) {
	stored := map[string]any{"dcs:documentStructure": map[string]any{}}
	partialChange := json.RawMessage(`{
		"semanticConditionValues": [
			{"forField": "urn:uuid:field-provider-country", "parameterValue": "AUT"}
		]
	}`)

	_, err := mergeContractDataChange(stored, partialChange)

	require.ErrorContains(t, err, "canonical dcs:documentStructure envelope")
}
