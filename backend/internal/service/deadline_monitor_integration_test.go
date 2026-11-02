package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"digital-contracting-service/internal/base/pgtest"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	pacpg "digital-contracting-service/internal/processauditandcompliance/db/pg"
	qry "digital-contracting-service/internal/processauditandcompliance/query"
)

// DCS-FR-CWE-09 (ADR-33): the scheduled sweep flags an obligation of a contract
// in force whose deadline passed without a report. A report received after the
// deadline does not clear the miss; one received by the deadline does.
func TestDeadlineMonitorFlagsUnreportedPassedDeadlinePostgres(t *testing.T) {
	database := pgtest.Open(t)
	_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,contract_data) VALUES ('active','','Org A','ACTIVE','tpl',1,
		'{"dcs:policies":{"odrl:obligation":[{"@id":"urn:uuid:report","odrl:constraint":[{"odrl:leftOperand":{"@id":"odrl:dateTime"},"odrl:operator":{"@id":"odrl:lt"},"odrl:rightOperand":"2026-01-01"}]}]}}')`)
	require.NoError(t, err)
	monitor := qry.ComplianceMonitor{DB: database, ATRepo: &cwepg.PostgresApprovalTaskRepo{}, CRepo: &cwepg.PostgresContractRepo{}, FRepo: &pacpg.PostgresRiskFindingRepo{}}
	deadlineRisks := func() int {
		result, err := monitor.Handle(context.Background(), qry.MonitorQry{MonitoredBy: qry.SystemMonitorPrincipal})
		require.NoError(t, err)
		count := 0
		for _, risk := range result.Risks {
			if risk.RiskType == qry.RiskTypeDeadlineMissed {
				count++
			}
		}
		return count
	}

	require.Equal(t, 1, deadlineRisks())

	_, err = database.Exec(`INSERT INTO contract_kpis (did,metric,value,verdict,rule_id) VALUES ('active','reported','yes','satisfied','urn:uuid:report')`)
	require.NoError(t, err)
	require.Equal(t, 1, deadlineRisks(), "a late report leaves the violation")

	_, err = database.Exec(`INSERT INTO contract_kpis (did,metric,value,verdict,rule_id,observed_at) VALUES ('active','reported','yes','satisfied','urn:uuid:report','2025-12-31T12:00:00')`)
	require.NoError(t, err)
	require.Zero(t, deadlineRisks())
}

// DCS-FR-SM-13: the sweep flags a contract on its way to signature whose
// signing deadline passed, and leaves a signed one alone.
func TestMonitorFlagsPassedSigningDeadlinePostgres(t *testing.T) {
	database := pgtest.Open(t)
	for did, row := range map[string][2]string{
		"late-approved":   {"APPROVED", "2026-01-01T00:00:00Z"},
		"late-signed":     {"SIGNED", "2026-01-01T00:00:00Z"},
		"future-approved": {"APPROVED", "2999-01-01T00:00:00Z"},
	} {
		_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,contract_data)
			VALUES ($1,'','Org A',$2,'tpl',1,jsonb_build_object('dcs:signingDeadline', $3::text))`, did, row[0], row[1])
		require.NoError(t, err)
	}
	monitor := qry.ComplianceMonitor{DB: database, ATRepo: &cwepg.PostgresApprovalTaskRepo{}, CRepo: &cwepg.PostgresContractRepo{}, FRepo: &pacpg.PostgresRiskFindingRepo{}}
	result, err := monitor.Handle(context.Background(), qry.MonitorQry{MonitoredBy: qry.SystemMonitorPrincipal})
	require.NoError(t, err)
	var flagged []string
	for _, risk := range result.Risks {
		if risk.RiskType == qry.RiskTypeSigningDeadlineMissed {
			flagged = append(flagged, risk.DID)
		}
	}
	require.Equal(t, []string{"late-approved"}, flagged)
}
