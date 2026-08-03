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
// The entry splits into a public header (component, event type, DID,
// timestamps, chain link, nonce — always readable) and a private body:
// EventData is stored encrypted under the CEK of the scope named by
// CEKScopeKind/CEKScopeID. Shredding that CEK erases the body while the leaf
// hash — computed over the stored object as-is — keeps verifying.
type AuditLogEntry struct {
	ID        int64  `json:"id"`
	Component string `json:"component"`
	EventType string `json:"event_type"`
	// EventData is the entry body. At rest it is the AES-256-GCM ciphertext
	// (JSON base64 string); readers replace it with the decrypted plaintext,
	// or with ErasedEventData once the scope's CEK is shredded.
	EventData     json.RawMessage `json:"event_data"`
	DID           *string         `json:"did"`
	CreatedAt     time.Time       `json:"created_at"`
	ResLogPredCID *string         `json:"res_log_pred_cid"`
	CEKScopeKind  string          `json:"cek_scope_kind,omitempty"`
	CEKScopeID    string          `json:"cek_scope_id,omitempty"`
	// Nonce blinds this entry's Merkle leaf. Without it the leaf hash would be
	// an unsalted commitment over highly guessable content (component, event
	// type, DID, second-precision timestamp), so anyone holding a published
	// proof could brute-force candidate entries and confirm a guess. Whoever is
	// entitled to the entry gets the nonce with it and can recompute the leaf;
	// nobody else can.
	Nonce string `json:"nonce"`
}

// ErasedEventData is the defined body of an audit entry whose scope CEK was
// shredded: the header stays readable, the body is gone for good.
var ErasedEventData = json.RawMessage(`{"erased":true}`)

// IsErased reports whether the entry's body was erased by a CEK shred.
func (e AuditLogEntry) IsErased() bool {
	return string(e.EventData) == string(ErasedEventData)
}
