// Package reviewtaskstate is the per-peer sub-state-machine for a
// contract's review tasks (OPEN -> APPROVED/REJECTED), gating the
// SUBMITTED -> REVIEWED transition the same way approvaltaskstate gates
// REVIEWED -> APPROVED.
package reviewtaskstate

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

type ReviewTaskState string

const (
	Open     ReviewTaskState = "OPEN"
	Rejected ReviewTaskState = "REJECTED"
	Approved ReviewTaskState = "APPROVED"
)

var validState = map[ReviewTaskState]bool{
	Open:     true,
	Rejected: true,
	Approved: true,
}

func NewReviewTaskState(s string) (ReviewTaskState, error) {
	ts := ReviewTaskState(strings.ToUpper(s))
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid review task state: %s", s)
	}
	return ts, nil
}

// IsValid reports whether the value is one of the declared ReviewTaskState values.
func (s ReviewTaskState) IsValid() bool {
	upper := ReviewTaskState(strings.ToUpper(string(s)))
	return validState[upper]
}

// String returns the string representation of the ReviewTaskState
func (s ReviewTaskState) String() string {
	return string(s)
}

// Scan implements the sql.Scanner interface
func (s *ReviewTaskState) Scan(value interface{}) error {
	if value == nil {
		return fmt.Errorf("review task state cannot be null")
	}

	var str string
	switch v := value.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		return fmt.Errorf("unsupported type for ReviewTaskState: %T", value)
	}

	state, err := NewReviewTaskState(str)
	if err != nil {
		return err
	}

	*s = state
	return nil
}

// Value implements the driver.Valuer interface
func (s ReviewTaskState) Value() (driver.Value, error) {
	if !s.IsValid() {
		return nil, fmt.Errorf("invalid review task state: %s", s)
	}
	return string(s), nil
}
