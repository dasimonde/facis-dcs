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

func TestAuditTemplatePoliciesFlagsCanonicalClauseWithoutContractDataBinding(t *testing.T) {
	data := canonicalTemplateData(t)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(*data, &decoded))
	structure := decoded["dcs:documentStructure"].(map[string]any)
	blocks := structure["dcs:blocks"].(map[string]any)["@list"].([]any)
	clause := blocks[0].(map[string]any)
	content := clause["dcs:content"].(map[string]any)["@list"].([]any)
	field := content[1].(map[string]any)
	field["@id"] = "urn:uuid:missing-field"
	raw, err := datatype.NewJSON(decoded)
	require.NoError(t, err)

	findings, err := AuditTemplatePolicies(&raw, TemplatePolicyAuditMetadata{
		DID:          "did:example:template",
		TemplateType: "CONTRACT_TEMPLATE",
		State:        "APPROVED",
	})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "FACIS-TPL-CLAUSE-001", "error"))
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
