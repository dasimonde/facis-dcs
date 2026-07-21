// Package datatype holds base-level, domain-agnostic value types shared by
// every domain (audit log entries, outbox events, JSON wrappers, pagination).
package datatype

import (
	"encoding/json"
	"time"
)

// AuditLogEntry is one entry of the tamper-evident audit trail. ResLogPredCID
// chains it to the previous IPFS-anchored entry for the same resource, so
// retroactively editing an entry breaks that resource's chain. Global tamper
// evidence — across resources and over the batch as a whole — comes from the
// Merkle checkpoint the entry is committed to (see AuditCheckpoint), not from
// a per-entry global link. See base/event.OutboxProcessor, which anchors these.
type AuditLogEntry struct {
	ID            int64           `json:"id"`
	Component     string          `json:"component"`
	EventType     string          `json:"event_type"`
	EventData     json.RawMessage `json:"event_data"`
	DID           *string         `json:"did"`
	CreatedAt     time.Time       `json:"created_at"`
	ResLogPredCID *string         `json:"res_log_pred_cid"`
}
