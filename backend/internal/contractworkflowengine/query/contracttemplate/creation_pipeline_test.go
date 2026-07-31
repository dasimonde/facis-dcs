package contracttemplate

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"
	contractdb "digital-contracting-service/internal/contractworkflowengine/db"
	semanticmapper "digital-contracting-service/internal/semantic/mapper"

	"github.com/stretchr/testify/require"
)

const (
	creationTemplateDID = "did:web:example:template:dach-service"
	creationContractDID = "did:web:example:contract:dach-service"
)

// TestCreateTemplateThenNormalizeContract verifies the full pipeline from
// canonical template through contract normalization. The semantic mapper is a
// pass-through: it returns the stored JSON-LD unchanged.
func TestCreateTemplateThenNormalizeContract(t *testing.T) {
	templateData := newCreationPipelineJSON(t, creationPipelineTemplate())
	persistedTemplate, err := validation.NormalizeTemplateDataForPersistence(templateData, creationTemplateDID)
	require.NoError(t, err)

	contractDraft, err := ConvertTemplateDataToContractData(persistedTemplate, creationTemplateDID)
	require.NoError(t, err)

	var contractData map[string]any
	require.NoError(t, json.Unmarshal(*contractDraft, &contractData))
	contractData["semanticConditionValues"] = creationPipelineValues()
	persistedContract, err := validation.NormalizeContractDataForPersistence(
		newCreationPipelineJSON(t, contractData),
		creationContractDID,
		true,
	)
	require.NoError(t, err)

	assertCreationPipelineLayout(t, persistedContract)
	assertCreationPipelinePolicies(t, persistedContract)

	now := time.Date(2026, time.June, 19, 12, 0, 0, 0, time.UTC)
	contract := contractdb.Contract{
		DID:             creationContractDID,
		ContractVersion: 1,
		State:           "APPROVED",
		CreatedBy:       "test-participant",
		CreatedAt:       now,
		UpdatedAt:       now,
		ContractData:    persistedContract,
	}

	published, err := semanticmapper.BuildContractJSONLD(contract)
	require.NoError(t, err)

	var stored, returned map[string]any
	require.NoError(t, json.Unmarshal(*persistedContract, &stored))
	raw, err := json.Marshal(published)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(raw, &returned))
	require.Equal(t, stored, returned)
}

func creationPipelineTemplate() map[string]any {
	return map[string]any{
		"@context": map[string]any{
			"dcs":  "https://w3id.org/facis/dcs/ontology/v1#",
			"odrl": "http://www.w3.org/ns/odrl/2/",
			"xsd":  "http://www.w3.org/2001/XMLSchema#",
		},
		"@type": "dcs:ContractTemplate",
		"dcs:metadata": map[string]any{
			"@type":            "dcs:TemplateMetadata",
			"dcs:title":        "DACH Service Agreement",
			"dcs:templateType": "dcs:Component",
		},
		"dcs:documentStructure": map[string]any{
			"@type":      "dcs:DocumentStructure",
			"dcs:blocks": map[string]any{"@list": creationPipelineBlocks()},
			"dcs:layout": creationPipelineLayout(),
		},
		"dcs:contractFields": creationPipelineRequirements(),
		"dcs:policies":       creationPipelinePolicyDefinitions(),
	}
}

func creationPipelineBlocks() []any {
	return []any{
		creationPipelineSection("party", "Party"),
		creationPipelineSection("customer", "Customer"),
		creationPipelineClause("customer", []any{
			"Customer ",
			creationPipelinePlaceholder("{{customer.legalName}}", "customer", "legalName"),
			" from ",
			creationPipelinePlaceholder("{{customer.country}}", "customer", "country"),
		}),
		creationPipelineSection("provider", "Provider"),
		creationPipelineClause("provider", []any{
			"Provider ",
			creationPipelinePlaceholder("{{provider.legalName}}", "provider", "legalName"),
			" from ",
			creationPipelinePlaceholder("{{provider.country}}", "provider", "country"),
		}),
		creationPipelineSection("payment", "Payment"),
		creationPipelineClause("payment", []any{
			"Payment ",
			creationPipelinePlaceholder("{{payment.amount}}", "payment", "amount"),
			" ",
			creationPipelinePlaceholder("{{payment.currency}}", "payment", "currency"),
		}),
		creationPipelineSection("availability", "Availability"),
		creationPipelineClause("availability", []any{
			"Availability must be at least ",
			creationPipelinePlaceholder("{{availability.availability}}", "availability", "availability"),
			" percent.",
		}),
	}
}

func creationPipelineLayout() []any {
	return []any{
		creationPipelineLayoutNode("root", true, "party", "payment", "availability"),
		creationPipelineLayoutNode("party", false, "customer", "provider"),
		creationPipelineLayoutNode("customer", false, "clause-customer"),
		creationPipelineLayoutNode("clause-customer", false),
		creationPipelineLayoutNode("provider", false, "clause-provider"),
		creationPipelineLayoutNode("clause-provider", false),
		creationPipelineLayoutNode("payment", false, "clause-payment"),
		creationPipelineLayoutNode("clause-payment", false),
		creationPipelineLayoutNode("availability", false, "clause-availability"),
		creationPipelineLayoutNode("clause-availability", false),
	}
}

func creationPipelineRequirements() []any {
	placeholders := []any{}
	placeholders = append(placeholders, creationPipelineRequirement("customer", "Customer", "CompanyParty", "customer",
		creationPipelineField("customer", "legalName", "company.legalName"),
		creationPipelineField("customer", "country", "company.location.country"),
	)...)
	placeholders = append(placeholders, creationPipelineRequirement("provider", "Provider", "CompanyParty", "provider",
		creationPipelineField("provider", "legalName", "company.legalName"),
		creationPipelineField("provider", "country", "company.location.country"),
	)...)
	placeholders = append(placeholders, creationPipelineRequirement("payment", "Payment", "ContractDataObject", "",
		creationPipelineField("payment", "amount", "contract.payment.amount"),
		creationPipelineField("payment", "currency", "contract.payment.currency"),
	)...)
	placeholders = append(placeholders, creationPipelineRequirement("availability", "Availability", "ContractDataObject", "",
		creationPipelineField("availability", "availability", "service.sla.availability"),
	)...)
	return placeholders
}

func creationPipelinePolicyDefinitions() map[string]any {
	return map[string]any{
		"@id":          creationTemplateDID + "#policy-set-1",
		"@type":        "odrl:Offer",
		"odrl:profile": map[string]any{"@id": "https://w3id.org/facis/dcs/ontology/v1/odrl-profile"},
		"odrl:obligation": []any{
			creationPipelinePolicy(
				"provider-country-dach",
				"provider",
				"country",
				"odrl:isAnyOf",
				[]any{
					map[string]any{"@value": "DEU", "@type": "xsd:string"},
					map[string]any{"@value": "AUT", "@type": "xsd:string"},
					map[string]any{"@value": "CHE", "@type": "xsd:string"},
				},
			),
			creationPipelinePolicy(
				"payment-currency-eur",
				"payment",
				"currency",
				"odrl:eq",
				map[string]any{"@value": "EUR", "@type": "xsd:string"},
			),
			creationPipelinePolicy(
				"availability-minimum",
				"availability",
				"availability",
				"odrl:gteq",
				map[string]any{"@value": 99.9, "@type": "xsd:decimal"},
			),
		},
	}
}

func creationPipelineValues() []any {
	return []any{
		creationPipelineValue("clause-customer", "customer", "legalName", "Customer GmbH"),
		creationPipelineValue("clause-customer", "customer", "country", "DEU"),
		creationPipelineValue("clause-provider", "provider", "legalName", "Provider AG"),
		creationPipelineValue("clause-provider", "provider", "country", "AUT"),
		creationPipelineValue("clause-payment", "payment", "amount", 1250.0),
		creationPipelineValue("clause-payment", "payment", "currency", "EUR"),
		creationPipelineValue("clause-availability", "availability", "availability", 99.9),
	}
}

func assertCreationPipelineLayout(t *testing.T, raw *datatype.JSON) {
	t.Helper()
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	structure := data["dcs:documentStructure"].(map[string]any)
	layout := structure["dcs:layout"].([]any)

	require.Equal(t,
		[]string{"party", "payment", "availability"},
		creationPipelineChildNames(t, layout, "root"),
	)
	require.Equal(t,
		[]string{"customer", "provider"},
		creationPipelineChildNames(t, layout, "party"),
	)
	for _, group := range []string{"customer", "provider", "payment", "availability"} {
		require.Equal(t,
			[]string{"clause-" + group},
			creationPipelineChildNames(t, layout, group),
		)
	}
}

func assertCreationPipelinePolicies(t *testing.T, raw *datatype.JSON) {
	t.Helper()
	var data map[string]any
	require.NoError(t, json.Unmarshal(*raw, &data))
	policySet := data["dcs:policies"].(map[string]any)
	policies := policySet["odrl:obligation"].([]any)

	dach := creationPipelinePolicyBySuffix(t, policies, "policy-provider-country-dach")
	dachConstraint := dach["odrl:constraint"].(map[string]any)
	require.Equal(t, "odrl:isAnyOf", dachConstraint["odrl:operator"].(map[string]any)["@id"])
	require.Equal(t, []any{
		map[string]any{"@value": "DEU", "@type": "xsd:string"},
		map[string]any{"@value": "AUT", "@type": "xsd:string"},
		map[string]any{"@value": "CHE", "@type": "xsd:string"},
	}, dachConstraint["odrl:rightOperand"])

	currency := creationPipelinePolicyBySuffix(t, policies, "policy-payment-currency-eur")
	currencyConstraint := currency["odrl:constraint"].(map[string]any)
	require.Equal(t, "odrl:eq", currencyConstraint["odrl:operator"].(map[string]any)["@id"])
	require.Equal(t, map[string]any{"@value": "EUR", "@type": "xsd:string"}, currencyConstraint["odrl:rightOperand"])

	availability := creationPipelinePolicyBySuffix(t, policies, "policy-availability-minimum")
	availabilityConstraint := availability["odrl:constraint"].(map[string]any)
	require.Equal(t, "odrl:gteq", availabilityConstraint["odrl:operator"].(map[string]any)["@id"])
	require.Equal(t, map[string]any{"@value": 99.9, "@type": "xsd:decimal"}, availabilityConstraint["odrl:rightOperand"])
}

func creationPipelineSection(id string, title string) map[string]any {
	return map[string]any{
		"@id":       creationTemplateDID + "#block-" + id,
		"@type":     "dcs:Section",
		"dcs:title": title,
	}
}

func creationPipelineClause(group string, content []any) map[string]any {
	return map[string]any{
		"@id":         creationTemplateDID + "#block-clause-" + group,
		"@type":       "dcs:Clause",
		"dcs:content": map[string]any{"@list": content},
	}
}

func creationPipelinePlaceholder(_ string, conditionID string, parameterName string) map[string]any {
	return map[string]any{"@id": creationPipelineFieldID(conditionID, parameterName)}
}

func creationPipelineLayoutNode(id string, root bool, children ...string) map[string]any {
	childRefs := make([]any, 0, len(children))
	for _, child := range children {
		childRefs = append(childRefs, map[string]any{"@id": creationTemplateDID + "#block-" + child})
	}
	node := map[string]any{
		"@id":          creationTemplateDID + "#block-" + id,
		"dcs:children": map[string]any{"@list": childRefs},
	}
	if root {
		node["dcs:isRoot"] = true
	}
	return node
}

// creationPipelineRequirement flattens a condition's fields into the top-level
// placeholder list — the clean self-contained model has no requirement grouping.
func creationPipelineRequirement(
	_ string,
	_ string,
	_ string,
	_ string,
	fields ...map[string]any,
) []any {
	placeholders := make([]any, len(fields))
	for index, field := range fields {
		placeholders[index] = field
	}
	return placeholders
}

func creationPipelineField(conditionID string, parameterName string, domainFieldName string) map[string]any {
	datatype := "xsd:string"
	if strings.Contains(domainFieldName, "amount") || strings.Contains(domainFieldName, "availability") {
		datatype = "xsd:decimal"
	}
	return map[string]any{
		"@id":          creationPipelineFieldID(conditionID, parameterName),
		"@type":        "dcs:ContractField",
		"dcs:label":    parameterName,
		"dcs:datatype": datatype,
		"dcs:shape": map[string]any{
			"@id": "https://w3id.org/facis/dcs/taxonomy/v1#field-" + creationPipelineSlug(domainFieldName),
		},
		"dcs:required": true,
	}
}

func creationPipelineFieldID(conditionID string, parameterName string) string {
	return creationTemplateDID + "#field-" + conditionID + "-" + parameterName
}

func creationPipelinePolicy(
	id string,
	conditionID string,
	parameterName string,
	operator string,
	rightOperand any,
) map[string]any {
	return map[string]any{
		"@id":           creationTemplateDID + "#policy-" + id,
		"@type":         "odrl:Duty",
		"odrl:action":   map[string]any{"@id": "dcs:provideCompliantValue"},
		"odrl:assigner": map[string]any{"@id": creationTemplateDID + "#" + conditionID},
		"odrl:assignee": map[string]any{"@id": creationTemplateDID},
		"odrl:target":   map[string]any{"@id": creationTemplateDID},
		"dcs:prose":     map[string]any{"@id": "urn:uuid:block-clause-1"},
		"odrl:constraint": map[string]any{
			"@type":             "odrl:Constraint",
			"odrl:leftOperand":  map[string]any{"@id": creationPipelineFieldID(conditionID, parameterName)},
			"odrl:operator":     map[string]any{"@id": operator},
			"odrl:rightOperand": rightOperand,
		},
	}
}

func creationPipelineValue(blockID string, conditionID string, parameterName string, value any) map[string]any {
	return map[string]any{
		"blockId":        blockID,
		"conditionId":    conditionID,
		"parameterName":  parameterName,
		"parameterValue": value,
	}
}

func creationPipelineChildNames(t *testing.T, layout []any, parent string) []string {
	t.Helper()
	parentID := creationContractDID + "#block-" + parent
	for _, rawNode := range layout {
		node := rawNode.(map[string]any)
		if node["@id"] != parentID {
			continue
		}
		children := node["dcs:children"].(map[string]any)["@list"].([]any)
		result := make([]string, 0, len(children))
		for _, rawChild := range children {
			childID := rawChild.(map[string]any)["@id"].(string)
			result = append(result, childID[len(creationContractDID+"#block-"):])
		}
		return result
	}
	require.Failf(t, "layout node not found", "missing %s", parentID)
	return nil
}

func creationPipelinePolicyBySuffix(t *testing.T, policies []any, suffix string) map[string]any {
	t.Helper()
	expectedID := creationContractDID + "#" + suffix
	for _, rawPolicy := range policies {
		policy := rawPolicy.(map[string]any)
		if policy["@id"] == expectedID {
			return policy
		}
	}
	require.Failf(t, "policy not found", "missing %s", expectedID)
	return nil
}

func creationPipelineSlug(value string) string {
	result := make([]rune, 0, len(value))
	separator := false
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			if separator && len(result) > 0 {
				result = append(result, '-')
			}
			result = append(result, char)
			separator = false
			continue
		}
		separator = true
	}
	return string(result)
}

func newCreationPipelineJSON(t *testing.T, value any) *datatype.JSON {
	t.Helper()
	raw, err := datatype.NewJSON(value)
	require.NoError(t, err)
	return &raw
}
