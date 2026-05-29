package datatype

import (
	"fmt"
	"strings"
)

type ResourceType string

const (
	TemplateResourceType ResourceType = "template"
	ContractResourceType ResourceType = "contract"
)

var validType = map[ResourceType]bool{
	TemplateResourceType: true,
	ContractResourceType: true,
}

func NewResourceType(s string) (ResourceType, error) {
	flag := ResourceType(strings.ToUpper(s))
	if !flag.IsValid() {
		return "", fmt.Errorf("invalid component type: %s", s)
	}
	return flag, nil
}

// IsValid checks if the ResourceType is a valid role
func (f ResourceType) IsValid() bool {
	upper := ResourceType(strings.ToUpper(string(f)))
	return validType[upper]
}

// String returns the string representation of the ResourceType
func (f ResourceType) String() string {
	return string(f)
}
