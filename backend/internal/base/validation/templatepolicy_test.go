package validation

import (
	"encoding/json"
	"testing"

	"digital-contracting-service/internal/base/datatype"

	"github.com/stretchr/testify/require"
)

func TestAuditTemplatePoliciesReturnsWarningsForMissingFinishedTemplateFields(t *testing.T) {
	findings, err := AuditTemplatePolicies(canonicalTemplateData(t), TemplatePolicyAuditMetadata{
		DID:          "did:example:template",
		TemplateType: "CONTRACT_TEMPLATE",
		State:        "APPROVED",
	})
	require.NoError(t, err)

	require.NotEmpty(t, findings)
	require.Contains(t, policyFindingRuleIDs(findings), "FACIS-TPL-LEGAL-001")
	require.NotContains(t, policyFindingRuleIDs(findings), "FACIS-TPL-PARTY-001")
}

func TestAuditTemplatePoliciesAcceptsCanonicalTemplateWithoutErrors(t *testing.T) {
	findings, err := AuditTemplatePolicies(canonicalTemplateData(t), TemplatePolicyAuditMetadata{
		DID:          "did:example:template",
		TemplateType: "CONTRACT_TEMPLATE",
		State:        "APPROVED",
	})
	require.NoError(t, err)

	for _, finding := range findings {
		require.NotEqual(t, "error", finding.Severity, finding.Message)
	}
}

// A clause is bound to contract data by a field placeholder in its prose or by
// the ODRL rule its prose backs (dcs:prose).
func TestAuditTemplatePoliciesFlagsCanonicalClauseWithoutContractDataBinding(t *testing.T) {
	audit := func(ruleOperand string) []PolicyFinding {
		t.Helper()
		data := canonicalTemplateData(t)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(*data, &decoded))
		structure := decoded["dcs:documentStructure"].(map[string]any)
		clause := structure["dcs:blocks"].(map[string]any)["@list"].([]any)[0].(map[string]any)
		clause["dcs:content"] = map[string]any{"@list": []any{"Provider country is agreed."}}
		constraint := firstPolicyDuty(decoded)["odrl:constraint"].(map[string]any)
		constraint["odrl:leftOperand"] = map[string]any{"@id": ruleOperand}
		raw, err := datatype.NewJSON(decoded)
		require.NoError(t, err)
		findings, err := AuditTemplatePolicies(&raw, TemplatePolicyAuditMetadata{
			DID:          "did:example:template",
			TemplateType: "CONTRACT_TEMPLATE",
			State:        "APPROVED",
		})
		require.NoError(t, err)
		return findings
	}

	require.True(t, hasFindingSeverity(audit("urn:uuid:missing-field"), "FACIS-TPL-CLAUSE-001", "error"))
	require.NotContains(t, policyFindingRuleIDs(audit("urn:uuid:field-provider-country")), "FACIS-TPL-CLAUSE-001")
}

// ADR-23: a field without dcs:shape that a data object references is
// identified by the referencing property.
func TestAuditTemplatePoliciesIdentifiesDataObjectLeafByItsProperty(t *testing.T) {
	audit := func(path string) []PolicyFinding {
		t.Helper()
		data := canonicalTemplateData(t)
		var decoded map[string]any
		require.NoError(t, json.Unmarshal(*data, &decoded))
		decoded["dcs:contractFields"] = append(decoded["dcs:contractFields"].([]any), map[string]any{
			"@id": "urn:uuid:field-leaf", "@type": "dcs:ContractField", "dcs:label": "Leaf",
			"dcs:datatype": "xsd:string", "dcs:required": true,
		})
		decoded["dcs:contractData"] = []any{map[string]any{
			"@id": "urn:uuid:object-1", "@type": "https://example.org/legal#Contract",
			path: map[string]any{"@id": "urn:uuid:field-leaf"},
		}}
		raw, err := datatype.NewJSON(decoded)
		require.NoError(t, err)
		findings, err := AuditTemplatePolicies(&raw, TemplatePolicyAuditMetadata{
			DID:          "did:example:template",
			TemplateType: "CONTRACT_TEMPLATE",
			State:        "APPROVED",
		})
		require.NoError(t, err)
		return findings
	}

	require.NotContains(t, policyFindingRuleIDs(audit("https://w3id.org/facis/dcs/taxonomy/v1#field-contract-payment-amount")), "FACIS-TPL-DOMAIN-001")
	require.True(t, hasFindingSeverity(audit("https://example.org/legal#undeclared"), "FACIS-TPL-DOMAIN-001", "error"))
}

func TestAuditTemplatePoliciesFlagsPolicyOperandOutsideContractData(t *testing.T) {
	data := canonicalTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	policy := firstPolicyDuty(decoded)
	constraint := policy["odrl:constraint"].(map[string]any)
	constraint["odrl:leftOperand"] = map[string]any{"@id": "urn:uuid:missing-field"}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	findings, err := AuditTemplatePolicies(&raw, TemplatePolicyAuditMetadata{
		DID:          "did:example:template",
		TemplateType: "CONTRACT_TEMPLATE",
		State:        "APPROVED",
	})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "FACIS-TPL-POLICY-001", "error"))
}

func TestAuditTemplatePoliciesAcceptsRequiredDomainFields(t *testing.T) {
	data := canonicalTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	decoded["dcs:contractFields"] = append(decoded["dcs:contractFields"].([]any),
		canonicalContractField("jurisdiction"),
		canonicalContractField("signature-level"),
	)
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	findings, err := AuditTemplatePolicies(&raw, TemplatePolicyAuditMetadata{
		DID:          "did:example:template",
		TemplateType: "CONTRACT_TEMPLATE",
		State:        "APPROVED",
	})
	require.NoError(t, err)

	ruleIDs := policyFindingRuleIDs(findings)
	require.NotContains(t, ruleIDs, "FACIS-TPL-LEGAL-001")
	require.NotContains(t, ruleIDs, "FACIS-TPL-PARTY-001")
	require.NotContains(t, ruleIDs, "FACIS-TPL-SIGN-001")
}

func TestAuditTemplatePoliciesDoesNotApplyCompletenessRulesToComponents(t *testing.T) {
	data := canonicalTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	delete(decoded, "dcs:contractFields")
	delete(decoded, "dcs:policies")
	structure := decoded["dcs:documentStructure"].(map[string]any)
	delete(structure, "dcs:layout")
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	findings, err := AuditTemplatePolicies(&raw, TemplatePolicyAuditMetadata{
		DID:          "did:example:component",
		TemplateType: "COMPONENT",
		State:        "DRAFT",
	})
	require.NoError(t, err)

	ruleIDs := policyFindingRuleIDs(findings)
	require.NotContains(t, ruleIDs, "FACIS-TPL-STRUCT-001")
	require.NotContains(t, ruleIDs, "FACIS-TPL-DATA-001")
	require.NotContains(t, ruleIDs, "FACIS-TPL-POLICY-001")
	require.NotContains(t, ruleIDs, "FACIS-TPL-LIFECYCLE-001")
	require.NotContains(t, ruleIDs, "FACIS-TPL-CLAUSE-001")
	require.NotContains(t, ruleIDs, "FACIS-TPL-LEGAL-001")
	require.NotContains(t, ruleIDs, "FACIS-TPL-PARTY-001")
	require.NotContains(t, ruleIDs, "FACIS-TPL-SIGN-001")
	for _, finding := range findings {
		require.NotEqual(t, "error", finding.Severity, finding.Message)
	}
}

func TestAuditTemplatePoliciesFlagsComponentInternalPolicyReferences(t *testing.T) {
	data := canonicalTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	policy := firstPolicyDuty(decoded)
	constraint := policy["odrl:constraint"].(map[string]any)
	constraint["odrl:leftOperand"] = map[string]any{"@id": "urn:uuid:missing-field"}
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	findings, err := AuditTemplatePolicies(&raw, TemplatePolicyAuditMetadata{
		DID:          "did:example:component",
		TemplateType: "COMPONENT",
		State:        "DRAFT",
	})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "FACIS-COMP-POLICY-001", "error"))
	require.False(t, hasFindingSeverity(findings, "FACIS-TPL-POLICY-001", "error"))
}

func canonicalContractField(id string) map[string]any {
	ontologyIRIs := map[string]string{
		"jurisdiction":    "https://w3id.org/facis/dcs/taxonomy/v1#field-contract-jurisdiction",
		"signature-level": "https://w3id.org/facis/dcs/taxonomy/v1#field-signature-requiredLevel",
	}
	return map[string]any{
		"@id":          "urn:uuid:field-" + id,
		"@type":        "dcs:ContractField",
		"dcs:label":    id,
		"dcs:datatype": "xsd:string",
		"dcs:shape":    map[string]any{"@id": ontologyIRIs[id]},
		"dcs:required": true,
	}
}

func policyFindingRuleIDs(findings []PolicyFinding) []string {
	ruleIDs := make([]string, len(findings))
	for i, finding := range findings {
		ruleIDs[i] = finding.RuleID
	}
	return ruleIDs
}
