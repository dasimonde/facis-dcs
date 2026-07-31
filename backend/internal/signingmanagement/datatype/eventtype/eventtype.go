package eventtype

import (
	"fmt"
	"strings"
)

type EventType string

const (
	Apply                EventType = "APPLY_SIGNATURE"
	Applied              EventType = "APPLIED_SIGNATURE"
	Validate             EventType = "VALIDATE_SIGNATURE"
	Verify               EventType = "VERIFY_SIGNATURE"
	RetrieveAll          EventType = "RETRIEVE_ALL_CONTRACTS"
	RetrieveByID         EventType = "RETRIEVE_CONTRACT_BY_ID"
	Revoke               EventType = "REVOKE_SIGNATURE"
	ComplianceValidation EventType = "COMPLIANCE_VALIDATION"
	Audit                EventType = "AUDIT_CONTRACT_TEMPLATE"
	SigningRequest       EventType = "SIGNING_REQUEST"
	Search               EventType = "SEARCH"
)

var validStates = map[EventType]bool{
	Apply:                true,
	Applied:              true,
	Validate:             true,
	Verify:               true,
	RetrieveAll:          true,
	RetrieveByID:         true,
	Revoke:               true,
	ComplianceValidation: true,
	Audit:                true,
	SigningRequest:       true,
	Search:               true,
}

func NewEventType(s string) (EventType, error) {
	ts := EventType(strings.ToUpper(s))
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid event type: %s", s)
	}
	return ts, nil
}

// IsValid reports whether the value is one of the declared EventType values.
func (s EventType) IsValid() bool {
	upper := EventType(strings.ToUpper(string(s)))
	return validStates[upper]
}

// String returns the string representation of the EventType
func (s EventType) String() string {
	return string(s)
}
