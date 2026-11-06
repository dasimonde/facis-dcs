package pg

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/auth/db"
)

type PostgresLockoutRepo struct{}

func (r *PostgresLockoutRepo) LockedUntil(ctx context.Context, tx *sqlx.Tx, key string) (time.Time, error) {
	var until time.Time
	err := tx.GetContext(ctx, &until, `SELECT locked_until FROM lockouts WHERE lockout_key = $1`, key)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	return until, err
}

func (r *PostgresLockoutRepo) SetLockout(ctx context.Context, tx *sqlx.Tx, key string, until time.Time) error {
	_, err := tx.ExecContext(ctx, `
        INSERT INTO lockouts (lockout_key, locked_until)
        VALUES ($1, $2)
        ON CONFLICT (lockout_key) DO UPDATE SET locked_until = $2`, key, until)
	return err
}

func (r *PostgresLockoutRepo) ClearLockout(ctx context.Context, tx *sqlx.Tx, key string) (bool, error) {
	res, err := tx.ExecContext(ctx, `UPDATE lockouts SET locked_until = NOW() WHERE lockout_key = $1 AND locked_until > NOW()`, key)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *PostgresLockoutRepo) ListActive(ctx context.Context, tx *sqlx.Tx) ([]db.Lockout, error) {
	locks := []db.Lockout{}
	err := tx.SelectContext(ctx, &locks, `SELECT lockout_key, locked_until FROM lockouts WHERE locked_until > NOW() ORDER BY locked_until DESC`)
	return locks, err
}
