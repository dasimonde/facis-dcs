// Package contractstate defines the contract lifecycle enum and the single
// explicit transition table (see transition.go) that is the sole source of
// truth for which state × event combinations are valid. This is the ONE
// contract state machine used across the whole backend; every consumer
// imports this package.
//
// Expired is set out-of-band by the CWE cron job, though readers see it
// instantly via the contracts_effective DB view.
package contractstate

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

// ContractState represents the lifecycle state of a contract
type ContractState string

const (
	Draft       ContractState = "DRAFT"
	Offered     ContractState = "OFFERED"
	Rejected    ContractState = "REJECTED"
	Withdrawn   ContractState = "WITHDRAWN"
	Negotiation ContractState = "NEGOTIATION"
	Submitted   ContractState = "SUBMITTED"
	Reviewed    ContractState = "REVIEWED"
	Approved    ContractState = "APPROVED"
	Signed      ContractState = "SIGNED"
	Active      ContractState = "ACTIVE"
	Revoked     ContractState = "REVOKED"
	Terminated  ContractState = "TERMINATED"
	Expired     ContractState = "EXPIRED"
)

var validStates = map[ContractState]bool{
	Draft:       true,
	Offered:     true,
	Rejected:    true,
	Withdrawn:   true,
	Negotiation: true,
	Submitted:   true,
	Reviewed:    true,
	Approved:    true,
	Signed:      true,
	Active:      true,
	Revoked:     true,
	Terminated:  true,
	Expired:     true,
}

// All returns every contract state, in lifecycle order — so a consumer that
// classifies states (which of them freeze the PDF artifact, say) derives its
// answer from the enum instead of restating it and silently missing a state
// added later.
func All() []ContractState {
	return []ContractState{
		Draft, Offered, Rejected, Withdrawn, Negotiation, Submitted,
		Reviewed, Approved, Signed, Active, Revoked, Terminated, Expired,
	}
}

func NewContractState(s string) (ContractState, error) {
	ts := ContractState(strings.ToUpper(s))
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid contract state: %s", s)
	}
	return ts, nil
}

// IsValid reports whether the value is one of the declared ContractState values.
func (s ContractState) IsValid() bool {
	upper := ContractState(strings.ToUpper(string(s)))
	return validStates[upper]
}

// String returns the string representation of the ContractState
func (s ContractState) String() string {
	return string(s)
}

// Scan implements the sql.Scanner interface
func (s *ContractState) Scan(value interface{}) error {
	if value == nil {
		return fmt.Errorf("contract state cannot be null")
	}

	var str string
	switch v := value.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		return fmt.Errorf("unsupported type for ContractState: %T", value)
	}

	state, err := NewContractState(str)
	if err != nil {
		return err
	}

	*s = state
	return nil
}

// Value implements the driver.Valuer interface
func (s ContractState) Value() (driver.Value, error) {
	if !s.IsValid() {
		return nil, fmt.Errorf("invalid contract state: %s", s)
	}
	return string(s), nil
}
