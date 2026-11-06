package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/auth/db"
)

type PostgresAccessAttemptRepo struct {
}

func (r *PostgresAccessAttemptRepo) Create(ctx context.Context, tx *sqlx.Tx, data db.AccessAttempt) error {
	statement := `
        INSERT INTO access_attempts (
            attempt_by, ip_address, attempted_at, success, service, method, roles, scope, did, justification, outcome, lockout_key
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
    `
	_, err := tx.ExecContext(ctx, statement,
		data.AttemptBy, data.IPAddress, data.AttemptedAt.UTC(), data.Success, data.Service, data.Method, data.Roles, data.Scope, data.DID, data.Justification, data.Outcome, data.LockoutKey)
	return err
}

func (r *PostgresAccessAttemptRepo) ReadByUserID(ctx context.Context, tx *sqlx.Tx, attemptBy string) ([]db.AccessAttempt, error) {
	query := `
        SELECT id, attempt_by, ip_address, attempted_at, success, service, method, roles, scope, did, justification, outcome
        FROM access_attempts
        WHERE attempt_by = $1
        ORDER BY attempted_at DESC
    `
	var attempts []db.AccessAttempt
	err := tx.SelectContext(ctx, &attempts, query, attemptBy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []db.AccessAttempt{}, nil
		}
		return []db.AccessAttempt{}, err
	}
	return attempts, nil
}

func (r *PostgresAccessAttemptRepo) ReadByIP(ctx context.Context, tx *sqlx.Tx, ip string) ([]db.AccessAttempt, error) {
	query := `
        SELECT id, attempt_by, ip_address, attempted_at, success, service, method, roles, scope, did, justification, outcome
        FROM access_attempts
        WHERE ip_address = $1
        ORDER BY attempted_at DESC
    `
	var attempts []db.AccessAttempt
	err := tx.SelectContext(ctx, &attempts, query, ip)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []db.AccessAttempt{}, nil
		}
		return []db.AccessAttempt{}, err
	}
	return attempts, nil
}

func (r *PostgresAccessAttemptRepo) CountFailedAttempts(ctx context.Context, tx *sqlx.Tx, attemptBy string, since time.Time) (int, error) {
	query := `
        SELECT COUNT(*)
        FROM access_attempts
        WHERE attempt_by = $1
          AND success = FALSE
          AND attempted_at > $2
    `
	var count int
	err := tx.GetContext(ctx, &count, query, attemptBy, since.UTC())
	if err != nil {
		return 0, fmt.Errorf("failed to count failed attempts for user %s: %w", attemptBy, err)
	}
	return count, nil
}

func (r *PostgresAccessAttemptRepo) CountLockoutAttempts(ctx context.Context, tx *sqlx.Tx, key string, since time.Time) (int, error) {
	var count int
	err := tx.GetContext(ctx, &count, `SELECT COUNT(*) FROM access_attempts WHERE lockout_key = $1 AND attempted_at > $2`, key, since.UTC())
	if err != nil {
		return 0, fmt.Errorf("failed to count lockout attempts for %s: %w", key, err)
	}
	return count, nil
}

func (r *PostgresAccessAttemptRepo) ReadPage(ctx context.Context, tx *sqlx.Tx, before *db.AccessAttemptCursor, limit int) ([]db.AccessAttempt, error) {
	query := `SELECT id, attempt_by, ip_address, attempted_at, success, service, method, roles, scope, did, justification, outcome FROM access_attempts`
	args := []any{}
	if before != nil {
		query += ` WHERE (attempted_at, id) < ($1, $2)`
		args = append(args, before.AttemptedAt.UTC(), before.ID)
	}
	args = append(args, limit)
	query += fmt.Sprintf(` ORDER BY attempted_at DESC, id DESC LIMIT $%d`, len(args))
	attempts := []db.AccessAttempt{}
	err := tx.SelectContext(ctx, &attempts, query, args...)
	return attempts, err
}
