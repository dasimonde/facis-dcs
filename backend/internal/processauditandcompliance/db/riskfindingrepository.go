// Package db declares the repository interfaces of the process-audit-and-
// compliance component; the Postgres implementations live in db/pg.
package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// RiskFinding is one compliance risk the continuous-monitoring sweep has
// flagged, held across sweeps so the same violation is reported once rather
// than on every run. DetailHash distinguishes several risks of the same type on
// the same contract (see the migration).
type RiskFinding struct {
	ContractDID     string     `db:"contract_did"`
	RiskType        string     `db:"risk_type"`
	DetailHash      string     `db:"detail_hash"`
	Detail          string     `db:"detail"`
	FirstDetectedAt time.Time  `db:"first_detected_at"`
	LastSeenAt      time.Time  `db:"last_seen_at"`
	ResolvedAt      *time.Time `db:"resolved_at"`
}

type RiskFindingRepo interface {
	// ListOpen returns every risk not yet resolved. The sweep compares it
	// against what it just detected: a finding present here but absent from
	// the sweep no longer holds and is closed.
	ListOpen(ctx context.Context, tx *sqlx.Tx) ([]RiskFinding, error)

	// Record registers a detected risk and reports whether this is a NEW
	// incident — either never seen before, or seen, resolved, and now
	// reoccurring. Only a new incident warrants an alert; a risk that merely
	// still holds updates last_seen_at silently.
	Record(ctx context.Context, tx *sqlx.Tx, finding RiskFinding) (bool, error)

	// Resolve closes an open finding whose risk the latest sweep no longer
	// detects.
	Resolve(ctx context.Context, tx *sqlx.Tx, finding RiskFinding, resolvedAt time.Time) error
}
