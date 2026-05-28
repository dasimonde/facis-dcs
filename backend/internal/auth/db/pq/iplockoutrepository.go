package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type PostgresIPLockoutRepo struct{}

func (r *PostgresIPLockoutRepo) IsLocked(ctx context.Context, tx *sqlx.Tx, ip string) (bool, *time.Time, error) {
	query := `
        SELECT locked_until FROM ip_lockouts
        WHERE ip_address = $1 AND locked_until > NOW()
        LIMIT 1
    `
	var lockedUntil time.Time
	err := tx.GetContext(ctx, &lockedUntil, query, ip)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, &lockedUntil, nil
}

func (r *PostgresIPLockoutRepo) SetLockout(ctx context.Context, tx *sqlx.Tx, ip string, until time.Time) error {
	statement := `
        INSERT INTO ip_lockouts (ip_address, locked_until)
        VALUES ($1, $2)
        ON CONFLICT (ip_address) DO UPDATE SET locked_until = $2
    `
	_, err := tx.ExecContext(ctx, statement, ip, until)
	fmt.Println(err)
	return err
}

func (r *PostgresIPLockoutRepo) ClearLockout(ctx context.Context, tx *sqlx.Tx, ip string) error {
	statement := `DELETE FROM ip_lockouts WHERE ip_address = $1`
	_, err := tx.ExecContext(ctx, statement, ip)
	return err
}
