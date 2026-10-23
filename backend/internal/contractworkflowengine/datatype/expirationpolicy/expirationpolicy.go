// Package expirationpolicy declares which workflow follows when a contract's
// exp_date passes. The expiry cron job sets EXPIRED and emits the policy with
// ContractExpired; subscribers and the contract manager start the workflow.
package expirationpolicy

import (
	"fmt"
	"strings"
)

type ExpirationPolicy string

const (
	Renewal     ExpirationPolicy = "RENEWAL"
	Termination ExpirationPolicy = "TERMINATION"
	Archiving   ExpirationPolicy = "ARCHIVING"
)

var validPolicies = map[ExpirationPolicy]bool{
	Renewal:     true,
	Termination: true,
	Archiving:   true,
}

func NewExpirationPolicy(s string) (ExpirationPolicy, error) {
	flag := ExpirationPolicy(strings.ToUpper(s))
	if !flag.IsValid() {
		return "", fmt.Errorf("invalid expiration policy: %s", s)
	}
	return flag, nil
}

// IsValid reports whether the value is one of the declared ExpirationPolicy values.
func (f ExpirationPolicy) IsValid() bool {
	upper := ExpirationPolicy(strings.ToUpper(string(f)))
	return validPolicies[upper]
}

// String returns the string representation of the ExpirationPolicy
func (f ExpirationPolicy) String() string {
	return string(f)
}
