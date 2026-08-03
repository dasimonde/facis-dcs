// Package contracttemplatestate mirrors templaterepository's own
// contracttemplatestate enum, but scoped to how the contract workflow engine
// reads/references approved templates when creating a contract — see
// contractworkflowengine/query/contracttemplate.
package contracttemplatestate

import (
	"database/sql/driver"
	"fmt"
	"strings"
)

// ContractTemplateState represents the lifecycle state of a contract template
type ContractTemplateState string

const (
	Draft      ContractTemplateState = "DRAFT"
	Submitted  ContractTemplateState = "SUBMITTED"
	Rejected   ContractTemplateState = "REJECTED"
	Reviewed   ContractTemplateState = "REVIEWED"
	Approved   ContractTemplateState = "APPROVED"
	Registered ContractTemplateState = "REGISTERED"
	Published  ContractTemplateState = "PUBLISHED"
	Deleted    ContractTemplateState = "DELETED"
	Deprecated ContractTemplateState = "DEPRECATED"
)

var validState = map[ContractTemplateState]bool{
	Draft:      true,
	Submitted:  true,
	Rejected:   true,
	Reviewed:   true,
	Approved:   true,
	Registered: true,
	Published:  true,
	Deleted:    true,
	Deprecated: true,
}

func NewContractTemplateState(s string) (ContractTemplateState, error) {
	ts := ContractTemplateState(strings.ToUpper(s))
	if !ts.IsValid() {
		return "", fmt.Errorf("invalid template state: %s", s)
	}
	return ts, nil
}

// IsValid reports whether the value is one of the declared ContractTemplateState values.
func (s ContractTemplateState) IsValid() bool {
	upper := ContractTemplateState(strings.ToUpper(string(s)))
	return validState[upper]
}

// String returns the string representation of the ContractTemplateState
func (s ContractTemplateState) String() string {
	return string(s)
}

// Scan implements the sql.Scanner interface
func (s *ContractTemplateState) Scan(value interface{}) error {
	if value == nil {
		return fmt.Errorf("template state cannot be null")
	}

	var str string
	switch v := value.(type) {
	case string:
		str = v
	case []byte:
		str = string(v)
	default:
		return fmt.Errorf("unsupported type for ContractTemplateState: %T", value)
	}

	state, err := NewContractTemplateState(str)
	if err != nil {
		return err
	}

	*s = state
	return nil
}

// Value implements the driver.Valuer interface
func (s ContractTemplateState) Value() (driver.Value, error) {
	if !s.IsValid() {
		return nil, fmt.Errorf("invalid template state: %s", s)
	}
	return string(s), nil
}
