package validation

import (
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"

	"github.com/stretchr/testify/require"
)

func TestNormalizeContractMutationForPersistencePreservesStoredBundle(t *testing.T) {
	stored := pinnedContractData(t)
	var storedDocument map[string]any
	require.NoError(t, json.Unmarshal(stored, &storedDocument))

	for _, test := range []struct {
		name       string
		clientPins func(map[string]any)
	}{
		{
			name: "omitted",
			clientPins: func(document map[string]any) {
				for _, field := range immutableSemanticBundleFields {
					delete(document, field)
				}
			},
		},
		{
			name: "manipulated",
			clientPins: func(document map[string]any) {
				document["@context"] = "https://attacker.test/semantic/context/evil?version=999"
				document["sh:shapesGraph"] = map[string]any{"@id": "https://attacker.test/semantic/shapes/evil?version=999"}
				document["dcs:effectiveShapes"] = []any{map[string]any{"@id": "https://attacker.test/semantic/shapes/evil?version=999"}}
				document["dcterms:conformsTo"] = map[string]any{"@id": "https://attacker.test/semantic/profile/evil?version=999"}
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			var candidate map[string]any
			require.NoError(t, json.Unmarshal(stored, &candidate))
			test.clientPins(candidate)
			candidate["dcs:metadata"].(map[string]any)["dcs:title"] = "edited"
			raw, err := datatype.NewJSON(candidate)
			require.NoError(t, err)

			normalized, err := NormalizeContractMutationForPersistence(&raw, &stored, "contract-1", false)
			require.NoError(t, err)
			var actual map[string]any
			require.NoError(t, json.Unmarshal(*normalized, &actual))
			for _, field := range immutableSemanticBundleFields {
				require.Equal(t, storedDocument[field], actual[field], field)
			}
			require.Equal(t, "edited", actual["dcs:metadata"].(map[string]any)["dcs:title"])
		})
	}
}

func TestNormalizeContractMutationForPersistenceRejectsIncompleteOrMalformedStoredBundle(t *testing.T) {
	valid := pinnedContractData(t)
	for _, test := range []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"missing context", func(document map[string]any) { delete(document, "@context") }},
		{"unversioned shapes", func(document map[string]any) {
			document["sh:shapesGraph"] = map[string]any{"@id": "https://dcs.test/semantic/shapes/facis-dcs"}
		}},
		{"empty effective shapes", func(document map[string]any) { document["dcs:effectiveShapes"] = []any{} }},
		{"invalid profile", func(document map[string]any) {
			document["dcterms:conformsTo"] = map[string]any{"@id": "https://dcs.test/semantic/profile/facis.sla.basic"}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var document map[string]any
			require.NoError(t, json.Unmarshal(valid, &document))
			test.mutate(document)
			stored, err := datatype.NewJSON(document)
			require.NoError(t, err)

			_, err = NormalizeContractMutationForPersistence(&valid, &stored, "contract-1", false)
			require.ErrorContains(t, err, "stored immutable semantic bundle")
		})
	}
}

func pinnedContractData(t *testing.T) datatype.JSON {
	t.Helper()
	raw := canonicalTemplateData(t)
	var document map[string]any
	require.NoError(t, json.Unmarshal(*raw, &document))
	document["@type"] = "dcs:Contract"
	metadata := document["dcs:metadata"].(map[string]any)
	metadata["@type"] = "dcs:ContractMetadata"
	delete(metadata, "dcs:templateType")
	document["dcs:policies"].(map[string]any)["@type"] = "odrl:Agreement"
	contract, err := datatype.NewJSON(document)
	require.NoError(t, err)
	pinned, err := PinSemanticBundle(
		&contract,
		"https://dcs.test/semantic/context/facis-dcs?version=3",
		"https://dcs.test/semantic/shapes/facis-dcs?version=4",
		[]string{"https://dcs.test/semantic/shapes/facis-dcs?version=4"},
		"https://dcs.test/semantic/profile/facis.sla.basic?version=5",
	)
	require.NoError(t, err)
	return *pinned
}
