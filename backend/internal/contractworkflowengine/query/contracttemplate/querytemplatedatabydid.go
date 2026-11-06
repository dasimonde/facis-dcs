package contracttemplate

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/validation"
)

// ConvertTemplateDataToContractData derives a contract document from a
// stored template document: @type becomes dcs:Contract, dcs:metadata's
// @type becomes dcs:ContractMetadata, derivedFromTemplate provenance is
// attached, and every party IRI the ODRL rules reference is materialized
// as a typed dcs:CompanyParty node.
func ConvertTemplateDataToContractData(raw *datatype.JSON, templateDID string, templateVersions ...int) (*datatype.JSON, error) {
	if raw == nil || !raw.IsNotNullValue() {
		return raw, nil
	}

	var templateDataMap map[string]interface{}
	if err := json.Unmarshal(*raw, &templateDataMap); err != nil {
		return nil, fmt.Errorf("unmarshal template data failed: %w", err)
	}

	if _, ok := templateDataMap["dcs:documentStructure"]; !ok {
		return nil, errors.New("template data must use the canonical dcs:documentStructure envelope")
	}

	templateDataMap["@type"] = "dcs:Contract"
	if metadata, ok := templateDataMap["dcs:metadata"].(map[string]interface{}); ok {
		metadata["@type"] = "dcs:ContractMetadata"
	}

	provenance := map[string]interface{}{"@id": base.ResourceIRI("template", templateDID)}
	if len(templateVersions) > 0 && templateVersions[0] > 0 {
		provenance["version"] = templateVersions[0]
	} else if metadata, ok := templateDataMap["dcs:metadata"].(map[string]interface{}); ok {
		if version, exists := metadata["dcs:templateVersion"]; exists {
			provenance["version"] = version
		}
	}
	templateDataMap["derivedFromTemplate"] = provenance
	materializeRuleParties(templateDataMap)
	populateFixedFieldValues(templateDataMap)

	contractData, err := datatype.NewJSON(templateDataMap)
	if err != nil {
		return nil, fmt.Errorf("marshal converted contract data failed: %w", err)
	}

	return validation.NormalizeContractData(&contractData, false)
}

// materializeRuleParties ensures every role-derived party IRI referenced as
// odrl:assigner/odrl:assignee resolves to a typed dcs:CompanyParty node in
// the document's dcs:parties.
func materializeRuleParties(doc map[string]interface{}) {
	policies, ok := doc["dcs:policies"].(map[string]interface{})
	if !ok {
		return
	}
	roles := map[string]string{}
	order := []string{}
	for _, bucket := range []string{"odrl:permission", "odrl:prohibition", "odrl:obligation"} {
		rules, ok := policies[bucket].([]interface{})
		if !ok {
			continue
		}
		for _, rawRule := range rules {
			rule, ok := rawRule.(map[string]interface{})
			if !ok {
				continue
			}
			for _, side := range []string{"odrl:assigner", "odrl:assignee"} {
				ref, ok := rule[side].(map[string]interface{})
				if !ok {
					continue
				}
				iri, _ := ref["@id"].(string)
				_, role, found := strings.Cut(iri, "#party-")
				if !found || role == "" {
					continue
				}
				role, err := url.PathUnescape(role)
				if err != nil || role == "" {
					continue
				}
				if _, seen := roles[iri]; !seen {
					roles[iri] = role
					order = append(order, iri)
				}
			}
		}
	}
	if len(order) == 0 {
		return
	}
	parties, _ := doc["dcs:parties"].([]interface{})
	existing := map[string]bool{}
	for _, rawParty := range parties {
		if party, ok := rawParty.(map[string]interface{}); ok {
			if iri, _ := party["@id"].(string); iri != "" {
				existing[iri] = true
			}
		}
	}
	for _, iri := range order {
		if existing[iri] {
			continue
		}
		parties = append(parties, map[string]interface{}{
			"@id":      iri,
			"@type":    "dcs:CompanyParty",
			"dcs:role": roles[iri],
		})
	}
	doc["dcs:parties"] = parties
}

// populateFixedFieldValues fills the contract terms that the template's SLA
// rules fix (DCS-FR-TR-12): an obligation constraint "field odrl:eq literal"
// gives that field its value. A field that already has a value keeps it.
// Constraints under odrl:or or odrl:xone fix nothing and are not read.
func populateFixedFieldValues(doc map[string]interface{}) {
	policies, ok := doc["dcs:policies"].(map[string]interface{})
	if !ok {
		return
	}
	fixed := map[string]interface{}{}
	obligations, _ := policies["odrl:obligation"].([]interface{})
	for _, rawRule := range obligations {
		rule, ok := rawRule.(map[string]interface{})
		if !ok {
			continue
		}
		constraints, isList := rule["odrl:constraint"].([]interface{})
		if !isList {
			constraints = []interface{}{rule["odrl:constraint"]}
		}
		for _, rawConstraint := range constraints {
			constraint, ok := rawConstraint.(map[string]interface{})
			if !ok {
				continue
			}
			operator, _ := constraint["odrl:operator"].(map[string]interface{})
			left, _ := constraint["odrl:leftOperand"].(map[string]interface{})
			field, _ := left["@id"].(string)
			if operator["@id"] != "odrl:eq" || field == "" {
				continue
			}
			right := constraint["odrl:rightOperand"]
			if node, isNode := right.(map[string]interface{}); right == nil || (isNode && node["@value"] == nil) {
				continue
			}
			fixed[field] = right
		}
	}
	fields, _ := doc["dcs:contractFields"].([]interface{})
	for _, rawField := range fields {
		field, ok := rawField.(map[string]interface{})
		if !ok {
			continue
		}
		id, _ := field["@id"].(string)
		value, isFixed := fixed[id]
		if _, hasValue := field["dcs:value"]; !isFixed || hasValue {
			continue
		}
		if _, isNode := value.(map[string]interface{}); !isNode {
			datatype, _ := field["dcs:datatype"].(string)
			if datatype == "" {
				datatype = "xsd:string"
			}
			value = map[string]interface{}{"@value": fmt.Sprint(value), "@type": datatype}
		}
		field["dcs:value"] = value
	}
}
