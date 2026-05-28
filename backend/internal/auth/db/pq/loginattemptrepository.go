package pg

import (
	"context"
	"database/sql"
	"digital-contracting-service/internal/auth/db"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type PostgresLoginAttemptRepo struct {
}

func (r *PostgresLoginAttemptRepo) Create(ctx context.Context, tx *sqlx.Tx, data db.LoginAttempt) error {
	statement := `
        INSERT INTO login_attempts (
            attempt_by, ip_address, attempted_at, success, service, method
        ) VALUES ($1, $2, $3, $4, $5, $6)
    `
	_, err := tx.ExecContext(ctx, statement,
		data.AttemptBy, data.IPAddress, data.AttemptedAt, data.Success, data.Service, data.Method)
	return err
}

func (r *PostgresLoginAttemptRepo) ReadByUserID(ctx context.Context, tx *sqlx.Tx, attemptBy string) ([]db.LoginAttempt, error) {
	query := `
        SELECT id, attempt_by, ip_address, attempted_at, success
        FROM login_attempts
        WHERE attempt_by = $1
        ORDER BY attempted_at DESC
    `
	var attempts []db.LoginAttempt
	err := tx.SelectContext(ctx, &attempts, query, attemptBy)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []db.LoginAttempt{}, nil
		}
		return []db.LoginAttempt{}, err
	}
	return attempts, nil
}

func (r *PostgresLoginAttemptRepo) ReadByIP(ctx context.Context, tx *sqlx.Tx, ip string) ([]db.LoginAttempt, error) {
	query := `
        SELECT id, attempt_by, ip_address, attempted_at, success
        FROM login_attempts
        WHERE ip_address = $1
        ORDER BY attempted_at DESC
    `
	var attempts []db.LoginAttempt
	err := tx.SelectContext(ctx, &attempts, query, ip)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []db.LoginAttempt{}, nil
		}
		return []db.LoginAttempt{}, err
	}
	return attempts, nil
}

func (r *PostgresLoginAttemptRepo) CountFailedAttempts(ctx context.Context, tx *sqlx.Tx, attemptBy string, since time.Time) (int, error) {
	query := `
        SELECT COUNT(*)
        FROM login_attempts
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

func (r *PostgresLoginAttemptRepo) CountFailedAttemptsByIP(ctx context.Context, tx *sqlx.Tx, ip string, since time.Time) (int, error) {
	query := `
        SELECT COUNT(*)
        FROM login_attempts
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

func (r *PostgresLoginAttemptRepo) DeleteOlderThan(ctx context.Context, tx *sqlx.Tx, before time.Time) error {
	statement := `
        DELETE FROM login_attempts
        WHERE attempted_at < $1
    `
	_, err := tx.ExecContext(ctx, statement, before)
	if err != nil {
		return fmt.Errorf("failed to delete old login attempts: %w", err)
	}
	return nil
}
