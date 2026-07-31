// Package base holds the shared kernel used by every domain: cross-cutting
// helpers (this file), plus the conf, datatype, db, event, identity, ipfs,
// tsa, and validation subpackages. Domains depend on base; base never
// depends on a domain.
package base

import "strings"

// IsAuditVisibleEventType returns whether an event should be shown in audit results.
// Read-only lookup events are useful operational traces, but they are not findings.
func IsAuditVisibleEventType(eventType string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(eventType))
	if normalized == "" {
		return true
	}
	return !strings.HasPrefix(normalized, "RETRIEVE_") && !strings.HasPrefix(normalized, "SEARCH_")
}
