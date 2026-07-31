package eventtype

type EventType string

const (
	TemplatePolicyAuditFinding             EventType = "TEMPLATE_POLICY_AUDIT_FINDING"
	TemplateApprovalProvenanceAuditFinding EventType = "TEMPLATE_APPROVAL_PROVENANCE_AUDIT_FINDING"
	ContractContentPolicyAuditFinding      EventType = "CONTRACT_CONTENT_POLICY_AUDIT_FINDING"
)

func (f EventType) String() string {
	return string(f)
}
