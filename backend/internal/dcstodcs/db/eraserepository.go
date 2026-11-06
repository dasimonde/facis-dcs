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
	// RefusedAt records when the peer refused the request for good; the
	// request is not sent again.
	RefusedAt *time.Time `db:"refused_at"`
	LastError *string    `db:"last_error"`
}

// EraseRepository persists the peer erase-handshake ledger. Kept separate
// from the sync_fails PDF-ship retry queue so ships and erasures never mix.
type EraseRepository interface {
	// Upsert registers an erase request toward a peer; repeating it for an
	// existing row is a no-op (the original requested_at stands).
	Upsert(ctx context.Context, did, peerDID string) error
	// RecordAttempt counts one failed delivery attempt and keeps its error.
	RecordAttempt(ctx context.Context, did, peerDID, lastError string) error
	// Refuse ends a request the peer refused for good and keeps its answer.
	Refuse(ctx context.Context, did, peerDID, lastError string) error
	// Confirm records the peer's acknowledgement; idempotent (the first
	// confirmation timestamp stands).
	Confirm(ctx context.Context, did, peerDID string) error
	// GetPending returns the requests that are neither confirmed nor refused
	// and whose backoff has passed: five seconds more per failed attempt, up
	// to five minutes.
	GetPending(ctx context.Context) ([]EraseRequest, error)
	// ListByDID returns the contract's requests, all peers, oldest first.
	ListByDID(ctx context.Context, did string) ([]EraseRequest, error)
}
