// Package negotiationtaskstate tracks, per responsible peer, whether that
// negotiator has responded in the current negotiation round (OPEN ->
// ACCEPTED). Once every negotiator's task is no longer OPEN, Submit merges
// all accepted change requests (see negotiationmerging) and bumps
// contract_version.
package negotiationtaskstate

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

type NegotiationTaskState string

const (
	Open     NegotiationTaskState = "OPEN"
	Accepted NegotiationTaskState = "ACCEPTED"
)

var validStates = map[NegotiationTaskState]bool{
	Open:     true,
	Accepted: true,
}

func NewNegotiationTaskState(s string) (NegotiationTaskState, error) {
	ts := NegotiationTaskState(strings.ToUpper(s))
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid approval task state: %s", s)
	}
	return ts, nil
}

// IsValid reports whether the value is one of the declared NegotiationTaskState values.
func (s NegotiationTaskState) IsValid() bool {
	upper := NegotiationTaskState(strings.ToUpper(string(s)))
	return validStates[upper]
}

// String returns the string representation of the NegotiationTaskState
func (s NegotiationTaskState) String() string {
	return string(s)
}

// Scan implements the sql.Scanner interface
func (s *NegotiationTaskState) Scan(value interface{}) error {
	if value == nil {
		return fmt.Errorf("negotiation task state cannot be null")
	}

	var str string
	switch v := value.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		return fmt.Errorf("unsupported type for NegotiationTaskState: %T", value)
	}

	state, err := NewNegotiationTaskState(str)
	if err != nil {
		return err
	}

	*s = state
	return nil
}

// Value implements the driver.Valuer interface
func (s NegotiationTaskState) Value() (driver.Value, error) {
	if !s.IsValid() {
		return nil, fmt.Errorf("invalid negotiation task state: %s", s)
	}
	return string(s), nil
}
