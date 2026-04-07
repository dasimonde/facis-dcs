package approvaltaskstate

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

type ApprovalTaskState string

const (
	Open     ApprovalTaskState = "OPEN"
	Rejected ApprovalTaskState = "REJECTED"
	Approved ApprovalTaskState = "APPROVED"
)

var validStates = map[ApprovalTaskState]bool{
	Open:     true,
	Rejected: true,
	Approved: true,
}

func NewApprovalTaskState(s string) (ApprovalTaskState, error) {
	ts := ApprovalTaskState(strings.ToUpper(s))
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid approval task state: %s", s)
	}
	return ts, nil
}

// IsValid checks if the ApprovalTaskState is a valid role
func (s ApprovalTaskState) IsValid() bool {
	upper := ApprovalTaskState(strings.ToUpper(string(s)))
	return validStates[upper]
}

// String returns the string representation of the ApprovalTaskState
func (s ApprovalTaskState) String() string {
	return string(s)
}

// Scan implements the sql.Scanner interface
func (s *ApprovalTaskState) Scan(value interface{}) error {
	if value == nil {
		return fmt.Errorf("approval task state cannot be null")
	}

	var str string
	switch v := value.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		return fmt.Errorf("unsupported type for ApprovalTaskState: %T", value)
	}

	state, err := NewApprovalTaskState(str)
	if err != nil {
		return err
	}

	*s = state
	return nil
}

// Value implements the driver.Valuer interface
func (s ApprovalTaskState) Value() (driver.Value, error) {
	if !s.IsValid() {
		return nil, fmt.Errorf("invalid approval task state: %s", s)
	}
	return string(s), nil
}
