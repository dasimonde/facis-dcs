package artifactstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// CEKRecord is one wrapped-CEK row. A shredded row stays in place as the
// destruction record (shredded_at/by/reason); it is never hard-deleted.
type CEKRecord struct {
	ScopeKind    string     `db:"scope_kind"`
	ScopeID      string     `db:"scope_id"`
	RecipientDID string     `db:"recipient_did"`
	WrappedCEK   []byte     `db:"wrapped_cek"`
	CreatedAt    time.Time  `db:"created_at"`
	ShreddedAt   *time.Time `db:"shredded_at"`
	ShreddedBy   *string    `db:"shredded_by"`
	ShredReason  *string    `db:"shred_reason"`
}

// CEKRepository persists wrapped-CEK records.
type CEKRepository interface {
	// Fetch returns the scope's record for the recipient — a live one if it
	// exists, otherwise the latest shredded one — or nil.
	Fetch(ctx context.Context, scope Scope, recipientDID string) (*CEKRecord, error)
	// List returns every record of the scope, all recipients, live and
	// shredded, oldest first.
	List(ctx context.Context, scope Scope) ([]CEKRecord, error)
	// Insert persists a wrapped CEK; false without error means a live record
	// already exists (concurrent creator) or the scope was shredded — either
	// way the caller must re-Fetch instead of using its own key.
	Insert(ctx context.Context, scope Scope, recipientDID string, wrappedCEK []byte) (bool, error)
	// Shred marks every live record of the scope destroyed and returns how
	// many were affected.
	Shred(ctx context.Context, scope Scope, shreddedBy, reason string) (int64, error)
}

type PostgresCEKRepo struct {
	DB *sqlx.DB
}

func (r *PostgresCEKRepo) Fetch(ctx context.Context, scope Scope, recipientDID string) (*CEKRecord, error) {
	var record CEKRecord
	err := r.DB.GetContext(ctx, &record, `
		SELECT scope_kind, scope_id, recipient_did, wrapped_cek, created_at, shredded_at, shredded_by, shred_reason
		FROM content_encryption_keys
		WHERE scope_kind = $1 AND scope_id = $2 AND recipient_did = $3
		ORDER BY (shredded_at IS NULL) DESC, created_at DESC
		LIMIT 1`,
		string(scope.Kind), scope.ID, recipientDID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &record, nil
}

func (r *PostgresCEKRepo) List(ctx context.Context, scope Scope) ([]CEKRecord, error) {
	var records []CEKRecord
	err := r.DB.SelectContext(ctx, &records, `
		SELECT scope_kind, scope_id, recipient_did, wrapped_cek, created_at, shredded_at, shredded_by, shred_reason
		FROM content_encryption_keys
		WHERE scope_kind = $1 AND scope_id = $2
		ORDER BY created_at`,
		string(scope.Kind), scope.ID)
	if err != nil {
		return nil, err
	}
	return records, nil
}

// Insert persists a wrapped CEK inside a transaction that holds the scope's
// advisory lock, so it serializes against Shred: once a scope has any shredded
// record, no new live record can ever be inserted for it — a shredded scope
// must not regrow keys.
func (r *PostgresCEKRepo) Insert(ctx context.Context, scope Scope, recipientDID string, wrappedCEK []byte) (bool, error) {
	tx, err := r.DB.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	if err := lockScope(ctx, tx, scope); err != nil {
		return false, err
	}

	var shredded bool
	if err := tx.GetContext(ctx, &shredded, `
		SELECT EXISTS (
			SELECT 1 FROM content_encryption_keys
			WHERE scope_kind = $1 AND scope_id = $2 AND shredded_at IS NOT NULL)`,
		string(scope.Kind), scope.ID); err != nil {
		return false, err
	}
	if shredded {
		return false, nil
	}

	result, err := tx.ExecContext(ctx, `
		INSERT INTO content_encryption_keys (scope_kind, scope_id, recipient_did, wrapped_cek)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (scope_kind, scope_id, recipient_did) WHERE shredded_at IS NULL DO NOTHING`,
		string(scope.Kind), scope.ID, recipientDID, wrappedCEK)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (r *PostgresCEKRepo) Shred(ctx context.Context, scope Scope, shreddedBy, reason string) (int64, error) {
	tx, err := r.DB.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	affected, err := r.ShredTx(ctx, tx, scope, shreddedBy, reason)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return affected, nil
}

// ShredTx marks every live record of the scope destroyed inside the caller's
// transaction (so the KEY_SHREDDED audit event commits atomically with the
// destruction). It takes the scope's advisory lock first: a concurrent Insert
// either commits before the shred and is caught by the UPDATE, or waits and
// then refuses because the scope has shredded records — live rows can never
// regrow after a shred.
func (r *PostgresCEKRepo) ShredTx(ctx context.Context, tx *sqlx.Tx, scope Scope, shreddedBy, reason string) (int64, error) {
	if err := lockScope(ctx, tx, scope); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE content_encryption_keys
		SET shredded_at = now(), shredded_by = $3, shred_reason = $4
		WHERE scope_kind = $1 AND scope_id = $2 AND shredded_at IS NULL`,
		string(scope.Kind), scope.ID, shreddedBy, reason)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// lockScope takes the scope's transaction-lifetime advisory lock, the
// serialization point between CEK inserts and shreds.
func lockScope(ctx context.Context, tx *sqlx.Tx, scope Scope) error {
	if _, err := tx.ExecContext(ctx,
		`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, scope.AAD()); err != nil {
		return fmt.Errorf("lock cek scope %s: %w", scope.AAD(), err)
	}
	return nil
}
