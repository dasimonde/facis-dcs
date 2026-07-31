package contracttemplate

import (
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"

	"github.com/stretchr/testify/require"
)

func TestConvertTemplateDataToContractDataKeepsCanonicalContent(t *testing.T) {
	raw, err := datatype.NewJSON(map[string]any{
		"@context": map[string]any{
			"dcs":  "https://w3id.org/facis/dcs/ontology/v1#",
			"odrl": "http://www.w3.org/ns/odrl/2/",
		},
		"@id":   "did:web:facis.example:template:1",
		"@type": "dcs:ContractTemplate",
		"dcs:metadata": map[string]any{
			"@type":            "dcs:TemplateMetadata",
			"dcs:templateType": "dcs:Component",
		},
		"dcs:documentStructure": map[string]any{
			"@type": "dcs:DocumentStructure",
			"dcs:blocks": map[string]any{"@list": []any{
				map[string]any{
					"@id":   "did:web:facis.example:template:1#block-clause-1",
					"@type": "dcs:Clause",
					"dcs:content": map[string]any{"@list": []any{
						"Availability ",
						map[string]any{"@id": "did:web:facis.example:template:1#field-cond-1-percent"},
					}},
				},
			}},
			"dcs:layout": []any{
				map[string]any{
					"@id":          "did:web:facis.example:template:1#block-root",
					"dcs:isRoot":   true,
					"dcs:children": map[string]any{"@list": []any{map[string]any{"@id": "did:web:facis.example:template:1#block-clause-1"}}},
				},
			},
		},
		"dcs:contractFields": []any{
			map[string]any{
				"@id":          "did:web:facis.example:template:1#field-cond-1-percent",
				"@type":        "dcs:ContractField",
				"dcs:label":    "Availability",
				"dcs:datatype": "xsd:decimal",
				"dcs:shape":    map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-service-sla-availability"},
				"dcs:required": true,
			},
		},
		"dcs:policies": map[string]any{
			"@id":          "did:web:facis.example:template:1#policy-set-1",
			"@type":        "odrl:Offer",
			"odrl:profile": map[string]any{"@id": "https://w3id.org/facis/dcs/ontology/v1/odrl-profile"},
			"odrl:obligation": []any{
				map[string]any{
					"@id":           "did:web:facis.example:template:1#policy-cond-1-percent-0",
					"@type":         "odrl:Duty",
					"odrl:action":   map[string]any{"@id": "dcs:provideCompliantValue"},
					"odrl:assigner": map[string]any{"@id": "did:web:facis.example:template:1#party-provider"},
					"odrl:assignee": map[string]any{"@id": "did:web:facis.example:template:1#party-customer"},
					"odrl:target":   map[string]any{"@id": "did:web:facis.example:template:1"},
					"dcs:prose":     map[string]any{"@id": "urn:uuid:block-clause-1"},
					"odrl:constraint": map[string]any{
						"@type":             "odrl:Constraint",
						"odrl:leftOperand":  map[string]any{"@id": "did:web:facis.example:template:1#field-cond-1-percent"},
						"odrl:operator":     map[string]any{"@id": "odrl:gteq"},
						"odrl:rightOperand": map[string]any{"@value": "99.95", "@type": "xsd:decimal"},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	converted, err := ConvertTemplateDataToContractData(&raw, "did:web:facis.example:template:1", 7)
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(*converted, &data))
	require.Equal(t, "dcs:Contract", data["@type"])
	provenance := data["derivedFromTemplate"].(map[string]any)
	require.Equal(t, "did:web:facis.example:template:1", provenance["@id"])
	require.Equal(t, float64(7), provenance["version"])
	require.Empty(t, data["semanticConditionValues"])
	parties := data["dcs:parties"].([]any)
	require.Len(t, parties, 2)
	provider := parties[0].(map[string]any)
	require.Equal(t, "did:web:facis.example:template:1#party-provider", provider["@id"])
	require.Equal(t, "dcs:CompanyParty", provider["@type"])
	require.Equal(t, "provider", provider["dcs:role"])
	structure := data["dcs:documentStructure"].(map[string]any)
	require.Len(t, structure["dcs:blocks"].(map[string]any)["@list"], 1)
	require.Len(t, data["dcs:contractFields"], 1)

	persisted, err := validation.NormalizeContractDataForPersistence(
		converted,
		"did:web:facis.example:contract:1",
		false,
	)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(*persisted, &data))
	require.Equal(t, "did:web:facis.example:contract:1", data["@id"])
	structure = data["dcs:documentStructure"].(map[string]any)
	block := structure["dcs:blocks"].(map[string]any)["@list"].([]any)[0].(map[string]any)
	require.Equal(t, "did:web:facis.example:contract:1#block-clause-1", block["@id"])
	fieldReference := block["dcs:content"].(map[string]any)["@list"].([]any)[1].(map[string]any)
	require.Equal(
		t,
		"did:web:facis.example:contract:1#field-cond-1-percent",
		fieldReference["@id"],
	)
	policySet := data["dcs:policies"].(map[string]any)
	policy := policySet["odrl:obligation"].([]any)[0].(map[string]any)
	constraint := policy["odrl:constraint"].(map[string]any)
	require.Equal(
		t,
		"did:web:facis.example:contract:1#field-cond-1-percent",
		constraint["odrl:leftOperand"].(map[string]any)["@id"],
	)
}

func TestConvertTemplateDataToContractDataRejectsNonCanonicalTemplate(t *testing.T) {
	raw, err := datatype.NewJSON(map[string]any{"documentBlocks": []any{}})
	require.NoError(t, err)

	_, err = ConvertTemplateDataToContractData(&raw, "did:web:facis.example:template:1")
	require.ErrorContains(t, err, "canonical dcs:documentStructure envelope")
}
