// Package pg is the Postgres implementation of the process-audit-and-
// compliance repository interfaces.
package pg

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/processauditandcompliance/db"
)

type PostgresRiskFindingRepo struct{}

func (r *PostgresRiskFindingRepo) ListOpen(ctx context.Context, tx *sqlx.Tx) ([]db.RiskFinding, error) {
	const query = `
        SELECT contract_did, risk_type, detail_hash, detail,
               first_detected_at, last_seen_at, resolved_at
        FROM compliance_risk_findings
        WHERE resolved_at IS NULL
        ORDER BY contract_did, risk_type, detail_hash
    `
	findings := make([]db.RiskFinding, 0)
	if err := tx.SelectContext(ctx, &findings, query); err != nil {
		return nil, err
	}
	return findings, nil
}

// Record is written as three narrow statements rather than one clever upsert so
// that "is this a new incident?" is decided by the database, not by comparing
// timestamps in Go: two sweeps may run at once (the scheduled one and a manual
// GET /pac/monitor), and only the one whose INSERT wins the primary key — or
// whose UPDATE finds the row still resolved — may raise the alert.
func (r *PostgresRiskFindingRepo) Record(ctx context.Context, tx *sqlx.Tx, finding db.RiskFinding) (bool, error) {

	const insert = `
        INSERT INTO compliance_risk_findings (
            contract_did, risk_type, detail_hash, detail, first_detected_at, last_seen_at
        ) VALUES ($1, $2, $3, $4, $5, $5)
        ON CONFLICT (contract_did, risk_type, detail_hash) DO NOTHING
    `
	inserted, err := tx.ExecContext(ctx, insert,
		finding.ContractDID, finding.RiskType, finding.DetailHash, finding.Detail, finding.FirstDetectedAt)
	if err != nil {
		return false, err
	}
	if rows, err := inserted.RowsAffected(); err != nil {
		return false, err
	} else if rows == 1 {
		return true, nil
	}

	// The finding exists. If it was resolved, this is a recurrence: a new
	// incident, whose detection time starts over (see the migration's note on
	// first_detected_at and MTTD).
	const reopen = `
        UPDATE compliance_risk_findings
        SET first_detected_at = $4, last_seen_at = $4, resolved_at = NULL
        WHERE contract_did = $1 AND risk_type = $2 AND detail_hash = $3
          AND resolved_at IS NOT NULL
    `
	reopened, err := tx.ExecContext(ctx, reopen,
		finding.ContractDID, finding.RiskType, finding.DetailHash, finding.FirstDetectedAt)
	if err != nil {
		return false, err
	}
	if rows, err := reopened.RowsAffected(); err != nil {
		return false, err
	} else if rows == 1 {
		return true, nil
	}

	// The risk was already open and still holds — record that it was seen, and
	// stay silent.
	const touch = `
        UPDATE compliance_risk_findings
        SET last_seen_at = $4
        WHERE contract_did = $1 AND risk_type = $2 AND detail_hash = $3
    `
	if _, err := tx.ExecContext(ctx, touch,
		finding.ContractDID, finding.RiskType, finding.DetailHash, finding.FirstDetectedAt); err != nil {
		return false, err
	}
	return false, nil
}

func (r *PostgresRiskFindingRepo) Resolve(ctx context.Context, tx *sqlx.Tx, finding db.RiskFinding, resolvedAt time.Time) error {
	const statement = `
        UPDATE compliance_risk_findings
        SET resolved_at = $4
        WHERE contract_did = $1 AND risk_type = $2 AND detail_hash = $3
          AND resolved_at IS NULL
    `
	_, err := tx.ExecContext(ctx, statement,
		finding.ContractDID, finding.RiskType, finding.DetailHash, resolvedAt)
	return err
}
