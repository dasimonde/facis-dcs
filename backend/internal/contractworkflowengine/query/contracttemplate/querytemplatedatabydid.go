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
