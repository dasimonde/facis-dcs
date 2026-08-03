// Package statuspublication provides the durable PostgreSQL-backed delivery
// queue for contract lifecycle status updates sent to the retained XFSC
// Status List Service.
package statuspublication

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/pdfgeneration/provenance"
)

const (
	pollInterval = 5 * time.Second
	maxRetry     = time.Minute
)

// EnqueueTx records a lifecycle publication intent in the same transaction as
// the state transition. Repeated delivery of the same state is idempotent.
func EnqueueTx(ctx context.Context, tx *sqlx.Tx, contractDID, status, reason string, effectiveAt time.Time) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO status_publication_queue
			(contract_did, status, reason, effective_at, created_at, next_attempt_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT (contract_did, status) DO NOTHING`,
		contractDID, status, reason, effectiveAt.UTC())
	if err != nil {
		return fmt.Errorf("enqueue %s status publication for %s: %w", status, contractDID, err)
	}
	return nil
}

type Worker struct {
	DB        *sqlx.DB
	Publisher provenance.StatusListPublisher
}

type entry struct {
	ContractDID string    `db:"contract_did"`
	Status      string    `db:"status"`
	Reason      string    `db:"reason"`
	EffectiveAt time.Time `db:"effective_at"`
	Attempts    int       `db:"attempt_count"`
}

// Run performs an immediate delivery pass, then retries pending entries every
// five seconds. Failures stay durable and back off to at most one minute,
// comfortably inside the five-minute synchronization target.
func (w *Worker) Run(ctx context.Context) {
	w.processAvailable(ctx)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.processAvailable(ctx)
		}
	}
}

func (w *Worker) processAvailable(ctx context.Context) {
	for {
		processed, err := w.ProcessOnce(ctx)
		if err != nil {
			log.Printf("statuspublication: %v", err)
			return
		}
		if !processed {
			return
		}
	}
}

// ProcessOnce claims and attempts one pending entry. It is exported for
// focused worker tests and operational drills.
func (w *Worker) ProcessOnce(ctx context.Context) (bool, error) {
	if w.DB == nil || w.Publisher == nil {
		return false, fmt.Errorf("status publication worker is not configured")
	}

	tx, err := w.DB.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	var item entry
	err = tx.GetContext(ctx, &item, `
		SELECT contract_did, status, reason, effective_at, attempt_count
		  FROM status_publication_queue
		 WHERE published_at IS NULL AND next_attempt_at <= CURRENT_TIMESTAMP
		 ORDER BY created_at
		 FOR UPDATE SKIP LOCKED
		 LIMIT 1`)
	if errors.Is(err, sql.ErrNoRows) {
		_ = tx.Rollback()
		return false, nil
	}
	if err != nil {
		_ = tx.Rollback()
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE status_publication_queue
		   SET attempt_count = attempt_count + 1,
		       next_attempt_at = CURRENT_TIMESTAMP + ($3 * INTERVAL '1 second')
		 WHERE contract_did = $1 AND status = $2`,
		item.ContractDID, item.Status, retryDelay(item.Attempts+1)/time.Second); err != nil {
		_ = tx.Rollback()
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}

	_, publishErr := w.Publisher.PublishStatus(ctx, item.ContractDID, item.Status, item.Reason, item.EffectiveAt)
	if publishErr != nil {
		_, err = w.DB.ExecContext(ctx, `
			UPDATE status_publication_queue SET last_error = $3
			 WHERE contract_did = $1 AND status = $2`,
			item.ContractDID, item.Status, publishErr.Error())
		if err != nil {
			return true, fmt.Errorf("publish %s for %s failed (%v), and recording failure failed: %w",
				item.Status, item.ContractDID, publishErr, err)
		}
		return true, nil
	}
	_, err = w.DB.ExecContext(ctx, `
		UPDATE status_publication_queue
		   SET published_at = CURRENT_TIMESTAMP, last_error = NULL
		 WHERE contract_did = $1 AND status = $2`,
		item.ContractDID, item.Status)
	return true, err
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Second << min(attempt-1, 6)
	if delay > maxRetry {
		return maxRetry
	}
	return delay
}
