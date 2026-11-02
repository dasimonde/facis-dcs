package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

const (
	AccessOutcomeSuccess = "SUCCESS"
	AccessOutcomeDenied  = "DENIED"
	AccessOutcomeFailed  = "FAILED"
)

type AccessAttemptCursor struct {
	AttemptedAt time.Time
	ID          int64
}

type AccessAttempt struct {
	ID            int64     `db:"id"`
	AttemptBy     *string   `db:"attempt_by"`
	IPAddress     string    `db:"ip_address"`
	AttemptedAt   time.Time `db:"attempted_at"`
	Outcome       string    `db:"outcome"`
	Success       bool      `db:"success"`
	Service       string    `db:"service"`
	Method        string    `db:"method"`
	Roles         string    `db:"roles"`
	Scope         string    `db:"scope"`
	DID           *string   `db:"did"`
	Justification string    `db:"justification"`
	// LockoutKey is set when the attempt counts toward a lockout of that key.
	LockoutKey *string `db:"lockout_key"`
}

type AccessAttemptRepo interface {
	Create(ctx context.Context, tx *sqlx.Tx, data AccessAttempt) error
	ReadPage(ctx context.Context, tx *sqlx.Tx, before *AccessAttemptCursor, limit int) ([]AccessAttempt, error)
	ReadByUserID(ctx context.Context, tx *sqlx.Tx, userID string) ([]AccessAttempt, error)
	ReadByIP(ctx context.Context, tx *sqlx.Tx, ip string) ([]AccessAttempt, error)
	CountFailedAttempts(ctx context.Context, tx *sqlx.Tx, userID string, since time.Time) (int, error)
	// CountLockoutAttempts counts the attempts that carry key and were made after since.
	CountLockoutAttempts(ctx context.Context, tx *sqlx.Tx, key string, since time.Time) (int, error)
}
