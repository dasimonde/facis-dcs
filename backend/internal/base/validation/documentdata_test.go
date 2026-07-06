package validation

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"digital-contracting-service/internal/base/datatype"

	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	SetJSONLDContextIRI(SchemaJSONLDContextV1)
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
						"schemaRef":     SchemaServiceV1,
						"semanticPath":  "service.sla.availability",
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
						map[string]any{
							"@type":       "dcs:Placeholder",
							"dcs:token":   "{{provider.country}}",
							"dcs:bindsTo": map[string]any{"@id": "urn:uuid:field-provider-country"},
						},
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
				"@id":               "urn:uuid:requirement-provider",
				"@type":             "dcs:DataRequirement",
				"dcs:conditionId":   "provider",
				"dcs:name":          "Provider",
				"dcs:schemaVersion": "v1",
				"dcs:entityType":    "CompanyParty",
				"dcs:entityRole":    "provider",
				"dcs:fields": []any{
					map[string]any{
						"@id":               "urn:uuid:field-provider-country",
						"@type":             "dcs:RequirementField",
						"dcs:parameterName": "country",
						"dcs:domainField":   map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-company-location-country"},
						"dcs:required":      true,
					},
				},
			},
		},
		"dcs:policies": []any{
			map[string]any{
				"@id":   "urn:uuid:policy-provider-country-0",
				"@type": "odrl:Duty",
				"odrl:constraint": map[string]any{
					"@type":             "odrl:Constraint",
					"odrl:leftOperand":  map[string]any{"@id": "urn:uuid:field-provider-country"},
					"odrl:operator":     map[string]any{"@id": "odrl:isAnyOf"},
					"odrl:rightOperand": []any{"DEU", "AUT", "CHE"},
				},
			},
		},
	})
	require.NoError(t, err)
	return &data
}

func validSemanticContractData(t *testing.T) *datatype.JSON {
	t.Helper()
	conditions := []any{
		partyCondition("provider", "Provider"),
		partyCondition("customer", "Customer"),
		map[string]any{
			"conditionId":   "payment",
			"conditionName": "Payment",
			"schemaVersion": "v1",
			"parameters": []any{
				semanticParam("amount", "decimal", SchemaContractV1, "contract.payment.amount"),
				semanticParam("currency", "string", SchemaContractV1, "contract.payment.currency"),
				semanticParam("dueDate", "date", SchemaContractV1, "contract.payment.dueDate"),
			},
		},
		map[string]any{
			"conditionId":   "sla",
			"conditionName": "SLA Availability",
			"schemaVersion": "v1",
			"parameters": []any{
				semanticParam("availability", "decimal", SchemaServiceV1, "service.sla.availability"),
			},
		},
	}
	data, err := datatype.NewJSON(map[string]any{
		"documentOutline": []any{
			map[string]any{"blockId": "root", "isRoot": true, "children": []any{"clause-main"}},
		},
		"documentBlocks": []any{
			map[string]any{
				"blockId": "clause-main",
				"type":    "CLAUSE",
				"text": strings.Join([]string{
					"Provider {{provider.legalName}} from {{provider.country}}",
					"Customer {{customer.legalName}} from {{customer.country}}",
					"Payment {{payment.amount}} {{payment.currency}} due {{payment.dueDate}}",
					"Availability {{sla.availability}}",
				}, "\n"),
				"conditionIds": []any{"provider", "customer", "payment", "sla"},
			},
		},
		"semanticConditions": conditions,
		"semanticConditionValues": []any{
			semanticValue("clause-main", "provider", "legalName", "Musterfirma"),
			semanticValue("clause-main", "provider", "country", "POL"),
			semanticValue("clause-main", "customer", "legalName", "Example company"),
			semanticValue("clause-main", "customer", "country", "DEU"),
			semanticValue("clause-main", "payment", "amount", 10000.0),
			semanticValue("clause-main", "payment", "currency", "EUR"),
			semanticValue("clause-main", "payment", "dueDate", "2026-06-19"),
			semanticValue("clause-main", "sla", "availability", 99.9),
		},
		"customMetaData": []any{},
	})
	require.NoError(t, err)
	return &data
}

func partyCondition(id string, name string) map[string]any {
	return map[string]any{
		"conditionId":   id,
		"conditionName": name,
		"schemaVersion": "v1",
		"entityType":    "CompanyParty",
		"entityRole":    id,
		"parameters": []any{
			semanticParam("legalName", "string", SchemaPartyV1, "company.legalName"),
			semanticParam("country", "string", SchemaPartyV1, "company.location.country"),
		},
	}
}

func semanticParam(name string, paramType string, schemaRef string, semanticPath string) map[string]any {
	return map[string]any{
		"parameterName": name,
		"type":          paramType,
		"schemaRef":     schemaRef,
		"semanticPath":  semanticPath,
		"isRequired":    true,
		"operators":     []any{},
	}
}

func semanticValue(blockID string, conditionID string, parameterName string, value any) map[string]any {
	return map[string]any{
		"blockId":        blockID,
		"conditionId":    conditionID,
		"parameterName":  parameterName,
		"parameterValue": value,
	}
}

func TestNormalizeTemplateDataRejectsLegacyStructure(t *testing.T) {
	_, err := NormalizeTemplateData(validTemplateData(t))
	require.ErrorContains(t, err, "canonical dcs:documentStructure envelope")
}

func TestNormalizeTemplateDataAcceptsCanonicalJSONLDEnvelope(t *testing.T) {
	normalized, err := NormalizeTemplateData(canonicalTemplateData(t))
	require.NoError(t, err)

	var result map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &result))
	require.Contains(t, result, "dcs:documentStructure")
	require.NotContains(t, result, "documentOutline")
	require.Equal(t, "http://www.w3.org/2001/XMLSchema#", result["@context"].(map[string]any)["xsd"])
}

func TestNormalizeContractDataAddsJSONLDContractType(t *testing.T) {
	normalized, err := NormalizeContractData(validTemplateData(t), false)
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &data))
	require.Equal(t, SchemaJSONLDContextV1, data["@context"])
	require.Equal(t, "Contract", data["@type"])
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
	require.Equal(t, "did:web:facis.example:template:1#field-provider-country", placeholder["dcs:bindsTo"].(map[string]any)["@id"])
	policy := data["dcs:policies"].([]any)[0].(map[string]any)
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
	policy := data["dcs:policies"].([]any)[0].(map[string]any)
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
	placeholder["dcs:bindsTo"] = map[string]any{"@id": "urn:uuid:field-missing"}
	invalid, err := datatype.NewJSON(data)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&invalid)
	require.ErrorContains(t, err, "placeholder binds to nonexistent contract data field")
}

func TestNormalizeTemplateDataRejectsMissingPolicyField(t *testing.T) {
	raw := canonicalTemplateData(t)
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	policy := data["dcs:policies"].([]any)[0].(map[string]any)
	constraint := policy["odrl:constraint"].(map[string]any)
	constraint["odrl:leftOperand"] = map[string]any{"@id": "urn:uuid:field-missing"}
	invalid, err := datatype.NewJSON(data)
	require.NoError(t, err)

	_, err = NormalizeTemplateData(&invalid)
	require.ErrorContains(t, err, "policy references nonexistent contract data field")
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

func TestNormalizeContractDataForPersistenceAddsDocumentIdentity(t *testing.T) {
	normalized, err := NormalizeContractDataForPersistence(validTemplateData(t), "did:web:facis.example:contract:1", false)
	require.NoError(t, err)

	var data map[string]any
	require.NoError(t, json.Unmarshal(*normalized, &data))
	require.Equal(t, "did:web:facis.example:contract:1", data["@id"])
	require.NotContains(t, data, "did")
}

func TestValidateContractSemanticsAcceptsCanonicalContract(t *testing.T) {
	raw := canonicalTemplateData(t)
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	data["@type"] = "dcs:Contract"
	data["semanticConditionValues"] = []any{}
	contract, err := datatype.NewJSON(data)
	require.NoError(t, err)

	require.NoError(t, ValidateContractSemantics(&contract))
}

func TestNormalizeContractDataAcceptsTypedSemanticValues(t *testing.T) {
	templateData := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*templateData, &decoded))
	decoded["semanticConditionValues"] = []any{
		map[string]any{
			"blockId":        "clause-1",
			"conditionId":    "cond-1",
			"parameterName":  "percent",
			"parameterValue": 99.9,
		},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, false)
	require.NoError(t, err)
}

func TestValidateContractSemanticsRejectsPlaceholderWithoutBinding(t *testing.T) {
	raw := validSemanticContractData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*raw, &decoded))
	decoded["placeholderBindings"] = []any{}
	contractData, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	err = ValidateContractSemantics(&contractData)
	require.ErrorContains(t, err, "has no binding")
}

func TestValidateContractSemanticsRejectsBindingToUnknownParameter(t *testing.T) {
	raw := validSemanticContractData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*raw, &decoded))
	decoded["placeholderBindings"] = []any{
		map[string]any{
			"@type":            "PlaceholderBinding",
			"source":           "clause-placeholder",
			"blockId":          "clause-main",
			"placeholder":      "{{provider.legalName}}",
			"boundToCondition": "provider",
			"boundToParameter": "missing",
		},
	}
	contractData, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	err = ValidateContractSemantics(&contractData)
	require.ErrorContains(t, err, "unknown parameter")
}

func TestNormalizeContractDataDoesNotInferRoleFromCustomerEntityType(t *testing.T) {
	data := validSemanticContractData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))

	conditions := decoded["semanticConditions"].([]any)
	for _, rawCondition := range conditions {
		condition := rawCondition.(map[string]any)
		if condition["conditionId"] != "customer" {
			continue
		}
		condition["entityType"] = "Customer"
		delete(condition, "entityRole")
		params := condition["parameters"].([]any)
		condition["parameters"] = []any{params[0], params[1]}
	}
	values := decoded["semanticConditionValues"].([]any)
	filtered := []any{}
	for _, rawValue := range values {
		value := rawValue.(map[string]any)
		if value["conditionId"] == "customer" && value["parameterName"] == "role" {
			continue
		}
		filtered = append(filtered, rawValue)
	}
	decoded["semanticConditionValues"] = filtered

	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, true)
	require.ErrorContains(t, err, "unsupported entityType")
}

func TestNormalizeContractDataRejectsNonCanonicalSemanticPathAliases(t *testing.T) {
	data := validSemanticContractData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))

	blocks := decoded["documentBlocks"].([]any)
	clause := blocks[0].(map[string]any)
	clause["text"] = strings.ReplaceAll(clause["text"].(string), "{{customer.legalName}}", "{{customer.company_legalName}}")
	clause["text"] = strings.ReplaceAll(clause["text"].(string), "{{customer.country}}", "{{customer.company_location_country}}")

	conditions := decoded["semanticConditions"].([]any)
	for _, rawCondition := range conditions {
		condition := rawCondition.(map[string]any)
		if condition["conditionId"] != "customer" {
			continue
		}
		condition["parameters"] = []any{
			semanticParam("company_legalName", "string", SchemaPartyV1, "company_legalName"),
			semanticParam("company_location_country", "string", SchemaPartyV1, "company_location_country"),
		}
	}

	values := decoded["semanticConditionValues"].([]any)
	for _, rawValue := range values {
		value := rawValue.(map[string]any)
		if value["conditionId"] != "customer" {
			continue
		}
		switch value["parameterName"] {
		case "legalName":
			value["parameterName"] = "company_legalName"
		case "country":
			value["parameterName"] = "company_location_country"
		}
	}

	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, true)
	require.ErrorContains(t, err, `unknown domain semanticPath "company_legalName"`)
}

func TestNormalizeContractDataAcceptsCompanyParties(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	decoded["parties"] = []any{
		map[string]any{"@type": "CompanyParty", "role": "supplier", "legalName": "Example Supplier GmbH"},
		map[string]any{"@type": "dcs:CompanyParty", "role": "customer", "legalName": "Example Customer AG"},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, false)
	require.NoError(t, err)
}

func TestNormalizeContractDataRejectsPartyRoleOutsideVocabulary(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	decoded["parties"] = []any{
		map[string]any{"@type": "CompanyParty", "role": "reseller", "legalName": "Example Reseller GmbH"},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, false)
	require.ErrorContains(t, err, "parties.0.role")
}

func TestNormalizeContractDataRejectsNonCompanyPartyType(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	decoded["parties"] = []any{
		map[string]any{"@type": "Company", "role": "supplier", "legalName": "Example Supplier GmbH"},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, false)
	require.ErrorContains(t, err, "parties.0.@type")
}

func TestNormalizeContractDataAcceptsSemanticValueInsideConstraint(t *testing.T) {
	data := validTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	conditions := decoded["semanticConditions"].([]any)
	condition := conditions[0].(map[string]any)
	params := condition["parameters"].([]any)
	params[0] = map[string]any{
		"parameterName": "country",
		"type":          "string",
		"schemaRef":     SchemaPartyV1,
		"semanticPath":  "company.location.country",
		"isRequired":    true,
		"operators":     []any{},
	}
	decoded["documentBlocks"].([]any)[0].(map[string]any)["text"] = "Country {{cond-1.country}}"
	decoded["semanticConditionValues"] = []any{
		map[string]any{
			"blockId":        "clause-1",
			"conditionId":    "cond-1",
			"parameterName":  "country",
			"parameterValue": "DEU",
		},
	}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	_, err = NormalizeContractData(&raw, true)
	require.NoError(t, err)
}

func TestValueConstraintResolvesAllowedValuesRef(t *testing.T) {
	err := valueMatchesConstraint("DEU", &valueConstraint{AllowedValuesRef: "ISO 3166-1 alpha-3"})
	require.NoError(t, err)

	err = valueMatchesConstraint("ZZZ", &valueConstraint{AllowedValuesRef: "ISO 3166-1 alpha-3"})
	require.ErrorContains(t, err, "expected one of")
}

func TestValueConstraintValidatesFormatWithoutAllowedValues(t *testing.T) {
	err := valueMatchesConstraint("DEU", &valueConstraint{Format: "iso-3166-1-alpha-3"})
	require.NoError(t, err)

	err = valueMatchesConstraint("DE", &valueConstraint{Format: "iso-3166-1-alpha-3"})
	require.ErrorContains(t, err, "expected value matching format")
}
