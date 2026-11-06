package eventtype

import (
	"fmt"
	"strings"
)

type EventType string

const (
	Apply                EventType = "APPLY_SIGNATURE"
	Applied              EventType = "APPLIED_SIGNATURE"
	ApplyFailed          EventType = "APPLY_SIGNATURE_FAILED"
	Validate             EventType = "VALIDATE_SIGNATURE"
	ValidateFailed       EventType = "VALIDATE_SIGNATURE_FAILED"
	Verify               EventType = "VERIFY_SIGNATURE"
	RetrieveAll          EventType = "RETRIEVE_ALL_CONTRACTS"
	RetrieveByID         EventType = "RETRIEVE_CONTRACT_BY_ID"
	RetrieveForSigning   EventType = "RETRIEVE_CONTRACT_FOR_SIGNING"
	Revoke               EventType = "REVOKE_SIGNATURE"
	RevokeFailed         EventType = "REVOKE_SIGNATURE_FAILED"
	ComplianceValidation EventType = "COMPLIANCE_VALIDATION"
	Audit                EventType = "AUDIT_CONTRACT_TEMPLATE"
	SigningRequest       EventType = "SIGNING_REQUEST"
	Search               EventType = "SEARCH"
)

var validStates = map[EventType]bool{
	Apply:                true,
	Applied:              true,
	ApplyFailed:          true,
	Validate:             true,
	ValidateFailed:       true,
	Verify:               true,
	RetrieveAll:          true,
	RetrieveByID:         true,
	RetrieveForSigning:   true,
	Revoke:               true,
	RevokeFailed:         true,
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
