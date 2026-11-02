package contracttemplate

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/datatype"
)

// DCS-FR-TR-12, DCS-FR-CWE-13: a contract created from a template takes the
// terms its SLA rules fix ("field odrl:eq literal") as filled values. The
// region field is the contract-jurisdiction concept, so the jurisdiction is
// filled in and searchable (contract search jurisdiction filter).
const jurisdictionShape = "https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction"

func TestContractTakesTheTermsTheSLARulesFix(t *testing.T) {
	const base = "did:web:facis.example:template:1"
	constraint := func(field, operator string, right any) map[string]any {
		return map[string]any{
			"@type":             "odrl:Constraint",
			"odrl:leftOperand":  map[string]any{"@id": base + "#" + field},
			"odrl:operator":     map[string]any{"@id": operator},
			"odrl:rightOperand": right,
		}
	}
	field := func(name, datatype string, extra map[string]any) map[string]any {
		f := map[string]any{"@id": base + "#" + name, "@type": "dcs:ContractField", "dcs:label": name, "dcs:datatype": datatype, "dcs:required": true}
		for k, v := range extra {
			f[k] = v
		}
		return f
	}
	raw, err := datatype.NewJSON(map[string]any{
		"@context":     map[string]any{"dcs": "https://w3id.org/facis/dcs/ontology/v1#", "odrl": "http://www.w3.org/ns/odrl/2/"},
		"@id":          base,
		"@type":        "dcs:ContractTemplate",
		"dcs:metadata": map[string]any{"@type": "dcs:TemplateMetadata", "dcs:templateType": "dcs:ContractTemplate"},
		"dcs:documentStructure": map[string]any{
			"@type": "dcs:DocumentStructure",
			"dcs:blocks": map[string]any{"@list": []any{
				map[string]any{"@id": base + "#block-clause-1", "@type": "dcs:Clause", "dcs:content": map[string]any{"@list": []any{"Terms"}}},
			}},
			"dcs:layout": []any{map[string]any{"@id": base + "#block-root", "dcs:isRoot": true,
				"dcs:children": map[string]any{"@list": []any{map[string]any{"@id": base + "#block-clause-1"}}}}},
		},
		"dcs:contractFields": []any{
			field("support-hours", "xsd:integer", nil),
			field("region", "xsd:string", map[string]any{"dcs:shape": map[string]any{"@id": jurisdictionShape}}),
			field("availability", "xsd:decimal", nil),
			field("fee", "xsd:decimal", map[string]any{"dcs:value": map[string]any{"@value": "10", "@type": "xsd:decimal"}}),
		},
		"dcs:policies": map[string]any{
			"@id":          base + "#policy-set-1",
			"@type":        "odrl:Offer",
			"odrl:profile": map[string]any{"@id": "https://w3id.org/facis/dcs/ontology/v1/odrl-profile"},
			"odrl:obligation": []any{map[string]any{
				"@type":         "odrl:Duty",
				"odrl:action":   map[string]any{"@id": "dcs:provideCompliantValue"},
				"@id":           base + "#policy-1",
				"odrl:target":   map[string]any{"@id": base},
				"odrl:assigner": map[string]any{"@id": base + "#party-https%3A%2F%2Fw3id.org%2Ffacis%2Fdcs%2Ftaxonomy%2Fv1%23role-provider"},
				"odrl:assignee": map[string]any{"@id": base + "#party-https%3A%2F%2Fw3id.org%2Ffacis%2Fdcs%2Ftaxonomy%2Fv1%23role-customer"},
				"dcs:prose":     map[string]any{"@id": base + "#block-clause-1"},
				"odrl:constraint": []any{
					constraint("support-hours", "odrl:eq", map[string]any{"@value": "24", "@type": "xsd:integer"}),
					constraint("region", "odrl:eq", "DEU"),
					constraint("availability", "odrl:gteq", map[string]any{"@value": "99.5", "@type": "xsd:decimal"}),
					constraint("fee", "odrl:eq", map[string]any{"@value": "20", "@type": "xsd:decimal"}),
				},
			}},
		},
	})
	require.NoError(t, err)

	contract, err := ConvertTemplateDataToContractData(&raw, base)
	require.NoError(t, err)
	var doc map[string]any
	require.NoError(t, json.Unmarshal(*contract, &doc))
	values := map[string]any{}
	shapes := map[string]any{}
	for _, raw := range doc["dcs:contractFields"].([]any) {
		f := raw.(map[string]any)
		values[f["@id"].(string)] = f["dcs:value"]
		shapes[f["@id"].(string)] = f["dcs:shape"]
	}
	require.Equal(t, map[string]any{"@value": "24", "@type": "xsd:integer"}, values[base+"#support-hours"])
	require.Equal(t, map[string]any{"@value": "DEU", "@type": "xsd:string"}, values[base+"#region"])
	require.Equal(t, map[string]any{"@id": jurisdictionShape}, shapes[base+"#region"], "the jurisdiction field keeps its concept")
	require.Nil(t, values[base+"#availability"], "a bound is not a fixed term")
	require.Equal(t, map[string]any{"@value": "10", "@type": "xsd:decimal"}, values[base+"#fee"], "a filled value is kept")
}
