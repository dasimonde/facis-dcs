package eventtype

import (
	"fmt"
	"strings"
)

type EventType string

const (
	Create       EventType = "CREATE_CONTRACT_TEMPLATE"
	Submit       EventType = "SUBMIT_CONTRACT_TEMPLATE"
	Approve      EventType = "APPROVE_CONTRACT_TEMPLATE"
	Reject       EventType = "REJECT_CONTRACT_TEMPLATE"
	Verify       EventType = "VERIFY_CONTRACT_TEMPLATE"
	Update       EventType = "UPDATE_CONTRACT_TEMPLATE"
	RetrieveAll  EventType = "RETRIEVE_ALL_CONTRACT_TEMPLATES"
	RetrieveByID EventType = "RETRIEVE_CONTRACT_TEMPLATE_BY_ID"
	Search       EventType = "SEARCH_CONTRACT_TEMPLATE"
	Archive      EventType = "ARCHIVE_CONTRACT_TEMPLATE"
	Register     EventType = "REGISTER_CONTRACT_TEMPLATE"
	Audit        EventType = "AUDIT_CONTRACT_TEMPLATE"
)

var validStates = map[EventType]bool{
	Create:       true,
	Submit:       true,
	Approve:      true,
	Reject:       true,
	Verify:       true,
	Update:       true,
	RetrieveAll:  true,
	RetrieveByID: true,
	Search:       true,
	Archive:      true,
	Register:     true,
	Audit:        true,
}

func NewEventType(s string) (EventType, error) {
	ts := EventType(strings.ToUpper(s))
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid event type: %s", s)
	}
	return ts, nil
}

// IsValid checks if the EventType is a valid role
func (s EventType) IsValid() bool {
	upper := EventType(strings.ToUpper(string(s)))
	return validStates[upper]
}

// String returns the string representation of the EventType
func (s EventType) String() string {
	return string(s)
}
