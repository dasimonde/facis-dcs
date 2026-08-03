package pq

import (
	"context"

	"digital-contracting-service/internal/dcstodcs/db"

	"github.com/jmoiron/sqlx"
)

// PostgresEraseRepository is the Postgres implementation of the peer
// erase-handshake ledger (contract_erasures).
type PostgresEraseRepository struct {
	DB *sqlx.DB
}

func (r *PostgresEraseRepository) Upsert(ctx context.Context, did, peerDID string) error {
	_, err := r.DB.ExecContext(ctx, `
		INSERT INTO contract_erasures (did, peer_did)
		VALUES ($1, $2)
		ON CONFLICT (did, peer_did) DO NOTHING`,
		did, peerDID)
	return err
}

func (r *PostgresEraseRepository) RecordAttempt(ctx context.Context, did, peerDID string) error {
	_, err := r.DB.ExecContext(ctx, `
		UPDATE contract_erasures
		SET retry_count = retry_count + 1, last_tried_at = now()
		WHERE did = $1 AND peer_did = $2`,
		did, peerDID)
	return err
}

func (r *PostgresEraseRepository) Confirm(ctx context.Context, did, peerDID string) error {
	_, err := r.DB.ExecContext(ctx, `
		UPDATE contract_erasures
		SET confirmed_at = now()
		WHERE did = $1 AND peer_did = $2 AND confirmed_at IS NULL`,
		did, peerDID)
	return err
}

func (r *PostgresEraseRepository) GetPending(ctx context.Context) ([]db.EraseRequest, error) {
	var requests []db.EraseRequest
	err := r.DB.SelectContext(ctx, &requests, `
		SELECT id, did, peer_did, requested_at, retry_count, last_tried_at, confirmed_at
		FROM contract_erasures
		WHERE confirmed_at IS NULL
		ORDER BY requested_at`)
	if err != nil {
		return nil, err
	}
	return requests, nil
}

func (r *PostgresEraseRepository) ListByDID(ctx context.Context, did string) ([]db.EraseRequest, error) {
	var requests []db.EraseRequest
	err := r.DB.SelectContext(ctx, &requests, `
		SELECT id, did, peer_did, requested_at, retry_count, last_tried_at, confirmed_at
		FROM contract_erasures
		WHERE did = $1
		ORDER BY requested_at`,
		did)
	if err != nil {
		return nil, err
	}
	return requests, nil
}
