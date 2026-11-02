package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// Lockout is one lock of a lockout key: a client address or a credential
// holder key (FR-UC-01-4).
type Lockout struct {
	Key         string    `db:"lockout_key" json:"key"`
	LockedUntil time.Time `db:"locked_until" json:"locked_until"`
}

type LockoutRepo interface {
	// LockedUntil returns the end of the last lock of key, or the zero time.
	// The lock is active while the end is in the future.
	LockedUntil(ctx context.Context, tx *sqlx.Tx, key string) (time.Time, error)
	SetLockout(ctx context.Context, tx *sqlx.Tx, key string, until time.Time) error
	// ClearLockout ends an active lock now. It reports false when key has no
	// active lock.
	ClearLockout(ctx context.Context, tx *sqlx.Tx, key string) (bool, error)
	ListActive(ctx context.Context, tx *sqlx.Tx) ([]Lockout, error)
}
