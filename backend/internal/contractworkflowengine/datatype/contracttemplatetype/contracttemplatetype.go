// Package contracttemplatetype distinguishes frame vs. sub-contract
// templates for the contract workflow engine's own template references.
package contracttemplatetype

import (
	"fmt"
	"strings"
)

type ContractTemplateType string

const (
	ContractTemplate ContractTemplateType = "CONTRACT_TEMPLATE"
	Component        ContractTemplateType = "COMPONENT"
)

var validFlag = map[ContractTemplateType]bool{
	ContractTemplate: true,
	Component:        true,
}

func NewContractTemplateType(s string) (ContractTemplateType, error) {
	flag := ContractTemplateType(strings.ToUpper(s))
	if !flag.IsValid() {
		return "", fmt.Errorf("invalid template type: %s", s)
	}
	return flag, nil
}

// IsValid reports whether the value is one of the declared ContractTemplateType values.
func (f ContractTemplateType) IsValid() bool {
	upper := ContractTemplateType(strings.ToUpper(string(f)))
	return validFlag[upper]
}

// String returns the string representation of the ContractTemplateType
func (f ContractTemplateType) String() string {
	return string(f)
}
