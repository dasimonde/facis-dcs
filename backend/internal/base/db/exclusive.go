package db

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// RunExclusive runs fn only when no other replica of this DCS instance runs
// the job called name at the same time. It reports false, and does not run
// fn, when another replica holds the job. The lock is a transaction-level
// advisory lock in a transaction that stays open while fn runs, so Postgres
// releases it when fn returns or when the replica loses its connection.
func RunExclusive(ctx context.Context, database *sqlx.DB, name string, fn func() error) (bool, error) {
	tx, err := database.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("lock job %s: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()

	var locked bool
	if err := tx.GetContext(ctx, &locked, `SELECT pg_try_advisory_xact_lock(hashtext('dcs-job:' || $1))`, name); err != nil {
		return false, fmt.Errorf("lock job %s: %w", name, err)
	}
	if !locked {
		return false, nil
	}
	return true, fn()
}

// RunLocked runs fn while no other replica runs the job called name. Unlike
// RunExclusive it waits until the job is free, so every replica runs fn, one
// after the other. A waiter holds no connection: it tries the lock again every
// lockRetryInterval, so the pool stays free for the holder's own transactions.
//
// Limit: polling, so waiters are not served in order; a session lock on a
// dedicated connection if order matters.
func RunLocked(ctx context.Context, database *sqlx.DB, name string, fn func() error) error {
	for {
		ran, err := RunExclusive(ctx, database, name, fn)
		if err != nil || ran {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("lock job %s: %w", name, ctx.Err())
		case <-time.After(lockRetryInterval):
		}
	}
}

// lockRetryInterval is how long a RunLocked waiter waits before it tries the
// lock again.
const lockRetryInterval = 100 * time.Millisecond
