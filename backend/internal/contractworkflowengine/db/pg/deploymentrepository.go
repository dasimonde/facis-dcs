package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/contractworkflowengine/db"
)

// PostgresDeploymentRepo persists contract deployments and KPI reports
// (contract_deployments, contract_kpis).
type PostgresDeploymentRepo struct {
}

func (r *PostgresDeploymentRepo) CreateDeployment(ctx context.Context, tx *sqlx.Tx, data db.ContractDeployment) error {
	statement := `
        INSERT INTO contract_deployments (
            did, contract_version, correlation_id, content_hash, target_id, target_url, status, requested_by, requested_at
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
    `
	_, err := tx.ExecContext(ctx, statement,
		data.DID, data.ContractVersion, data.CorrelationID, data.ContentHash, data.TargetID, data.TargetURL, data.Status, data.RequestedBy, data.RequestedAt)
	return err
}

func (r *PostgresDeploymentRepo) FindDeploymentByCorrelationID(ctx context.Context, tx *sqlx.Tx, correlationID string) (*db.ContractDeployment, error) {
	query := `
        SELECT id, did, contract_version, correlation_id, content_hash, target_id, target_url, status, requested_by, requested_at,
               acknowledged_at, receipt_hash, tsa_token, dispatch_error
        FROM contract_deployments
        WHERE correlation_id = $1
    `
	var deployment db.ContractDeployment
	err := tx.GetContext(ctx, &deployment, query, correlationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("read contract deployment %s: %w", correlationID, err)
	}
	return &deployment, nil
}

func (r *PostgresDeploymentRepo) AcknowledgeDeployment(ctx context.Context, tx *sqlx.Tx, correlationID string, receiptHash string, tsaToken string, acknowledgedAt time.Time) error {
	statement := `
        UPDATE contract_deployments
        SET status = 'ACKNOWLEDGED', acknowledged_at = $2, receipt_hash = $3, tsa_token = $4
        WHERE correlation_id = $1
    `
	_, err := tx.ExecContext(ctx, statement, correlationID, acknowledgedAt, receiptHash, tsaToken)
	return err
}

// MarkDispatchFailed records that the outbound call never reached the target.
// Only rows still claiming DISPATCHED are touched, so a target that
// acknowledged before this ran is not retroactively marked failed.
func (r *PostgresDeploymentRepo) MarkDispatchFailed(ctx context.Context, tx *sqlx.Tx, correlationID string, reason string) error {
	statement := `
        UPDATE contract_deployments
        SET status = 'DISPATCH_FAILED', dispatch_error = $2
        WHERE correlation_id = $1 AND status = 'DISPATCHED'
    `
	_, err := tx.ExecContext(ctx, statement, correlationID, reason)
	return err
}

func (r *PostgresDeploymentRepo) CreateKPI(ctx context.Context, tx *sqlx.Tx, data db.ContractKPI) error {
	statement := `
        INSERT INTO contract_kpis (did, correlation_id, metric, value, observed_at, verdict, rule_id)
        VALUES ($1, $2, $3, $4, $5, $6, $7)
    `
	_, err := tx.ExecContext(ctx, statement, data.DID, data.CorrelationID, data.Metric, data.Value, data.ObservedAt, data.Verdict, data.RuleID)
	return err
}

func (r *PostgresDeploymentRepo) ReadKPIsByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]db.ContractKPI, error) {
	query := `
        SELECT id, did, correlation_id, metric, value, observed_at, verdict, rule_id
        FROM contract_kpis
        WHERE did = $1
        ORDER BY observed_at ASC, id ASC
    `
	var kpis []db.ContractKPI
	err := tx.SelectContext(ctx, &kpis, query, did)
	if err != nil {
		return nil, err
	}
	return kpis, nil
}
