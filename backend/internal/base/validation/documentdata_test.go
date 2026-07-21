package validation

import (
	"encoding/json"
	"os"
	"testing"

	"digital-contracting-service/internal/base/datatype"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// ADR-8/ADR-9: AuditContractContent's SHACL/profile enforcement reads
	// from the Semantic Hub only (no disk fallback) — tests install a
	// ShapeSource fixture backed by the real hub authoring files so the
	// real goRDFlib SHACL engine runs end to end without a live database
	// (see contractcontentaudit_test.go).
	SetShapeSource(fixtureShapeSource{
		shapesTTL:   mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-shapes.ttl"),
		profileYAML: mustReadRepoFile("backend/internal/semantichub/assets/facis.sla.basic.v1.yaml"),
		contextJSON: mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-context.jsonld"),
	})
	os.Exit(m.Run())
}

func validTemplateData(t *testing.T) *datatype.JSON {
	t.Helper()
	data, err := datatype.NewJSON(map[string]any{
		"documentOutline": []any{
			map[string]any{"blockId": "root", "isRoot": true, "children": []any{"clause-1"}},
		},
		"documentBlocks": []any{
			map[string]any{"blockId": "clause-1", "type": "CLAUSE", "text": "Availability {{cond-1.percent}}", "conditionIds": []any{"cond-1"}},
		},
		"semanticConditions": []any{
			map[string]any{
				"conditionId":   "cond-1",
				"conditionName": "Availability",
				"schemaVersion": "v1",
				"parameters": []any{
					map[string]any{
						"parameterName": "percent",
						"type":          "decimal",
						"fieldIri":      "https://w3id.org/facis/dcs/taxonomy/v1#field-service-sla-availability",
						"isRequired":    true,
						"operators":     []any{},
					},
				},
			},
		},
		"customMetaData": []any{},
	})
	require.NoError(t, err)
	return &data
}

func canonicalTemplateData(t *testing.T) *datatype.JSON {
	t.Helper()
	data, err := datatype.NewJSON(map[string]any{
		"@context": map[string]any{
			"dcs":  "https://w3id.org/facis/dcs/ontology/v1#",
			"odrl": "http://www.w3.org/ns/odrl/2/",
		},
		"@type": "dcs:ContractTemplate",
		"dcs:metadata": map[string]any{
			"@type":            "dcs:TemplateMetadata",
			"dcs:title":        "Provider eligibility",
			"dcs:templateType": "dcs:Component",
		},
		"dcs:documentStructure": map[string]any{
			"@type": "dcs:DocumentStructure",
			"dcs:blocks": map[string]any{"@list": []any{
				map[string]any{
					"@id":   "urn:uuid:block-clause-1",
					"@type": "dcs:Clause",
					"dcs:content": map[string]any{"@list": []any{
						"Provider country: ",
						map[string]any{"@id": "urn:uuid:field-provider-country"},
					}},
				},
			}},
			"dcs:layout": []any{
				map[string]any{
					"@id":          "urn:uuid:block-root",
					"dcs:isRoot":   true,
					"dcs:children": map[string]any{"@list": []any{map[string]any{"@id": "urn:uuid:block-clause-1"}}},
				},
			},
		},
		"dcs:contractData": []any{
			map[string]any{
				"@id":                 "urn:uuid:field-provider-country",
				"@type":               "dcs:Placeholder",
				"dcs:label":           "Provider country",
				"dcs:datatype":        "xsd:string",
				"dcs:shape":           map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-company-location-country"},
				"dcs:valueConstraint": map[string]any{"format": "iso-3166-1-alpha-3"},
				"dcs:required":        true,
			},
		},
		"dcs:policies": map[string]any{
			"@id":          "urn:uuid:policy-set-1",
			"@type":        "odrl:Offer",
			"odrl:profile": map[string]any{"@id": "https://w3id.org/facis/dcs/ontology/v1/odrl-profile"},
			"odrl:obligation": []any{
				map[string]any{
					"@id":           "urn:uuid:policy-provider-country-0",
					"@type":         "odrl:Duty",
					"odrl:action":   map[string]any{"@id": "dcs:provideCompliantValue"},
					"odrl:assigner": map[string]any{"@id": "urn:uuid:party-provider"},
					"odrl:assignee": map[string]any{"@id": "urn:uuid:party-customer"},
					"odrl:target":   map[string]any{"@id": "urn:uuid:policy-target"},
					"dcs:prose":     map[string]any{"@id": "urn:uuid:block-clause-1"},
					"odrl:constraint": map[string]any{
						"@type":             "odrl:Constraint",
						"odrl:leftOperand":  map[string]any{"@id": "urn:uuid:field-provider-country"},
						"odrl:operator":     map[string]any{"@id": "odrl:isAnyOf"},
						"odrl:rightOperand": []any{"DEU", "AUT", "CHE"},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	return &data
}

// firstPolicyDuty returns the first odrl:duty rule node from the canonical
// dcs:policies odrl:Set structure produced by canonicalTemplateData.
func firstPolicyDuty(data map[string]any) map[string]any {
	set := data["dcs:policies"].(map[string]any)
	duties := set["odrl:obligation"].([]any)
	return duties[0].(map[string]any)
}

func TestNormalizeTemplateDataRejectsNonCanonicalStructure(t *testing.T) {
	_, err := NormalizeTemplateData(validTemplateData(t))
	require.ErrorContains(t, err, "canonical dcs:documentStructure envelope")
}

func TestNormalizeContractDataRejectsNonCanonicalStructure(t *testing.T) {
	_, err := NormalizeContractData(validTemplateData(t), false)
	require.ErrorContains(t, err, "canonical dcs:documentStructure envelope")
}

func TestNormalizeTemplateDataAcceptsCanonicalJSONLDEnvelope(t *testing.T) {
	normalized, err := NormalizeTemplateData(canonicalTemplateData(t))
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &result))
	require.Contains(t, result, "dcs:documentStructure")
	require.NotContains(t, result, "documentOutline")
	// normalizeCanonicalContext anchors @context as [hub context URL,
	// submitted inline prefix map] (ADR-8).
	anchored := result["@context"].([]any)
	require.Equal(t, SchemaJSONLDContextV1, anchored[0])
	require.Equal(t, "https://w3id.org/facis/dcs/ontology/v1#", anchored[1].(map[string]any)["dcs"])
	// The shapes pin rides on sh:shapesGraph (the ADR-8 anchor).
	require.Equal(t, SchemaSHACLShapesV1, result["sh:shapesGraph"].(map[string]any)["@id"])
	require.NotContains(t, result, "dcs:schemaRefs")
}

func TestNormalizeTemplateDataForPersistenceAddsDocumentIdentity(t *testing.T) {
	normalized, err := NormalizeTemplateDataForPersistence(canonicalTemplateData(t), "did:web:facis.example:template:1")
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &data))
	require.Equal(t, "did:web:facis.example:template:1", data["@id"])
	require.NotContains(t, data, "did")
	structure := data["dcs:documentStructure"].(map[string]any)
	block := structure["dcs:blocks"].(map[string]any)["@list"].([]any)[0].(map[string]any)
	require.Equal(t, "did:web:facis.example:template:1#block-clause-1", block["@id"])
	placeholder := block["dcs:content"].(map[string]any)["@list"].([]any)[1].(map[string]any)
	require.Equal(t, "did:web:facis.example:template:1#field-provider-country", placeholder["@id"])
	policy := firstPolicyDuty(data)
	constraint := policy["odrl:constraint"].(map[string]any)
	require.Equal(t, "did:web:facis.example:template:1#field-provider-country", constraint["odrl:leftOperand"].(map[string]any)["@id"])
}

func TestNormalizeTemplateDataForPersistenceRebasesCopiedTemplateIDs(t *testing.T) {
	first, err := NormalizeTemplateDataForPersistence(canonicalTemplateData(t), "did:web:facis.example:template:source")
	require.NoError(t, err)
	copied, err := NormalizeTemplateDataForPersistence(first, "did:web:facis.example:template:copy")
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(*copied, &data))
	structure := data["dcs:documentStructure"].(map[string]any)
	block := structure["dcs:blocks"].(map[string]any)["@list"].([]any)[0].(map[string]any)
	require.Equal(t, "did:web:facis.example:template:copy#block-clause-1", block["@id"])
	policy := firstPolicyDuty(data)
	constraint := policy["odrl:constraint"].(map[string]any)
	require.Equal(t, "did:web:facis.example:template:copy#field-provider-country", constraint["odrl:leftOperand"].(map[string]any)["@id"])
}

func TestNormalizeTemplateDataRejectsMissingPlaceholderField(t *testing.T) {
	raw := canonicalTemplateData(t)
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	structure := data["dcs:documentStructure"].(map[string]any)
	block := structure["dcs:blocks"].(map[string]any)["@list"].([]any)[0].(map[string]any)
	placeholder := block["dcs:content"].(map[string]any)["@list"].([]any)[1].(map[string]any)
	placeholder["@id"] = "urn:uuid:field-missing"
	invalid, err := datatype.NewJSON(data)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&invalid)
	require.ErrorContains(t, err, "placeholder references nonexistent contract data field")
}

func TestNormalizeTemplateDataRejectsMissingPolicyField(t *testing.T) {
	raw := canonicalTemplateData(t)
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	policy := firstPolicyDuty(data)
	constraint := policy["odrl:constraint"].(map[string]any)
	constraint["odrl:leftOperand"] = map[string]any{"@id": "urn:uuid:field-missing"}
	invalid, err := datatype.NewJSON(data)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&invalid)
	require.ErrorContains(t, err, "policy references nonexistent contract data field")
}

// TestNormalizeTemplateDataAcceptsConstraintConjunctionWithContextOperand
// proves the greenfield array-constraint shape validates: a rule's
// odrl:constraint is a conjunction (an array), and an ODRL context operand
// (spatial) is accepted as use-time access context rather than rejected as a
// nonexistent data field.
func TestNormalizeTemplateDataAcceptsConstraintConjunctionWithContextOperand(t *testing.T) {
	raw := canonicalTemplateData(t)
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	policy := firstPolicyDuty(data)
	policy["odrl:constraint"] = []any{
		map[string]any{
			"@type":             "odrl:Constraint",
			"odrl:leftOperand":  map[string]any{"@id": "urn:uuid:field-provider-country"},
			"odrl:operator":     map[string]any{"@id": "odrl:isAnyOf"},
			"odrl:rightOperand": []any{"DEU", "AUT", "CHE"},
		},
		map[string]any{
			"@type":             "odrl:Constraint",
			"odrl:leftOperand":  map[string]any{"@id": "odrl:spatial"},
			"odrl:operator":     map[string]any{"@id": "odrl:eq"},
			"odrl:rightOperand": map[string]any{"@value": "DE", "@type": "xsd:string"},
		},
	}
	valid, err := datatype.NewJSON(data)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&valid)
	require.NoError(t, err)
}

// permissionWithDuty builds a Permission (copying the canonical rule's parties
// and prose) carrying the given odrl:duty payload, and installs it as the
// policy set's permission bucket.
func permissionWithDuty(t *testing.T, duty any) datatype.JSON {
	t.Helper()
	raw := canonicalTemplateData(t)
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	base := firstPolicyDuty(data)
	permission := map[string]any{
		"@id":           "urn:uuid:policy-permission-1",
		"@type":         "odrl:Permission",
		"odrl:action":   map[string]any{"@id": "odrl:use"},
		"odrl:assigner": base["odrl:assigner"],
		"odrl:assignee": base["odrl:assignee"],
		"odrl:target":   base["odrl:target"],
		"dcs:prose":     base["dcs:prose"],
		"odrl:duty":     duty,
	}
	data["dcs:policies"].(map[string]any)["odrl:permission"] = []any{permission}
	out, err := datatype.NewJSON(data)
	require.NoError(t, err)
	return out
}

// TestNormalizeTemplateDataAcceptsPermissionWithNestedDuty proves a Permission
// may carry a nested Duty fragment (ODRL IM §2.5): its own action plus a
// constraint on an existing data field, no parties of its own.
func TestNormalizeTemplateDataAcceptsPermissionWithNestedDuty(t *testing.T) {
	valid := permissionWithDuty(t, []any{
		map[string]any{
			"@type":       "odrl:Duty",
			"odrl:action": map[string]any{"@id": "odrl:compensate"},
			"odrl:constraint": []any{
				map[string]any{
					"@type":             "odrl:Constraint",
					"odrl:leftOperand":  map[string]any{"@id": "urn:uuid:field-provider-country"},
					"odrl:operator":     map[string]any{"@id": "odrl:isAnyOf"},
					"odrl:rightOperand": []any{"DEU"},
				},
			},
		},
	})
	_, err := NormalizeTemplateData(&valid)
	require.NoError(t, err)
}

// TestNormalizeTemplateDataRejectsDutyWithoutAction proves a nested duty must
// declare an action.
func TestNormalizeTemplateDataRejectsDutyWithoutAction(t *testing.T) {
	invalid := permissionWithDuty(t, []any{
		map[string]any{"@type": "odrl:Duty"},
	})
	_, err := NormalizeTemplateData(&invalid)
	require.ErrorContains(t, err, "duty is missing odrl:action")
}

// TestNormalizeTemplateDataRejectsDutyConstraintOnUnknownField proves a nested
// duty's constraints are checked against declared fields like a rule's own.
func TestNormalizeTemplateDataRejectsDutyConstraintOnUnknownField(t *testing.T) {
	invalid := permissionWithDuty(t, []any{
		map[string]any{
			"@type":       "odrl:Duty",
			"odrl:action": map[string]any{"@id": "odrl:compensate"},
			"odrl:constraint": []any{
				map[string]any{
					"@type":            "odrl:Constraint",
					"odrl:leftOperand": map[string]any{"@id": "urn:uuid:field-does-not-exist"},
					"odrl:operator":    map[string]any{"@id": "odrl:eq"},
				},
			},
		},
	})
	_, err := NormalizeTemplateData(&invalid)
	require.ErrorContains(t, err, "nonexistent contract data field")
}

// TestNormalizeTemplateDataRejectsDutyOnNonPermission proves odrl:duty may only
// nest under a Permission — a policy-level Duty belongs under odrl:obligation.
func TestNormalizeTemplateDataRejectsDutyOnNonPermission(t *testing.T) {
	raw := canonicalTemplateData(t)
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	obligation := firstPolicyDuty(data)
	obligation["odrl:duty"] = []any{
		map[string]any{"@type": "odrl:Duty", "odrl:action": map[string]any{"@id": "odrl:compensate"}},
	}
	invalid, err := datatype.NewJSON(data)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&invalid)
	require.ErrorContains(t, err, "may only be attached to a Permission")
}

// TestNormalizeTemplateDataAcceptsExtendedContextOperands proves the full ODRL
// core Left Operand vocabulary (beyond spatial/dateTime) is recognized as
// use-time context and accepted, not rejected as a nonexistent data field.
func TestNormalizeTemplateDataAcceptsExtendedContextOperands(t *testing.T) {
	raw := canonicalTemplateData(t)
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	policy := firstPolicyDuty(data)
	policy["odrl:constraint"] = []any{
		map[string]any{
			"@type":             "odrl:Constraint",
			"odrl:leftOperand":  map[string]any{"@id": "odrl:elapsedTime"},
			"odrl:operator":     map[string]any{"@id": "odrl:lteq"},
			"odrl:rightOperand": map[string]any{"@value": "P30D", "@type": "xsd:duration"},
		},
		map[string]any{
			"@type":             "odrl:Constraint",
			"odrl:leftOperand":  map[string]any{"@id": "odrl:virtualLocation"},
			"odrl:operator":     map[string]any{"@id": "odrl:eq"},
			"odrl:rightOperand": map[string]any{"@value": "https://vr.example/room1", "@type": "xsd:string"},
		},
	}
	valid, err := datatype.NewJSON(data)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&valid)
	require.NoError(t, err)
}

func TestNormalizeTemplateDataRejectsUnreferencedBlock(t *testing.T) {
	raw := canonicalTemplateData(t)
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	structure := data["dcs:documentStructure"].(map[string]any)
	blocksWrapper := structure["dcs:blocks"].(map[string]any)
	blocksWrapper["@list"] = append(blocksWrapper["@list"].([]any), map[string]any{
		"@id":      "urn:uuid:block-unreferenced",
		"@type":    "dcs:TextBlock",
		"dcs:text": "unused",
	})
	invalid, err := datatype.NewJSON(data)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&invalid)
	require.ErrorContains(t, err, "is not referenced by layout")
}

func TestNormalizeTemplateDataAcceptsUnreferencedClause(t *testing.T) {
	raw := canonicalTemplateData(t)
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	structure := data["dcs:documentStructure"].(map[string]any)
	blocksWrapper := structure["dcs:blocks"].(map[string]any)
	blocksWrapper["@list"] = append(blocksWrapper["@list"].([]any), map[string]any{
		"@id":         "urn:uuid:block-clause-pool",
		"@type":       "dcs:Clause",
		"dcs:title":   "Reusable clause",
		"dcs:content": map[string]any{"@list": []any{"Reusable content"}},
	})
	contract, err := datatype.NewJSON(data)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&contract)
	require.NoError(t, err)
}

func TestValidateContractSemanticsAcceptsCanonicalContract(t *testing.T) {
	raw := canonicalTemplateData(t)
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	data["@type"] = "dcs:Contract"
	data["dcs:policies"].(map[string]any)["@type"] = "odrl:Agreement"
	contract, err := datatype.NewJSON(data)
	require.NoError(t, err)

	require.NoError(t, ValidateContractSemantics(&contract))
}
