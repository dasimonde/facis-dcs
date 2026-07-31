// Package negotiationactionflag is the accept/reject decision flag on the
// "respond" endpoint (accept/reject one negotiation change request). Unlike
// most other contract-mutating endpoints, respond does not require
// updated_at and so is not covered by the optimistic-concurrency check
// (see contractworkflowengine/command package doc).
package negotiationactionflag

import (
	"fmt"
	"strings"
)

type NegotiationActionFlag string

const (
	Accepting NegotiationActionFlag = "ACCEPTING"
	Rejecting NegotiationActionFlag = "REJECTING"
)

var validFlag = map[NegotiationActionFlag]bool{
	Accepting: true,
	Rejecting: true,
}

func NewNegotiationActionFlag(s string) (NegotiationActionFlag, error) {
	flag := NegotiationActionFlag(strings.ToUpper(s))
	if !flag.IsValid() {
		return "", fmt.Errorf("invalid negotiation action flag: %s", s)
	}
	return flag, nil
}

// IsValid reports whether the value is one of the declared NegotiationActionFlag values.
func (f NegotiationActionFlag) IsValid() bool {
	upper := NegotiationActionFlag(strings.ToUpper(string(f)))
	return validFlag[upper]
}

// String returns the string representation of the NegotiationActionFlag
func (f NegotiationActionFlag) String() string {
	return string(f)
}
