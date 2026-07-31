package processauditandcompliance

import (
	"time"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/templaterepository/db"

	"digital-contracting-service/internal/base/validation"
)

func ContractContentPolicyFindingEventData(finding validation.PolicyFinding, metadata validation.ContractContentAuditMetadata) map[string]any {
	data := map[string]any{
		"policySetId":     finding.PolicySetID,
		"policyVersion":   finding.PolicyVersion,
		"ruleId":          finding.RuleID,
		"title":           finding.Title,
		"severity":        finding.Severity,
		"message":         finding.Message,
		"path":            finding.Path,
		"fieldIri":        finding.FieldIri,
		"ontologyTerm":    finding.OntologyTerm,
		"objectType":      "contract",
		"objectDid":       metadata.ContractDID,
		"contractVersion": metadata.ContractVersion,
		"auditedBy":       metadata.AuditedBy,
	}
	addPolicyFindingDetails(data, finding)
	return data
}

func TemplatePolicyFindingEventData(finding validation.PolicyFinding, template *db.ContractTemplate) map[string]any {
	data := map[string]any{
		"policySetId":   finding.PolicySetID,
		"policyVersion": finding.PolicyVersion,
		"ruleId":        finding.RuleID,
		"title":         finding.Title,
		"severity":      finding.Severity,
		"message":       finding.Message,
		"path":          finding.Path,
		"fieldIri":      finding.FieldIri,
		"ontologyTerm":  finding.OntologyTerm,
	}
	addPolicyFindingDetails(data, finding)
	if template == nil {
		return data
	}

	data["objectType"] = "contractTemplate"
	data["objectDid"] = template.DID
	data["objectName"] = template.Name
	data["objectDescription"] = base.DerefString(template.Description)
	data["templateType"] = template.TemplateType
	data["state"] = template.State
	data["version"] = template.Version
	data["createdBy"] = template.CreatedBy
	data["createdAt"] = template.CreatedAt.UTC().Format(time.RFC3339)
	data["updatedAt"] = template.UpdatedAt.UTC().Format(time.RFC3339)
	return data
}

func TemplateApprovalProvenanceFindingEventData(finding validation.PolicyFinding, did string) map[string]any {
	data := map[string]any{
		"policySetId":   finding.PolicySetID,
		"policyVersion": finding.PolicyVersion,
		"ruleId":        finding.RuleID,
		"title":         finding.Title,
		"severity":      finding.Severity,
		"message":       finding.Message,
		"path":          finding.Path,
		"fieldIri":      finding.FieldIri,
		"ontologyTerm":  finding.OntologyTerm,
		"objectType":    "contractTemplate",
		"objectDid":     did,
	}
	addPolicyFindingDetails(data, finding)
	return data
}

func addPolicyFindingDetails(data map[string]any, finding validation.PolicyFinding) {
	if finding.Requirement != "" {
		data["requirement"] = finding.Requirement
	}
	if finding.ActualValue != nil {
		data["actualValue"] = finding.ActualValue
	}
	if finding.ExpectedValue != nil {
		data["expectedValue"] = finding.ExpectedValue
	}
	if len(finding.ExpectedValues) > 0 {
		data["expectedValues"] = finding.ExpectedValues
	}
	if finding.Operator != "" {
		data["operator"] = finding.Operator
	}
}
