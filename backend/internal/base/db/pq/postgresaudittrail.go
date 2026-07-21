// Package pq is the Postgres implementation of the base-level audit-trail
// repository interface (base/db.AuditTrailRepository).
package pq

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
)

type PostgresAuditTrailRepository struct{}

// UpdateLogCID stores the IPFS CID of the most recently anchored audit entry
// for (component, did) — the "predecessor CID" the OutboxProcessor reads back
// on that resource's next event to build its tamper-evident hash chain.
// Tamper evidence ACROSS resources comes from the Merkle checkpoints
// (AppendCheckpoint), not from a row in this table.
func (r *PostgresAuditTrailRepository) UpdateLogCID(ctx context.Context, tx *sqlx.Tx, component string, did string, lastLogDID *string) error {
	statement := `UPDATE audit_trail_log SET last_log_cid = $3 WHERE component = $1 AND did = $2`
	result, err := tx.ExecContext(ctx, statement, component, did, lastLogDID)
	if err != nil {
		return err
	}
	rows, _ := result.RowsAffected()

	if rows == 0 {
		statement := `
        INSERT INTO audit_trail_log (
            component, did, last_log_cid
        ) VALUES ($1, $2, $3)
    	`
		_, err = tx.ExecContext(ctx, statement, component, did, lastLogDID)
		if err != nil {
			return err
		}
		return nil
	}

	return err
}

func (r *PostgresAuditTrailRepository) ReadLogCIDs(ctx context.Context, tx *sqlx.Tx, component string) ([]*string, error) {
	query := `
        SELECT last_log_cid
        FROM audit_trail_log WHERE component = $1;
    `
	var result []*string
	err := tx.SelectContext(ctx, &result, query, component)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

func (r *PostgresAuditTrailRepository) ReadLogCID(ctx context.Context, tx *sqlx.Tx, component string, did string) (*string, error) {
	query := `
        SELECT last_log_cid
        FROM audit_trail_log WHERE did = $1 AND component = $2;
    `
	var result *string
	err := tx.GetContext(ctx, &result, query, did, component)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return result, nil
}

// AppendCheckpoint stores one Merkle checkpoint over an anchored batch. The
// checkpoint bytes themselves live in IPFS at cid; this row is the index that
// makes the head, the walk order and the pending timestamps findable.
func (r *PostgresAuditTrailRepository) AppendCheckpoint(ctx context.Context, tx *sqlx.Tx, cid, root string, prevRoot *string, leafCount int, tsaSignature *string) (int64, error) {
	statement := `
        INSERT INTO audit_checkpoints (cid, root, prev_root, leaf_count, tsa_signature, timestamped_at)
        VALUES ($1, $2, $3, $4, $5, CASE WHEN $5::text IS NULL THEN NULL ELSE CURRENT_TIMESTAMP END)
        RETURNING seq
    `
	var seq int64
	if err := tx.GetContext(ctx, &seq, statement, cid, root, prevRoot, leafCount, tsaSignature); err != nil {
		return 0, err
	}
	return seq, nil
}

func (r *PostgresAuditTrailRepository) ReadLatestCheckpointRoot(ctx context.Context, tx *sqlx.Tx) (*string, error) {
	query := `SELECT root FROM audit_checkpoints ORDER BY seq DESC LIMIT 1`
	var root *string
	if err := tx.GetContext(ctx, &root, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return root, nil
}

// ReadCheckpoints returns the most recent checkpoints first, which is the order
// the audit trail is read in.
func (r *PostgresAuditTrailRepository) ReadCheckpoints(ctx context.Context, tx *sqlx.Tx, limit int) ([]datatype.AuditCheckpointRecord, error) {
	query := `
        SELECT seq, cid, root, prev_root, leaf_count, tsa_signature, created_at, timestamped_at
        FROM audit_checkpoints ORDER BY seq DESC LIMIT $1
    `
	var records []datatype.AuditCheckpointRecord
	if err := tx.SelectContext(ctx, &records, query, limit); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *PostgresAuditTrailRepository) ReadCheckpointsAwaitingTimestamp(ctx context.Context, tx *sqlx.Tx, limit int) ([]datatype.AuditCheckpointRecord, error) {
	query := `
        SELECT seq, cid, root, prev_root, leaf_count, tsa_signature, created_at, timestamped_at
        FROM audit_checkpoints WHERE tsa_signature IS NULL ORDER BY seq ASC LIMIT $1
    `
	var records []datatype.AuditCheckpointRecord
	if err := tx.SelectContext(ctx, &records, query, limit); err != nil {
		return nil, err
	}
	return records, nil
}

func (r *PostgresAuditTrailRepository) UpdateCheckpointTimestamp(ctx context.Context, tx *sqlx.Tx, seq int64, tsaSignature string) error {
	statement := `
        UPDATE audit_checkpoints
        SET tsa_signature = $2, timestamped_at = CURRENT_TIMESTAMP
        WHERE seq = $1 AND tsa_signature IS NULL
    `
	_, err := tx.ExecContext(ctx, statement, seq, tsaSignature)
	return err
}

// AppendCheckpointLeaves records the ordered leaves of one checkpoint so an
// inclusion proof can be built for a single entry later.
func (r *PostgresAuditTrailRepository) AppendCheckpointLeaves(ctx context.Context, tx *sqlx.Tx, seq int64, entryCIDs, leafHashes []string) error {
	if len(entryCIDs) != len(leafHashes) {
		return errors.New("checkpoint leaves: CID and hash counts differ")
	}
	statement := `
        INSERT INTO audit_checkpoint_leaves (checkpoint_seq, idx, entry_cid, leaf_hash)
        VALUES ($1, $2, $3, $4)
    `
	for i := range entryCIDs {
		if _, err := tx.ExecContext(ctx, statement, seq, i, entryCIDs[i], leafHashes[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *PostgresAuditTrailRepository) ReadLatestCheckpoint(ctx context.Context, tx *sqlx.Tx) (*datatype.AuditCheckpointRecord, error) {
	query := `
        SELECT seq, cid, root, prev_root, leaf_count, tsa_signature, created_at, timestamped_at
        FROM audit_checkpoints ORDER BY seq DESC LIMIT 1
    `
	var record datatype.AuditCheckpointRecord
	if err := tx.GetContext(ctx, &record, query); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &record, nil
}

// ReadCheckpointForEntry returns the checkpoint that commits to entryCID, that
// checkpoint's leaf hashes in order, and the entry's index among them.
func (r *PostgresAuditTrailRepository) ReadCheckpointForEntry(ctx context.Context, tx *sqlx.Tx, entryCID string) (*datatype.AuditCheckpointRecord, []string, int, error) {
	var located struct {
		Seq int64 `db:"checkpoint_seq"`
		Idx int   `db:"idx"`
	}
	if err := tx.GetContext(ctx, &located, `
        SELECT checkpoint_seq, idx FROM audit_checkpoint_leaves WHERE entry_cid = $1 ORDER BY checkpoint_seq LIMIT 1
    `, entryCID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil, 0, nil
		}
		return nil, nil, 0, err
	}

	var record datatype.AuditCheckpointRecord
	if err := tx.GetContext(ctx, &record, `
        SELECT seq, cid, root, prev_root, leaf_count, tsa_signature, created_at, timestamped_at
        FROM audit_checkpoints WHERE seq = $1
    `, located.Seq); err != nil {
		return nil, nil, 0, err
	}

	var leafHashes []string
	if err := tx.SelectContext(ctx, &leafHashes, `
        SELECT leaf_hash FROM audit_checkpoint_leaves WHERE checkpoint_seq = $1 ORDER BY idx ASC
    `, located.Seq); err != nil {
		return nil, nil, 0, err
	}

	return &record, leafHashes, located.Idx, nil
}
