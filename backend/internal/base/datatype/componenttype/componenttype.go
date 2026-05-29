package componenttype

import (
	"fmt"
	"strings"
)

type ComponentType string

const (
	ContractTemplateRepo      ComponentType = "CONTRACT_TEMPLATE_REPOSITORY"
	ContractWorkflowEngine    ComponentType = "CONTRACT_WORKFLOW_ENGINE"
	ProcessAuditAndCompliance ComponentType = "PROCESS_AUDIT_AND_COMPLIANCE"
	SignatureManagement       ComponentType = "SIGNATURE_MANAGEMENT"
)

var validType = map[ComponentType]bool{
	ContractTemplateRepo:      true,
	ContractWorkflowEngine:    true,
	ProcessAuditAndCompliance: true,
	SignatureManagement:       true,
}

func NewComponentType(s string) (ComponentType, error) {
	flag := ComponentType(strings.ToUpper(s))
	if !flag.IsValid() {
		return "", fmt.Errorf("invalid component type: %s", s)
	}
	return flag, nil
}

// IsValid checks if the ComponentType is a valid role
func (f ComponentType) IsValid() bool {
	upper := ComponentType(strings.ToUpper(string(f)))
	return validType[upper]
}

// String returns the string representation of the ComponentType
func (f ComponentType) String() string {
	return string(f)
}
