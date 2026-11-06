package validation

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSearchProjectionJSONPreservesNumericPrecision(t *testing.T) {
	contextJSON := `{"@context":{"@version":1.1,"dcs":"https://w3id.org/facis/dcs/ontology/v1#","ex":"https://example.org/"}}`
	defer SwapShapeSource(fixtureShapeSource{contextJSON: contextJSON})()
	raw := []byte(`{"@context":{"@version":1.1},"dcs:contractFields":[{"dcs:shape":{"@id":"ex:field"},"dcs:value":18446744073709551615}],"ex:direct":12345678901234567890.123456789}`)
	projection, err := SearchProjectionJSON(context.Background(), raw)
	require.NoError(t, err)
	var values map[string][]any
	decoder := json.NewDecoder(bytes.NewReader(projection))
	decoder.UseNumber()
	require.NoError(t, decoder.Decode(&values))
	require.Equal(t, json.Number("18446744073709551615"), values["https://example.org/field"][0])
	require.Equal(t, json.Number("12345678901234567890.123456789"), values["https://example.org/direct"][0])
}

func TestSearchProjectionExpandsDocumentContext(t *testing.T) {
	contextJSON := `{"@context":{"dcs":"https://w3id.org/facis/dcs/ontology/v1#","dcst":"https://w3id.org/facis/dcs/taxonomy/v1#"}}`
	document := map[string]any{
		"dcs:contractFields": []any{map[string]any{
			"dcs:shape": map[string]any{"@id": "dcst:field-contract-jurisdiction"},
			"dcs:value": "DEU",
		}},
	}
	projection, err := SearchProjection(context.Background(), document, fixtureShapeSource{contextJSON: contextJSON})
	require.NoError(t, err)
	require.Equal(t, []any{"DEU"}, projection["https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction"])
}

func TestExpandedSearchProjectionUsesFilledValuesAndFullIRIs(t *testing.T) {
	dcs := dcsNamespace()
	field := "https://example.org/field/jurisdiction"
	property := "https://example.org/property/count"
	root := map[string]any{
		dcs + "contractFields": []any{
			map[string]any{dcs + "shape": []any{map[string]any{"@id": field}}, dcs + "value": []any{map[string]any{"@value": false}, map[string]any{"@value": float64(0)}, map[string]any{"@value": ""}}},
			map[string]any{dcs + "shape": []any{map[string]any{"@id": "https://other.example/field/jurisdiction"}}},
		},
		property:                []any{map[string]any{"@value": "3"}},
		dcs + "policies":        []any{map[string]any{odrlIRI + "rightOperand": []any{map[string]any{"@value": "not a filled value"}}}},
		dcs + "valueConstraint": []any{map[string]any{property: []any{map[string]any{"@value": "allowed but unfilled"}}}},
		"@graph":                []any{map[string]any{"@type": []any{odrlIRI + "Constraint"}, property: []any{map[string]any{"@value": "not a filled value"}}}},
	}
	projection := expandedSearchProjection(root)
	require.Equal(t, []any{false, float64(0)}, projection[field])
	require.Equal(t, []any{"3"}, projection[property])
	require.NotContains(t, projection, "https://other.example/field/jurisdiction")
	require.NotContains(t, projection, odrlIRI+"rightOperand")
}

func TestSearchProjectionResolvesFieldReferences(t *testing.T) {
	dcs := dcsNamespace()
	property := "https://example.org/property/jurisdiction"
	emptyProperty := "https://example.org/property/empty"
	projection := expandedSearchProjection(map[string]any{"@graph": []any{
		map[string]any{property: []any{map[string]any{"@id": "urn:field:filled"}}, emptyProperty: []any{map[string]any{"@id": "urn:field:empty"}}},
		map[string]any{"https://example.org/nested": []any{map[string]any{"@id": "urn:field:filled", dcs + "shape": []any{map[string]any{"@id": "urn:shape"}}, dcs + "value": []any{map[string]any{"@value": "Germany"}}}}},
		map[string]any{"@id": "urn:field:empty", "@type": []any{dcs + "ContractField"}},
	}})
	require.Equal(t, []any{"Germany"}, projection[property])
	require.NotContains(t, projection, emptyProperty)
}

func TestSearchProjectionIndexesJSONLDLists(t *testing.T) {
	dcs := dcsNamespace()
	field := "https://example.org/field"
	property := "https://example.org/property"
	values := []any{map[string]any{"@value": "one"}, map[string]any{"@value": "two"}}
	projection := expandedSearchProjection(map[string]any{
		dcs + "contractFields": []any{map[string]any{
			dcs + "shape": []any{map[string]any{"@id": field}},
			dcs + "value": []any{map[string]any{"@list": values}},
		}},
		property: []any{map[string]any{"@list": values}},
	})
	require.Equal(t, []any{"one", "two"}, projection[field])
	require.Equal(t, []any{"one", "two"}, projection[property])
}

func TestSearchProjectionResolvesFieldReferencesInLists(t *testing.T) {
	dcs := dcsNamespace()
	property := "https://example.org/property"
	projection := expandedSearchProjection(map[string]any{"@graph": []any{
		map[string]any{property: []any{map[string]any{"@list": []any{
			map[string]any{"@id": "urn:filled"}, map[string]any{"@id": "urn:empty"},
		}}}},
		map[string]any{"@id": "urn:filled", "@type": []any{dcs + "ContractField"}, dcs + "value": []any{map[string]any{"@value": "Germany"}}},
		map[string]any{"@id": "urn:empty", "@type": []any{dcs + "ContractField"}},
	}})
	require.Equal(t, []any{"Germany"}, projection[property])
}

func TestSearchProjectionIndexesTypedIRIValues(t *testing.T) {
	property := "https://example.org/property"
	contextJSON := `{"@context":{"ex":"https://example.org/"}}`
	projection, err := SearchProjection(context.Background(), map[string]any{"ex:property": []any{
		map[string]any{"@id": "ex:value"},
		map[string]any{"@id": "ex:value", "@type": "ex:Class"},
	}}, fixtureShapeSource{contextJSON: contextJSON})
	require.NoError(t, err)
	require.Equal(t, []any{"https://example.org/value", "https://example.org/value"}, projection[property])
}

func TestSearchProjectionResolvesSeparateJSONLDGraphNodes(t *testing.T) {
	contextJSON := `{"@context":{"dcs":"https://w3id.org/facis/dcs/ontology/v1#","ex":"https://example.org/"}}`
	document := map[string]any{"@graph": []any{
		map[string]any{"@id": "urn:root", "ex:jurisdiction": map[string]any{"@id": "urn:filled"}, "ex:empty": map[string]any{"@id": "urn:empty"}},
		map[string]any{"@id": "urn:filled", "@type": "dcs:ContractField", "dcs:shape": map[string]any{"@id": "ex:shape"}},
		map[string]any{"@id": "urn:filled", "dcs:value": "Germany"},
		map[string]any{"@id": "urn:empty", "@type": "dcs:ContractField"},
	}}
	projection, err := SearchProjection(context.Background(), document, fixtureShapeSource{contextJSON: contextJSON})
	require.NoError(t, err)
	require.Equal(t, []any{"Germany"}, projection["https://example.org/jurisdiction"])
	require.Equal(t, []any{"Germany"}, projection["https://example.org/shape"])
	require.NotContains(t, projection, "https://example.org/empty")
}
