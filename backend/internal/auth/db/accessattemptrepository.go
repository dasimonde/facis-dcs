package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type AccessAttempt struct {
	ID            int64     `db:"id"`
	AttemptBy     *string   `db:"user_id"`
	IPAddress     string    `db:"ip_address"`
	AttemptedAt   time.Time `db:"attempted_at"`
	Success       bool      `db:"success"`
	Service       string    `db:"service"`
	Method        string    `db:"method"`
	Roles         string    `db:"roles"`
	Scope         string    `db:"scope"`
	DID           *string   `db:"did"`
	Justification string    `db:"justification"`
}

type AccessAttemptRepo interface {
	Create(ctx context.Context, tx *sqlx.Tx, data AccessAttempt) error
	ReadByUserID(ctx context.Context, tx *sqlx.Tx, userID string) ([]AccessAttempt, error)
	ReadByIP(ctx context.Context, tx *sqlx.Tx, ip string) ([]AccessAttempt, error)
	CountFailedAttempts(ctx context.Context, tx *sqlx.Tx, userID string, since time.Time) (int, error)
	CountFailedAttemptsByIP(ctx context.Context, tx *sqlx.Tx, ip string, since time.Time) (int, error)
	DeleteOlderThan(ctx context.Context, tx *sqlx.Tx, before time.Time) error
}
