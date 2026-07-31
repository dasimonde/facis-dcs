// Package db holds the repository interface backing dcstodcs's retry queue
// for failed peer broadcasts (SyncFail) and its cross-instance sync
// provenance store; db/pg holds the Postgres implementation.
package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type SyncFail struct {
	ID          uint64    `db:"id"`
	DID         string    `db:"did"`
	RetryCount  int       `db:"retry_count"`
	CreatedAt   time.Time `db:"created_at"`
	LastTriedAt time.Time `db:"last_tried_at"`
	// GateIncidentRecorded mirrors sync_fails.gate_incident_recorded (see
	// CreateOrUpdateSyncFailEntry) — read here only so GetPendingSyncFails'
	// `SELECT *` has a destination for every column; the retry scheduler
	// itself does not need to inspect it.
	GateIncidentRecorded bool `db:"gate_incident_recorded"`
}

// SyncSignature is the origin peer's JAdES signature over a synced
// contract's canonical representation (DCS-FR-SM-02), persisted on the
// receiving instance as the contract's cross-instance provenance artifact.
type SyncSignature struct {
	DID              string     `db:"did"`
	ContractVersion  int        `db:"contract_version"`
	FromPeerDID      string     `db:"from_peer_did"`
	JadesSignature   string     `db:"jades_signature"`
	ReceivedAt       time.Time  `db:"received_at"`
	PoAEvidence      []byte     `db:"poa_evidence"`
	PoARevalidatedAt *time.Time `db:"poa_revalidated_at"`
}

type SyncRepository interface {
	GetPendingSyncFails(ctx context.Context, tx *sqlx.Tx) ([]SyncFail, error)
	// CreateOrUpdateSyncFailEntry upserts a sync_fails entry for did.
	// isGateFailure marks this particular attempt as caused by the ADR-19
	// trust gate's agreement-credential check (as opposed to e.g. the PDF not
	// being stored yet); shouldRecordIncident reports whether THIS call is
	// the first one to observe a gate failure for this entry — true at most
	// once per entry, regardless of how many non-gate-failure or repeat
	// gate-failure retries created/touched it before or after. The caller
	// uses this to record a trust-gate denial incident exactly once.
	CreateOrUpdateSyncFailEntry(ctx context.Context, tx *sqlx.Tx, did string, isGateFailure bool) (shouldRecordIncident bool, err error)
	DeleteSyncFailEntry(ctx context.Context, tx *sqlx.Tx, peerDID string) error

	// UpsertSyncSignature stores the latest verified JAdES signature received
	// for a synced contract; GetSyncSignature returns nil when none exists.
	UpsertSyncSignature(ctx context.Context, tx *sqlx.Tx, sig SyncSignature) error
	GetSyncSignature(ctx context.Context, tx *sqlx.Tx, did string) (*SyncSignature, error)
}
