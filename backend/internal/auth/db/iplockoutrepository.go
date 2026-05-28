package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type IPLockout struct {
	ID          int64     `db:"id"`
	IPAddress   string    `db:"ip_address"`
	LockedUntil time.Time `db:"locked_until"`
	CreatedAt   time.Time `db:"created_at"`
}

type IPLockoutRepo interface {
	IsLocked(ctx context.Context, tx *sqlx.Tx, ip string) (bool, *time.Time, error)
	SetLockout(ctx context.Context, tx *sqlx.Tx, ip string, until time.Time) error
	ClearLockout(ctx context.Context, tx *sqlx.Tx, ip string) error
}
