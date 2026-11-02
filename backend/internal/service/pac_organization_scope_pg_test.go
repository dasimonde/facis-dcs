package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	pac "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/pgtest"
	cwepg "digital-contracting-service/internal/contractworkflowengine/db/pg"
	"digital-contracting-service/internal/middleware"
	pacpg "digital-contracting-service/internal/processauditandcompliance/db/pg"
	"digital-contracting-service/internal/processauditandcompliance/workflowgate"
)

const reviewRunID = "11111111-1111-1111-1111-111111111111"

// pacOrganizationFixture seeds a contract of Org Y with an open approval task
// (a MISSING_APPROVAL risk) and a pending REVIEW gate run.
func pacOrganizationFixture(t *testing.T) (*processAuditAndCompliancesrvc, *sqlx.DB) {
	t.Helper()
	database := pgtest.Open(t)
	peer := "did:web:local"
	_, err := database.Exec(`INSERT INTO contracts (did,origin,created_by,state,template_did,template_version,name,contract_data)
		VALUES ('y-contract',$1,'Org Y','SUBMITTED','template',1,'y','{}')`, peer)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO contract_approval_task (did,state,approver,created_by) VALUES ('y-contract','OPEN','approver-y','Org Y')`)
	require.NoError(t, err)
	_, err = database.Exec(`INSERT INTO pac_workflow_gate_runs (run_id,correlation_id,snapshot_id,contract_did,contract_version,contract_state,contract_updated_at,gate,status,content_hash,snapshot_json,effective_shapes,profile_version,local_evaluation)
		VALUES ($1,'22222222-2222-2222-2222-222222222222','snap','y-contract',1,'SUBMITTED',now(),'approval','REVIEW','sha256:x','{}','[]',1,
		'{"result":"REVIEW","findings":[{"ruleId":"regulatory.gdpr.pid","severity":"warning","message":"the document carries personal data"}]}')`, reviewRunID)
	require.NoError(t, err)
	return &processAuditAndCompliancesrvc{
		DB:           database,
		ATrailReader: base.AuditTrailReader{Artifacts: artifactstore.New(nil, nil, nil, peer, "", nil)},
		CRepo:        &cwepg.PostgresContractRepo{},
		ATRepo:       &cwepg.PostgresApprovalTaskRepo{},
		FRepo:        &pacpg.PostgresRiskFindingRepo{},
		WorkflowGate: &workflowgate.Coordinator{DB: database},
	}, database
}

func complianceOfficer(organization string) context.Context {
	return middleware.InjectAuthContext(context.Background(), []string{"Compliance Officer"}, "holder", organization)
}

// DCS-FR-PACM-02/-03/-05/-07, ADR-31: a Compliance Officer of Org X neither
// sees Org Y's risks and gate runs nor files, exports, decides or reports on
// Org Y's contract.
func TestComplianceOfficerIsLimitedToTheContractsOfItsOrganization(t *testing.T) {
	s, database := pacOrganizationFixture(t)
	x := complianceOfficer("Org X")

	monitor, err := s.Monitor(x, &pac.MonitorPayload{})
	require.NoError(t, err)
	for _, risk := range monitor.Risks {
		require.NotEqual(t, "y-contract", risk.Did, "monitor shows Org Y's contract to Org X")
	}
	runs, err := s.WorkflowGateRuns(x, &pac.WorkflowGateRunsPayload{})
	require.NoError(t, err)
	require.Empty(t, runs)

	y := "y-contract"
	_, err = s.IncidentReport(x, &pac.IncidentReportPayload{ContractDid: &y, Findings: []*pac.PACIncidentFinding{{RiskType: "X", Detail: "x"}}})
	require.Equal(t, "forbidden", errorName(err), "incident on a foreign contract")
	_, err = s.WorkflowGateReview(x, &pac.WorkflowGateReviewPayload{RunID: reviewRunID, Decision: "reject", Justification: "x"})
	require.Equal(t, "forbidden", errorName(err), "foreign review decision")
	var decisions int
	require.NoError(t, database.Get(&decisions, `SELECT count(*) FROM pac_workflow_gate_review_decisions`))
	require.Zero(t, decisions)
	_, err = s.PartyReport(x, &pac.PartyReportPayload{Did: y})
	require.Equal(t, "forbidden", errorName(err), "foreign party report")

	// Org Y's own Compliance Officer files the case; Org X can neither list
	// nor export it.
	filed, err := s.IncidentReport(complianceOfficer("Org Y"), &pac.IncidentReportPayload{ContractDid: &y, Findings: []*pac.PACIncidentFinding{{RiskType: "X", Detail: "x"}}})
	require.NoError(t, err)
	caseID := filed.(map[string]any)["case_id"].(string)
	cases, err := s.Cases(x, &pac.CasesPayload{})
	require.NoError(t, err)
	require.Empty(t, cases)
	_, err = s.CaseFile(x, &pac.CaseFilePayload{CaseID: caseID, Justification: "x"})
	require.Equal(t, "forbidden", errorName(err), "foreign case file")
	var exports int
	require.NoError(t, database.Get(&exports, `SELECT count(*) FROM outbox_events WHERE event_type = 'PAC_CASE_FILE_EXPORTED'`))
	require.Zero(t, exports)
}

// DCS-FR-PACM-03, DCS-IR-PACM-04: the Compliance Officer of the contract's
// organization finds the REVIEW run with its findings and decides it, and
// finds and exports its cases.
func TestComplianceOfficerReviewsGateRunsAndCasesOfItsOrganization(t *testing.T) {
	s, _ := pacOrganizationFixture(t)
	y := complianceOfficer("Org Y")

	monitor, err := s.Monitor(y, &pac.MonitorPayload{})
	require.NoError(t, err)
	require.NotEmpty(t, monitor.Risks)

	runs, err := s.WorkflowGateRuns(y, &pac.WorkflowGateRunsPayload{})
	require.NoError(t, err)
	require.Len(t, runs, 1)
	require.Equal(t, reviewRunID, runs[0].RunID)
	require.Equal(t, []string{"regulatory.gdpr.pid: the document carries personal data"}, runs[0].Findings)
	decision, err := s.WorkflowGateReview(y, &pac.WorkflowGateReviewPayload{RunID: reviewRunID, Decision: "reject", Justification: "checked"})
	require.NoError(t, err)
	require.Equal(t, "Org Y", decision.DecidedBy)
	runs, err = s.WorkflowGateRuns(y, &pac.WorkflowGateRunsPayload{})
	require.NoError(t, err)
	require.Empty(t, runs, "a decided run waits no longer")

	contract := "y-contract"
	_, err = s.IncidentReport(y, &pac.IncidentReportPayload{ContractDid: &contract})
	require.Equal(t, "bad_request", errorName(err), "a report without findings")

	_, err = s.DB.Exec(`INSERT INTO contract_templates (did,created_by,state,template_type,party_type,name) VALUES ('tpl','creator','REGISTERED','CONTRACT_TEMPLATE','SINGLE_PARTY','n')`)
	require.NoError(t, err)
	template, unknown := "tpl", "no-such-template"
	_, err = s.IncidentReport(y, &pac.IncidentReportPayload{TemplateDid: &unknown, Findings: []*pac.PACIncidentFinding{{RiskType: "X", Detail: "x"}}})
	require.Equal(t, "not_found", errorName(err), "an incident on an unknown template")
	filed, err := s.IncidentReport(y, &pac.IncidentReportPayload{TemplateDid: &template, Findings: []*pac.PACIncidentFinding{{RiskType: "X", Detail: "x"}}})
	require.NoError(t, err)
	caseID := filed.(map[string]any)["case_id"].(string)

	cases, err := s.Cases(complianceOfficer("Org X"), &pac.CasesPayload{})
	require.NoError(t, err, "template cases are not organization-scoped")
	require.Len(t, cases, 1)
	require.Equal(t, caseID, cases[0].CaseID)
	require.Equal(t, "template", cases[0].LinkedTo)
	require.Nil(t, cases[0].ExportedAt)
	content, err := s.CaseFile(y, &pac.CaseFilePayload{CaseID: caseID, Justification: "regulator"})
	require.NoError(t, err)
	var file struct{ DID string }
	require.NoError(t, json.Unmarshal(content, &file))
	require.Equal(t, "tpl", file.DID)
	cases, err = s.Cases(y, &pac.CasesPayload{})
	require.NoError(t, err)
	require.NotNil(t, cases[0].ExportedAt)
}

// DCS-FR-PACM-04: the identity of a denied actor is audit-log content; the
// monitor names it only to a request that gives a justification.
func TestMonitorNamesDeniedActorsOnlyWithAJustification(t *testing.T) {
	s, database := pacOrganizationFixture(t)
	_, err := database.Exec(`INSERT INTO outbox_events (component, event_type, event_data, did)
		VALUES ('CONTRACT_WORKFLOW_ENGINE', 'CONTRACT_ACCESS_DENIED', '{"retrieved_by":"Org Z"}', 'y-contract')`)
	require.NoError(t, err)
	y := complianceOfficer("Org Y")
	denial := func(p *pac.MonitorPayload) string {
		monitor, err := s.Monitor(y, p)
		require.NoError(t, err)
		for _, risk := range monitor.Risks {
			if risk.RiskType == "UNAUTHORIZED_ACCESS" {
				return risk.Detail
			}
		}
		t.Fatal("no unauthorized-access risk")
		return ""
	}
	require.NotContains(t, denial(&pac.MonitorPayload{}), "Org Z")
	justification := "investigation of the denied access"
	require.Contains(t, denial(&pac.MonitorPayload{Justification: &justification}), "Org Z")
}
