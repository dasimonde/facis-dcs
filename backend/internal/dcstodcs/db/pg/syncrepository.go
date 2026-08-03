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

// UpsertSettlement replaces an earlier settlement for the same (contract,
// settling party, audience), delivery state included: a settlement of a new
// document supersedes the previous one and has not been delivered yet. The
// producer only writes when the artifact actually changed, so this never
// resets the delivery state of a settlement already in the peer's hands.
func (r PostgresSyncRepository) UpsertSettlement(ctx context.Context, tx *sqlx.Tx, s db.Settlement) error {
	statement := `
        INSERT INTO contract_settlements (did, from_peer_did, to_peer_did, contract_version, document_digest, settled_at, jades_signature, recorded_at, delivered_at)
        VALUES ($1, $2, $3, $4, $5, $6, $7, CURRENT_TIMESTAMP, $8)
        ON CONFLICT (did, from_peer_did, to_peer_did) DO UPDATE SET
            contract_version = EXCLUDED.contract_version,
            document_digest  = EXCLUDED.document_digest,
            settled_at       = EXCLUDED.settled_at,
            jades_signature  = EXCLUDED.jades_signature,
            recorded_at      = CURRENT_TIMESTAMP,
            delivered_at     = EXCLUDED.delivered_at
    `
	_, err := tx.ExecContext(ctx, statement, s.DID, s.FromPeerDID, s.ToPeerDID, s.ContractVersion, s.DocumentDigest, s.SettledAt.UTC(), s.JadesSignature, s.DeliveredAt)
	return err
}

// GetSettlement returns what fromPeerDID settled for the contract. The key
// also carries the audience, and an own settlement is stored once per
// recipient, so the newest is the answer.
func (r PostgresSyncRepository) GetSettlement(ctx context.Context, tx *sqlx.Tx, did, fromPeerDID string) (*db.Settlement, error) {
	query := `
        SELECT did, from_peer_did, to_peer_did, contract_version, document_digest, settled_at, jades_signature, recorded_at, delivered_at
        FROM contract_settlements
        WHERE did = $1 AND from_peer_did = $2
        ORDER BY settled_at DESC
        LIMIT 1
    `
	var settlement db.Settlement
	err := tx.GetContext(ctx, &settlement, query, did, fromPeerDID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &settlement, nil
}

func (r PostgresSyncRepository) GetSettlementsBy(ctx context.Context, tx *sqlx.Tx, did, fromPeerDID string) ([]db.Settlement, error) {
	query := `
        SELECT did, from_peer_did, to_peer_did, contract_version, document_digest, settled_at, jades_signature, recorded_at, delivered_at
        FROM contract_settlements
        WHERE did = $1 AND from_peer_did = $2
        ORDER BY to_peer_did
    `
	var settlements []db.Settlement
	if err := tx.SelectContext(ctx, &settlements, query, did, fromPeerDID); err != nil {
		return nil, err
	}
	return settlements, nil
}

func (r PostgresSyncRepository) DeleteSettlementsBy(ctx context.Context, tx *sqlx.Tx, did, fromPeerDID string) error {
	statement := `
        DELETE FROM contract_settlements
        WHERE did = $1 AND from_peer_did = $2
    `
	_, err := tx.ExecContext(ctx, statement, did, fromPeerDID)
	return err
}

func (r PostgresSyncRepository) GetUndeliveredSettlements(ctx context.Context, tx *sqlx.Tx, fromPeerDID string) ([]db.Settlement, error) {
	query := `
        SELECT did, from_peer_did, to_peer_did, contract_version, document_digest, settled_at, jades_signature, recorded_at, delivered_at
        FROM contract_settlements
        WHERE from_peer_did = $1 AND delivered_at IS NULL
        ORDER BY recorded_at
    `
	var settlements []db.Settlement
	if err := tx.SelectContext(ctx, &settlements, query, fromPeerDID); err != nil {
		return nil, err
	}
	return settlements, nil
}

func (r PostgresSyncRepository) MarkSettlementDelivered(ctx context.Context, tx *sqlx.Tx, did, fromPeerDID, toPeerDID string) error {
	statement := `
        UPDATE contract_settlements
        SET delivered_at = CURRENT_TIMESTAMP
        WHERE did = $1 AND from_peer_did = $2 AND to_peer_did = $3
    `
	_, err := tx.ExecContext(ctx, statement, did, fromPeerDID, toPeerDID)
	return err
}

// UpsertSettlementWithdrawal replaces an undelivered earlier withdrawal toward
// the same audience: it named an earlier settlement, and the settlement that
// has to go now is the one this row names.
func (r PostgresSyncRepository) UpsertSettlementWithdrawal(ctx context.Context, tx *sqlx.Tx, w db.SettlementWithdrawal) error {
	statement := `
        INSERT INTO contract_settlement_withdrawals (did, from_peer_did, to_peer_did, document_digest, withdrawn_at, recorded_at, delivered_at)
        VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP, NULL)
        ON CONFLICT (did, from_peer_did, to_peer_did) DO UPDATE SET
            document_digest = EXCLUDED.document_digest,
            withdrawn_at    = EXCLUDED.withdrawn_at,
            recorded_at     = CURRENT_TIMESTAMP,
            delivered_at    = NULL
    `
	_, err := tx.ExecContext(ctx, statement, w.DID, w.FromPeerDID, w.ToPeerDID, w.DocumentDigest, w.WithdrawnAt.UTC())
	return err
}

func (r PostgresSyncRepository) GetUndeliveredSettlementWithdrawals(ctx context.Context, tx *sqlx.Tx, fromPeerDID string) ([]db.SettlementWithdrawal, error) {
	query := `
        SELECT did, from_peer_did, to_peer_did, document_digest, withdrawn_at, recorded_at, delivered_at
        FROM contract_settlement_withdrawals
        WHERE from_peer_did = $1 AND delivered_at IS NULL
        ORDER BY recorded_at
    `
	var withdrawals []db.SettlementWithdrawal
	if err := tx.SelectContext(ctx, &withdrawals, query, fromPeerDID); err != nil {
		return nil, err
	}
	return withdrawals, nil
}

func (r PostgresSyncRepository) MarkSettlementWithdrawalDelivered(ctx context.Context, tx *sqlx.Tx, did, fromPeerDID, toPeerDID string) error {
	statement := `
        UPDATE contract_settlement_withdrawals
        SET delivered_at = CURRENT_TIMESTAMP
        WHERE did = $1 AND from_peer_did = $2 AND to_peer_did = $3
    `
	_, err := tx.ExecContext(ctx, statement, did, fromPeerDID, toPeerDID)
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
