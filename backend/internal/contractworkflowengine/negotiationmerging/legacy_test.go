package negotiationmerging

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestContractDataAlreadyEqualsDetectsMaterializedRedline(t *testing.T) {
	doc := map[string]any{
		"@type": "dcs:Contract",
		"dcs:documentStructure": map[string]any{
			"dcs:blocks": map[string]any{"@list": []any{}},
			"dcs:layout": []any{},
		},
		"clause": "already on the formal row",
	}
	raw, err := json.Marshal(doc)
	require.NoError(t, err)

	equal, err := ContractDataAlreadyEquals(doc, raw)
	require.NoError(t, err)
	require.True(t, equal)
}

func TestContractDataAlreadyEqualsRejectsDifferentDocument(t *testing.T) {
	base := map[string]any{
		"@type": "dcs:Contract",
		"dcs:documentStructure": map[string]any{
			"dcs:blocks": map[string]any{"@list": []any{}},
			"dcs:layout": []any{},
		},
		"clause": "formal",
	}
	change := json.RawMessage(`{
		"@type":"dcs:Contract",
		"dcs:documentStructure":{"dcs:blocks":{"@list":[]},"dcs:layout":[]},
		"clause":"redline"
	}`)

	equal, err := ContractDataAlreadyEquals(base, change)
	require.NoError(t, err)
	require.False(t, equal)
}
