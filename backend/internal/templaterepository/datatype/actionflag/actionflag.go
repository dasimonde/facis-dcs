// Package actionflag is the "forward_to" decision on Submit while a template
// is in SUBMITTED (Approval routes to REVIEWED, Draft routes back to DRAFT
// for rework). Note this is a different value set than
// contractworkflowengine/datatype/actionflag's Approval/Reject.
package actionflag

import (
	"fmt"
	"strings"
)

type ActionFlag string

const (
	Approval ActionFlag = "APPROVAL"
	Draft    ActionFlag = "DRAFT"
)

var validFlag = map[ActionFlag]bool{
	Approval: true,
	Draft:    true,
}

func NewActionFlag(s string) (ActionFlag, error) {
	flag := ActionFlag(strings.ToUpper(s))
	if !flag.IsValid() {
		return "", fmt.Errorf("invalid action flag: %s", s)
	}
	return flag, nil
}

// IsValid reports whether the value is one of the declared ActionFlag values.
func (f ActionFlag) IsValid() bool {
	upper := ActionFlag(strings.ToUpper(string(f)))
	return validFlag[upper]
}

// String returns the string representation of the ActionFlag
func (f ActionFlag) String() string {
	return string(f)
}
