// Package pq is the Postgres implementation of dcstodcs's sync repository
// (sync-fail retry queue + cross-instance sync provenance store).
package pq

import (
	"context"
	"database/sql"
	"errors"

	"digital-contracting-service/internal/dcstodcs/db"

	"github.com/jmoiron/sqlx"
)

type PostgresSyncRepository struct{}

// CreateOrUpdateSyncFailEntry upserts the retry entry and reports whether
// THIS call is the first to observe a trust-gate failure for it.
// gate_incident_recorded is a per-row latch: the "existing" CTE reads its
// pre-upsert value from the same statement-start snapshot the INSERT/UPDATE
// itself observes, so shouldRecordIncident is true exactly once — whether
// the row is being freshly created by a gate failure, or already existed for
// an unrelated reason (e.g. the PDF not being stored yet, isGateFailure
// false) and only now, on a later retry, actually hits a gate failure.
func (r PostgresSyncRepository) CreateOrUpdateSyncFailEntry(ctx context.Context, tx *sqlx.Tx, did string, isGateFailure bool) (bool, error) {
	statement := `
        WITH existing AS (
            SELECT gate_incident_recorded FROM sync_fails WHERE did = $1
        ), upsert AS (
            INSERT INTO sync_fails (did, retry_count, created_at, last_tried_at, gate_incident_recorded)
            VALUES ($1, 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, $2)
            ON CONFLICT (did) DO UPDATE SET
                retry_count            = sync_fails.retry_count + 1,
                last_tried_at          = CURRENT_TIMESTAMP,
                gate_incident_recorded = sync_fails.gate_incident_recorded OR $2
            RETURNING did
        )
        SELECT $2 AND NOT COALESCE((SELECT gate_incident_recorded FROM existing), FALSE)
        FROM upsert
    `
	var shouldRecordIncident bool
	if err := tx.GetContext(ctx, &shouldRecordIncident, statement, did, isGateFailure); err != nil {
		return false, err
	}
	return shouldRecordIncident, nil
}

func (r PostgresSyncRepository) DeleteSyncFailEntry(ctx context.Context, tx *sqlx.Tx, did string) error {
	statement := `
        DELETE FROM sync_fails WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}

func (r PostgresSyncRepository) GetPendingSyncFails(ctx context.Context, tx *sqlx.Tx) ([]db.SyncFail, error) {
	query := `
        SELECT *
        FROM sync_fails
    `
	var syncFails []db.SyncFail
	err := tx.SelectContext(ctx, &syncFails, query)
	if err != nil {
		return nil, err
	}
	return syncFails, nil
}

func (r PostgresSyncRepository) UpsertSyncSignature(ctx context.Context, tx *sqlx.Tx, sig db.SyncSignature) error {
	statement := `
        INSERT INTO contract_sync_signatures (did, contract_version, from_peer_did, jades_signature, received_at, poa_evidence, poa_revalidated_at)
        VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP, $5, $6)
        ON CONFLICT (did) DO UPDATE SET
            contract_version = EXCLUDED.contract_version,
            from_peer_did    = EXCLUDED.from_peer_did,
            jades_signature  = EXCLUDED.jades_signature,
            received_at      = CURRENT_TIMESTAMP,
            poa_evidence     = EXCLUDED.poa_evidence,
            poa_revalidated_at = EXCLUDED.poa_revalidated_at
    `
	_, err := tx.ExecContext(ctx, statement, sig.DID, sig.ContractVersion, sig.FromPeerDID, sig.JadesSignature, sig.PoAEvidence, sig.PoARevalidatedAt)
	return err
}

func (r PostgresSyncRepository) GetSyncSignature(ctx context.Context, tx *sqlx.Tx, did string) (*db.SyncSignature, error) {
	query := `
        SELECT did, contract_version, from_peer_did, jades_signature, received_at, poa_evidence, poa_revalidated_at
        FROM contract_sync_signatures
        WHERE did = $1
    `
	var sig db.SyncSignature
	err := tx.GetContext(ctx, &sig, query, did)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &sig, nil
}
