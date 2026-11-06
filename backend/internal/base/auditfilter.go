// Package base holds the shared kernel used by every domain: cross-cutting
// helpers (this file), plus the conf, datatype, db, event, identity, ipfs,
// tsa, and validation subpackages. Domains depend on base; base never
// depends on a domain.
package base

import "strings"

// archiveLookupEventTypes are the read operations on the archive. They stay in
// the anchored audit trail: DCS-FR-CSA-18 requires every archive retrieval in
// the immutable audit log.
var archiveLookupEventTypes = map[string]bool{
	"RETRIEVE_ARCHIVED_CONTRACTS": true,
	"SEARCH_ARCHIVED_CONTRACTS":   true,
}

// IsAuditVisibleEventType returns whether an event should be shown in audit results.
// Read-only lookup events are useful operational traces, but they are not findings.
// Archive lookups are the exception.
func IsAuditVisibleEventType(eventType string) bool {
	normalized := strings.ToUpper(strings.TrimSpace(eventType))
	if normalized == "" || archiveLookupEventTypes[normalized] {
		return true
	}
	return !strings.HasPrefix(normalized, "RETRIEVE_") && !strings.HasPrefix(normalized, "SEARCH_")
}
