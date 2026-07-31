package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"digital-contracting-service/internal/contractworkflowengine/db"

	"github.com/jmoiron/sqlx"
)

// PostgresContractTargetRepo stores the registry of Contract Target Systems
// deployments may be dispatched to (ADR-25).
type PostgresContractTargetRepo struct{}

func (r *PostgresContractTargetRepo) ListTargets(ctx context.Context, tx *sqlx.Tx) ([]db.ContractTarget, error) {
	const query = `
        SELECT id, name, url, description, enabled, oauth_client_id, secret_issued_at, created_by, created_at, updated_at
        FROM contract_targets
        ORDER BY name ASC
    `
	targets := []db.ContractTarget{}
	if err := tx.SelectContext(ctx, &targets, query); err != nil {
		return nil, fmt.Errorf("list contract targets: %w", err)
	}
	return targets, nil
}

func (r *PostgresContractTargetRepo) ReadTarget(ctx context.Context, tx *sqlx.Tx, id string) (*db.ContractTarget, error) {
	const query = `
        SELECT id, name, url, description, enabled, oauth_client_id, secret_issued_at, created_by, created_at, updated_at
        FROM contract_targets
        WHERE id = $1
    `
	var target db.ContractTarget
	if err := tx.GetContext(ctx, &target, query, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("read contract target %s: %w", id, err)
	}
	return &target, nil
}

func (r *PostgresContractTargetRepo) CreateTarget(ctx context.Context, tx *sqlx.Tx, data db.ContractTarget) (*db.ContractTarget, error) {
	const statement = `
        INSERT INTO contract_targets (name, url, description, enabled, created_by, oauth_client_id)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id, name, url, description, enabled, oauth_client_id, secret_issued_at, created_by, created_at, updated_at
    `
	var created db.ContractTarget
	if err := tx.GetContext(ctx, &created, statement, data.Name, data.URL, data.Description, data.Enabled, data.CreatedBy, data.OAuthClientID); err != nil {
		return nil, fmt.Errorf("create contract target %q: %w", data.Name, err)
	}
	return &created, nil
}

func (r *PostgresContractTargetRepo) UpdateTarget(ctx context.Context, tx *sqlx.Tx, data db.ContractTarget) (*db.ContractTarget, error) {
	const statement = `
        UPDATE contract_targets
        SET name = $2, url = $3, description = $4, enabled = $5, updated_at = CURRENT_TIMESTAMP
        WHERE id = $1
        RETURNING id, name, url, description, enabled, oauth_client_id, secret_issued_at, created_by, created_at, updated_at
    `
	var updated db.ContractTarget
	if err := tx.GetContext(ctx, &updated, statement, data.ID, data.Name, data.URL, data.Description, data.Enabled); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("update contract target %s: %w", data.ID, err)
	}
	return &updated, nil
}

func (r *PostgresContractTargetRepo) DeleteTarget(ctx context.Context, tx *sqlx.Tx, id string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM contract_targets WHERE id = $1`, id); err != nil {
		return fmt.Errorf("delete contract target %s: %w", id, err)
	}
	return nil
}

func (r *PostgresContractTargetRepo) CountContractsDesignating(ctx context.Context, tx *sqlx.Tx, id string) (int, error) {
	var count int
	if err := tx.GetContext(ctx, &count, `SELECT COUNT(*) FROM contracts WHERE target_id = $1`, id); err != nil {
		return 0, fmt.Errorf("count contracts designating target %s: %w", id, err)
	}
	return count, nil
}

func (r *PostgresContractTargetRepo) DesignateForContract(ctx context.Context, tx *sqlx.Tx, did string, targetID *string) (bool, error) {
	const statement = `
        UPDATE contracts
        SET target_id = $2, updated_at = CURRENT_TIMESTAMP
        WHERE did = $1
    `
	result, err := tx.ExecContext(ctx, statement, did, targetID)
	if err != nil {
		return false, fmt.Errorf("designate target for contract %s: %w", did, err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("designate target for contract %s: %w", did, err)
	}
	return affected > 0, nil
}

func (r *PostgresContractTargetRepo) SetCredential(ctx context.Context, tx *sqlx.Tx, id string, oauthClientID string, issuedAt time.Time) error {
	const statement = `
        UPDATE contract_targets
        SET oauth_client_id = $2, secret_issued_at = $3, updated_at = CURRENT_TIMESTAMP
        WHERE id = $1
    `
	if _, err := tx.ExecContext(ctx, statement, id, oauthClientID, issuedAt); err != nil {
		return fmt.Errorf("record the callback credential for contract target %s: %w", id, err)
	}
	return nil
}
