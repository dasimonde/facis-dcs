// Package componenttype enumerates the domains that can own an event/audit
// entry (used as the "component" tag on outbox events and audit-trail rows,
// and to route outbox-processor CID-chaining per resource, see
// base/event.OutboxProcessor).
package componenttype

import (
	"fmt"
	"strings"
)

type ComponentType string

const (
	ContractTemplateRepo         ComponentType = "CONTRACT_TEMPLATE_REPOSITORY"
	ContractWorkflowEngine       ComponentType = "CONTRACT_WORKFLOW_ENGINE"
	ContractStorageArchive       ComponentType = "CONTRACT_STORAGE_ARCHIVE"
	ProcessAuditAndCompliance    ComponentType = "PROCESS_AUDIT_AND_COMPLIANCE"
	SignatureManagement          ComponentType = "SIGNATURE_MANAGEMENT"
	TemplateCatalogueIntegration ComponentType = "TEMPLATE_CATALOGUE_INTEGRATION"
	System                       ComponentType = "SYSTEM"
)

var validType = map[ComponentType]bool{
	ContractTemplateRepo:         true,
	ContractWorkflowEngine:       true,
	ContractStorageArchive:       true,
	ProcessAuditAndCompliance:    true,
	SignatureManagement:          true,
	TemplateCatalogueIntegration: true,
	System:                       true,
}

func NewComponentType(s string) (ComponentType, error) {
	flag := ComponentType(strings.ToUpper(s))
	if !flag.IsValid() {
		return "", fmt.Errorf("invalid component type: %s", s)
	}
	return flag, nil
}

// IsValid reports whether the value is one of the declared ComponentType values.
func (f ComponentType) IsValid() bool {
	upper := ComponentType(strings.ToUpper(string(f)))
	return validType[upper]
}

// String returns the string representation of the ComponentType
func (f ComponentType) String() string {
	return string(f)
}
