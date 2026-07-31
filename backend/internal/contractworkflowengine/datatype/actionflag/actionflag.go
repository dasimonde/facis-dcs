// Package actionflag is the "forward_to" decision on Submit while a contract
// is in SUBMITTED (APPROVAL routes to REVIEWED, REJECT routes back to
// NEGOTIATION). Values are case-insensitive on parse but only APPROVAL and
// REJECT are accepted — not, e.g., "approved" or "rejected".
package actionflag

import (
	"fmt"
	"strings"
)

type ActionFlag string

const (
	Approval ActionFlag = "APPROVAL"
	Reject   ActionFlag = "REJECT"
)

var validFlag = map[ActionFlag]bool{
	Approval: true,
	Reject:   true,
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
