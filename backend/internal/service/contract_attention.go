package service

import (
	"context"
	"time"

	cwe "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/contractreadpolicy"
)

// Attention lists the contracts the caller may read that need attention
// (DCS-FR-CSA-23): the notice period before expiry has started, or the
// contract expired or was terminated in the last 30 days. It keeps no state.
func (s *contractWorkflowEnginesrvc) Attention(ctx context.Context, _ *cwe.ContractAttentionRequest) ([]*cwe.ContractAttentionItem, error) {
	ctx, err := contractReadContext(ctx, s.DIDDocument)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	var params []any
	readable := contractreadpolicy.SQL(ctx, "c.did", &params)
	var rows []struct {
		DID          string     `db:"did"`
		Name         string     `db:"name"`
		State        string     `db:"state"`
		Reason       string     `db:"reason"`
		ExpDate      *time.Time `db:"exp_date"`
		TerminatedAt *time.Time `db:"terminated_at"`
	}
	err = s.DB.SelectContext(ctx, &rows, `
		SELECT c.did, COALESCE(c.name, '') AS name, c.state::text AS state, c.exp_date, t.effective_at AS terminated_at,
		       CASE c.state WHEN 'TERMINATED' THEN 'terminated' WHEN 'EXPIRED' THEN 'expired' ELSE 'renewal_due' END AS reason
		FROM contracts_effective c LEFT JOIN contract_terminations t ON t.did = c.did
		WHERE `+readable+` AND (
		    (c.state = 'TERMINATED' AND t.effective_at > CURRENT_TIMESTAMP - INTERVAL '30 days')
		 OR (c.state = 'EXPIRED' AND c.exp_date > CURRENT_TIMESTAMP - INTERVAL '30 days')
		 OR (c.state NOT IN ('TERMINATED', 'REJECTED', 'EXPIRED', 'WITHDRAWN', 'REVOKED')
		     AND c.exp_date - make_interval(days => COALESCE(c.exp_notice_period, 0)) <= CURRENT_TIMESTAMP))
		ORDER BY COALESCE(t.effective_at, c.exp_date), c.did`, params...)
	if err != nil {
		return nil, cwe.MakeInternalError(err)
	}

	items := make([]*cwe.ContractAttentionItem, 0, len(rows))
	for _, row := range rows {
		item := &cwe.ContractAttentionItem{Did: row.DID, State: row.State, Reason: row.Reason}
		if row.Name != "" {
			item.Name = &row.Name
		}
		if row.ExpDate != nil {
			exp := row.ExpDate.UTC().Format(time.RFC3339)
			item.ExpDate = &exp
		}
		if row.TerminatedAt != nil {
			terminated := row.TerminatedAt.UTC().Format(time.RFC3339)
			item.TerminatedAt = &terminated
		}
		items = append(items, item)
	}
	return items, nil
}
