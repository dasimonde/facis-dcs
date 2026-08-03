package db

import (
	"context"
	"time"
)

// EraseRequest is one row of the peer erase-handshake ledger: an erase
// request toward one counterparty instance for one contract. A row without
// ConfirmedAt is pending and is retried by the erase scheduler; ConfirmedAt
// records when the peer acknowledged shredding its wrapped CEKs.
type EraseRequest struct {
	ID          uint64     `db:"id"`
	DID         string     `db:"did"`
	PeerDID     string     `db:"peer_did"`
	RequestedAt time.Time  `db:"requested_at"`
	RetryCount  int        `db:"retry_count"`
	LastTriedAt *time.Time `db:"last_tried_at"`
	ConfirmedAt *time.Time `db:"confirmed_at"`
}

// EraseRepository persists the peer erase-handshake ledger. Kept separate
// from the sync_fails PDF-ship retry queue so ships and erasures never mix.
type EraseRepository interface {
	// Upsert registers an erase request toward a peer; repeating it for an
	// existing row is a no-op (the original requested_at stands).
	Upsert(ctx context.Context, did, peerDID string) error
	// RecordAttempt counts one failed delivery attempt.
	RecordAttempt(ctx context.Context, did, peerDID string) error
	// Confirm records the peer's acknowledgement; idempotent (the first
	// confirmation timestamp stands).
	Confirm(ctx context.Context, did, peerDID string) error
	// GetPending returns every not-yet-confirmed request.
	GetPending(ctx context.Context) ([]EraseRequest, error)
	// ListByDID returns the contract's requests, all peers, oldest first.
	ListByDID(ctx context.Context, did string) ([]EraseRequest, error)
}
