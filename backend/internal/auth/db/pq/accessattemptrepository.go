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
            attempt_by, ip_address, attempted_at, success, service, method, roles, scope, did, justification
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
    `
	_, err := tx.ExecContext(ctx, statement,
		data.AttemptBy, data.IPAddress, data.AttemptedAt, data.Success, data.Service, data.Method, data.Roles, data.Scope, data.DID, data.Justification)
	return err
}

func (r *PostgresAccessAttemptRepo) ReadByUserID(ctx context.Context, tx *sqlx.Tx, attemptBy string) ([]db.AccessAttempt, error) {
	query := `
        SELECT id, attempt_by, ip_address, attempted_at, success, service, method, roles, scope, did, justification
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
        SELECT id, attempt_by, ip_address, attempted_at, success, service, method, roles, scope, did, justification
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
	err := tx.GetContext(ctx, &count, query, attemptBy, since)
	if err != nil {
		return 0, fmt.Errorf("failed to count failed attempts for user %s: %w", attemptBy, err)
	}
	return count, nil
}

func (r *PostgresAccessAttemptRepo) CountFailedAttemptsByIP(ctx context.Context, tx *sqlx.Tx, ip string, since time.Time) (int, error) {
	query := `
        SELECT COUNT(*)
        FROM access_attempts
        WHERE ip_address = $1
          AND success = FALSE
          AND attempted_at > $2
    `
	var count int
	err := tx.GetContext(ctx, &count, query, ip, since)
	if err != nil {
		return 0, fmt.Errorf("failed to count failed attempts for ip %s: %w", ip, err)
	}
	return count, nil
}

func (r *PostgresAccessAttemptRepo) DeleteOlderThan(ctx context.Context, tx *sqlx.Tx, before time.Time) error {
	statement := `
        DELETE FROM access_attempts
        WHERE attempted_at < $1
    `
	_, err := tx.ExecContext(ctx, statement, before)
	if err != nil {
		return fmt.Errorf("failed to delete old login attempts: %w", err)
	}
	return nil
}
