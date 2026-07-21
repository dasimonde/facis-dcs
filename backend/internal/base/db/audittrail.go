// Package db holds the base-level repository interfaces shared across
// domains: the audit-trail CID store (this file) and the transactional
// outbox (see PersistEvent/UpdateOutboxEvent). db/pq holds the Postgres
// implementations.
package db

import (
	"context"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
)

type AuditTrailRepository interface {
	ReadLogCID(ctx context.Context, tx *sqlx.Tx, component string, did string) (*string, error)
	ReadLogCIDs(ctx context.Context, tx *sqlx.Tx, component string) ([]*string, error)
	UpdateLogCID(ctx context.Context, tx *sqlx.Tx, component string, did string, logCID *string) error

	// AppendCheckpoint records one anchored batch and returns its sequence
	// number. tsaSignature is nil when the TSA did not answer in time; the
	// checkpoint is still valid and gets timestamped by a later pass.
	AppendCheckpoint(ctx context.Context, tx *sqlx.Tx, cid, root string, prevRoot *string, leafCount int, tsaSignature *string) (int64, error)
	ReadLatestCheckpointRoot(ctx context.Context, tx *sqlx.Tx) (*string, error)
	ReadCheckpoints(ctx context.Context, tx *sqlx.Tx, limit int) ([]datatype.AuditCheckpointRecord, error)
	ReadCheckpointsAwaitingTimestamp(ctx context.Context, tx *sqlx.Tx, limit int) ([]datatype.AuditCheckpointRecord, error)
	UpdateCheckpointTimestamp(ctx context.Context, tx *sqlx.Tx, seq int64, tsaSignature string) error
}
