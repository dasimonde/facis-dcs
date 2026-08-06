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

// Settlement is one party's verified statement that it reached its own
// settled state (NEGOTIATION -> SUBMITTED) on a named version of a contract
// document — the evidence the signing gate requires about the counterparty.
//
// The same row shape records both directions: FromPeerDID is the party that
// settled, ToPeerDID the instance it settled toward. A row this instance
// produced (FromPeerDID == own did:web) additionally carries the delivery
// bookkeeping — DeliveredAt stays NULL until the peer has accepted it, which
// is what lets a failed ship be re-delivered instead of vanishing.
type Settlement struct {
	DID             string     `db:"did"`
	FromPeerDID     string     `db:"from_peer_did"`
	ToPeerDID       string     `db:"to_peer_did"`
	ContractVersion int        `db:"contract_version"`
	DocumentDigest  string     `db:"document_digest"`
	SettledAt       time.Time  `db:"settled_at"`
	JadesSignature  string     `db:"jades_signature"`
	RecordedAt      time.Time  `db:"recorded_at"`
	DeliveredAt     *time.Time `db:"delivered_at"`
}

// SettlementWithdrawal is one party taking back the settlement it made of a
// named document version. It is queued and delivered like a settlement — the
// peer that holds the settlement has to be told, or its signing gate goes on
// reading a withdrawn agreement as evidence — and carries no signature: the
// JAdES is produced at delivery from these fields (dcstodcs.BuildSettlementWithdrawal).
//
// DocumentDigest is the version the withdrawn settlement covered, which is what
// binds the withdrawal to one settlement rather than to whichever one the peer
// happens to hold when it arrives.
type SettlementWithdrawal struct {
	DID            string     `db:"did"`
	FromPeerDID    string     `db:"from_peer_did"`
	ToPeerDID      string     `db:"to_peer_did"`
	DocumentDigest string     `db:"document_digest"`
	WithdrawnAt    time.Time  `db:"withdrawn_at"`
	RecordedAt     time.Time  `db:"recorded_at"`
	DeliveredAt    *time.Time `db:"delivered_at"`
}

type SyncRepository interface {
	GetPendingSyncFails(ctx context.Context, tx *sqlx.Tx, backoffBase, maxBackoff time.Duration, limit int) ([]SyncFail, error)
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

	// UpsertSettlement stores a settlement artifact, replacing an earlier one
	// for the same (contract, settling party, audience). GetSettlement returns
	// what the named party settled for the contract, or nil when it has not
	// settled — the signing gate reads absence as "not agreed".
	UpsertSettlement(ctx context.Context, tx *sqlx.Tx, settlement Settlement) error
	GetSettlement(ctx context.Context, tx *sqlx.Tx, did, fromPeerDID string) (*Settlement, error)

	// GetSettlementsBy returns every settlement one party recorded for a
	// contract, one per audience — read before DeleteSettlementsBy so the
	// withdrawal queued toward each audience can name the version that audience
	// was told about.
	GetSettlementsBy(ctx context.Context, tx *sqlx.Tx, did, fromPeerDID string) ([]Settlement, error)

	// DeleteSettlementsBy removes every settlement one party recorded for a
	// contract, toward any audience. Called with this instance's own did:web
	// when it withdraws the agreement it settled (the workflow engine's
	// rejection edges), which is the only way a party may move off a document
	// it has already agreed to, and with a counterparty's did:web when that
	// counterparty ships a withdrawal of its own.
	DeleteSettlementsBy(ctx context.Context, tx *sqlx.Tx, did, fromPeerDID string) error

	// UpsertSettlementWithdrawal queues a withdrawal toward one audience,
	// replacing an undelivered earlier one for the same (contract, withdrawing
	// party, audience). GetUndeliveredSettlementWithdrawals and
	// MarkSettlementWithdrawalDelivered are the settlement queue's delivery
	// bookkeeping applied to it.
	UpsertSettlementWithdrawal(ctx context.Context, tx *sqlx.Tx, withdrawal SettlementWithdrawal) error
	GetUndeliveredSettlementWithdrawals(ctx context.Context, tx *sqlx.Tx, fromPeerDID string) ([]SettlementWithdrawal, error)
	MarkSettlementWithdrawalDelivered(ctx context.Context, tx *sqlx.Tx, did, fromPeerDID, toPeerDID string) error

	// GetUndeliveredSettlements returns the settlements this instance produced
	// (fromPeerDID == its own did:web) that no peer has confirmed yet, for the
	// retry scheduler to re-ship; MarkSettlementDelivered closes one out.
	GetUndeliveredSettlements(ctx context.Context, tx *sqlx.Tx, fromPeerDID string) ([]Settlement, error)
	MarkSettlementDelivered(ctx context.Context, tx *sqlx.Tx, did, fromPeerDID, toPeerDID string) error
}
