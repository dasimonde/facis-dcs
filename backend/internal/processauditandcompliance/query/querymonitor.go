package qry

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/approvaltaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	pacdb "digital-contracting-service/internal/processauditandcompliance/db"
	event2 "digital-contracting-service/internal/processauditandcompliance/event"
)

// RiskTypeMissingApproval flags a contract sitting in an approval-pending
// state while at least one required approval task is still OPEN
// (DCS-FR-PACM-03: contracts failing compliance checks are flagged for
// review; the missing approval is the policy violation being monitored).
const RiskTypeMissingApproval = "MISSING_APPROVAL"

// RiskTypeUnauthorizedAccess flags a contract carrying a persisted
// access-denial audit artifact (CONTRACT_ACCESS_DENIED, committed by the
// party read-scoping denial branch in query/contract/querybyid.go before the
// 403 is returned): the denied attempt is the unauthorized-access violation
// being monitored (DCS-FR-PACM-02).
const RiskTypeUnauthorizedAccess = "UNAUTHORIZED_ACCESS"

// RiskTypeUnderperformance flags a contract already in force whose own ODRL
// policies are no longer satisfied by the values reported against it — a
// missed delivery target, a breached service level, a financial term not met
// (DCS-FR-CWE-31: "Alerts MUST be raised for underperformance or missed
// targets"). Unlike the approval-time gate, which REFUSES to approve a
// contract whose terms are already violated, this is monitoring of a contract
// in force: the violation is reported, not blocked, because the contract
// exists and the breach is a fact to be recorded and acted on.
const RiskTypeUnderperformance = "CONTRACT_UNDERPERFORMANCE"

// RiskTypeDeploymentFailed flags a deployment the Contract Target System never
// received (DCS-FR-CWE-31, "alerts for underperformance or missed targets").
// The contract is in force on this side and absent on the other, and until this
// existed the only trace was a line in the process log.
const RiskTypeDeploymentFailed = "CONTRACT_DEPLOYMENT_FAILED"

// SystemMonitorPrincipal attributes an unattended sweep (cmd/pacmonitor) in the
// audit trail. Such a run carries no user roles on purpose: no operator
// authorised it, the schedule did, and stamping a human role onto it would let
// the trail claim someone was present who was not.
const SystemMonitorPrincipal = "system:pac-monitor"

// approvalPendingStates are the contract states in which an OPEN approval
// task means the contract is waiting on a required approval decision.
var approvalPendingStates = map[contractstate.ContractState]bool{
	contractstate.Submitted: true,
	contractstate.Reviewed:  true,
}

// accessDenial is one persisted CONTRACT_ACCESS_DENIED artifact read back
// from the outbox audit rows; RetrievedBy is the denied actor (the
// OID4VP-disclosed organization claim the read path persists).
type accessDenial struct {
	DID         string `db:"did"`
	RetrievedBy string `db:"retrieved_by"`
}

// unauthorizedAccessRisks maps persisted access denials to compliance risks,
// one per distinct (contract, actor) pair, mirroring the MISSING_APPROVAL
// per-(contract, approver) dedup.
func unauthorizedAccessRisks(denials []accessDenial, checkedAt time.Time) []event2.ComplianceRisk {
	risks := make([]event2.ComplianceRisk, 0)
	seen := make(map[string]bool)
	for _, denial := range denials {
		if denial.DID == "" || seen[denial.DID+"|"+denial.RetrievedBy] {
			continue
		}
		seen[denial.DID+"|"+denial.RetrievedBy] = true
		risks = append(risks, event2.ComplianceRisk{
			DID:      denial.DID,
			RiskType: RiskTypeUnauthorizedAccess,
			Detail: fmt.Sprintf(
				"unauthorized access to contract %s was attempted by %s and denied",
				denial.DID, denial.RetrievedBy),
			DetectedAt: checkedAt,
		})
	}
	return risks
}

type MonitorQry struct {
	MonitoredBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type MonitorResult struct {
	CheckedAt time.Time
	Risks     []event2.ComplianceRisk
}

type ComplianceMonitor struct {
	DB     *sqlx.DB
	ATRepo cwedb.ApprovalTaskRepo
	CRepo  cwedb.ContractRepo
	FRepo  pacdb.RiskFindingRepo
}

// findingOf is the persistent identity of a detected risk: the contract, the
// rule that fired, and a hash of the detail text, which is what distinguishes
// two risks of the same type on the same contract (a second outstanding
// approver, a second denied actor). Detail is derived only from persisted
// facts, never from the sweep's own clock, so the same violation hashes the
// same on every sweep.
func findingOf(risk event2.ComplianceRisk, detectedAt time.Time) pacdb.RiskFinding {
	sum := sha256.Sum256([]byte(risk.Detail))
	return pacdb.RiskFinding{
		ContractDID:     risk.DID,
		RiskType:        risk.RiskType,
		DetailHash:      hex.EncodeToString(sum[:]),
		Detail:          risk.Detail,
		FirstDetectedAt: detectedAt,
	}
}

// reconcile folds the sweep's findings into the risk register and returns the
// ones that are NEW — first ever, or reoccurring after being resolved. Risks
// that were already open are recorded as still seen but raise no second alert,
// and open findings the sweep no longer detects are closed.
//
// This governs alerting only. The sweep response keeps listing every risk that
// currently holds, because it answers a different question: not "what happened"
// but "what is wrong right now".
func (h *ComplianceMonitor) reconcile(ctx context.Context, tx *sqlx.Tx, risks []event2.ComplianceRisk, checkedAt time.Time) ([]event2.ComplianceRisk, error) {

	open, err := h.FRepo.ListOpen(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("could not read open compliance findings: %w", err)
	}

	stillHolds := make(map[string]bool, len(risks))
	newlyDetected := make([]event2.ComplianceRisk, 0)
	for _, risk := range risks {
		finding := findingOf(risk, checkedAt)
		stillHolds[finding.ContractDID+"|"+finding.RiskType+"|"+finding.DetailHash] = true

		isNew, err := h.FRepo.Record(ctx, tx, finding)
		if err != nil {
			return nil, fmt.Errorf("could not record compliance finding for %s: %w", risk.DID, err)
		}
		if isNew {
			newlyDetected = append(newlyDetected, risk)
		}
	}

	for _, finding := range open {
		if stillHolds[finding.ContractDID+"|"+finding.RiskType+"|"+finding.DetailHash] {
			continue
		}
		if err := h.FRepo.Resolve(ctx, tx, finding, checkedAt); err != nil {
			return nil, fmt.Errorf("could not resolve compliance finding for %s: %w", finding.ContractDID, err)
		}
	}

	return newlyDetected, nil
}

// Handle sweeps all OPEN approval tasks and flags those whose contract is in
// an approval-pending state as MISSING_APPROVAL risks, then reads back the
// persisted access-denial artifacts and flags the affected contracts as
// UNAUTHORIZED_ACCESS risks. The sweep itself is recorded in the audit trail
// via ComplianceMonitorEvent, risks included, so a detected risk is both
// flagged (response) and reported (audit trail).
//
// Handle is driven both by GET /pac/monitor and, unattended, by the scheduled
// sweep (cmd/pacmonitor). The response always lists every risk that currently
// holds; alerting is deduplicated against the risk register, see reconcile.
func (h *ComplianceMonitor) Handle(ctx context.Context, query MonitorQry) (*MonitorResult, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	openTasks, err := h.ATRepo.ReadAllInState(ctx, tx, approvaltaskstate.Open.String())
	if err != nil {
		return nil, fmt.Errorf("could not read open approval tasks: %w", err)
	}

	checkedAt := time.Now().UTC()
	risks := make([]event2.ComplianceRisk, 0)
	seen := make(map[string]bool)
	for _, task := range openTasks {
		if seen[task.DID+"|"+task.Approver] {
			continue
		}
		seen[task.DID+"|"+task.Approver] = true

		data, err := h.CRepo.ReadDataByDID(ctx, tx, task.DID)
		if err != nil {
			return nil, fmt.Errorf("could not read contract %s for open approval task: %w", task.DID, err)
		}
		state, err := contractstate.NewContractState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not parse contract state for %s: %w", task.DID, err)
		}
		if !approvalPendingStates[state] {
			continue
		}
		risks = append(risks, event2.ComplianceRisk{
			DID:      task.DID,
			RiskType: RiskTypeMissingApproval,
			Detail: fmt.Sprintf(
				"contract in state %s is missing a required approval from %s",
				state, task.Approver),
			DetectedAt: checkedAt,
		})
	}

	perfRisks, err := h.underperformanceRisks(ctx, tx, checkedAt)
	if err != nil {
		return nil, err
	}
	risks = append(risks, perfRisks...)

	dispatchRisks, err := h.deploymentFailureRisks(ctx, tx, checkedAt)
	if err != nil {
		return nil, err
	}
	risks = append(risks, dispatchRisks...)

	denialQuery := `
        SELECT COALESCE(did, '') AS did,
               COALESCE(event_data ->> 'retrieved_by', '') AS retrieved_by
        FROM outbox_events
        WHERE event_type = $1
        ORDER BY id
    `
	var denials []accessDenial
	if err := tx.SelectContext(ctx, &denials, denialQuery, eventtype.AccessDenied.String()); err != nil {
		return nil, fmt.Errorf("could not read persisted access denials: %w", err)
	}
	risks = append(risks, unauthorizedAccessRisks(denials, checkedAt)...)

	evt := event2.ComplianceMonitorEvent{
		MonitoredBy: query.MonitoredBy,
		OccurredAt:  checkedAt,
		Risks:       risks,
		HolderDID:   query.HolderDID,
		UserRoles:   query.UserRoles,
	}
	if err := event.Create(ctx, tx, evt, componenttype.ProcessAuditAndCompliance); err != nil {
		return nil, fmt.Errorf("could not create monitor event: %w", err)
	}
	newlyDetected, err := h.reconcile(ctx, tx, risks, checkedAt)
	if err != nil {
		return nil, err
	}

	// Anchor each NEWLY detected risk against the affected contract's PAC
	// chain — the sweep event above has no resource DID and only reaches the
	// global chain (see ComplianceRiskEvent doc). Risks that were already on
	// record are not re-anchored: with the sweep running unattended every few
	// minutes, that would bury the audit trail under repetitions of the same
	// violation and make the moment it was actually detected unfindable.
	for _, risk := range newlyDetected {
		riskEvt := event2.ComplianceRiskEvent{
			DID:         risk.DID,
			RiskType:    risk.RiskType,
			Detail:      risk.Detail,
			MonitoredBy: query.MonitoredBy,
			OccurredAt:  checkedAt,
			HolderDID:   query.HolderDID,
			UserRoles:   query.UserRoles,
		}
		if err := event.Create(ctx, tx, riskEvt, componenttype.ProcessAuditAndCompliance); err != nil {
			return nil, fmt.Errorf("could not create compliance risk event for %s: %w", risk.DID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return &MonitorResult{CheckedAt: checkedAt, Risks: risks}, nil
}

// underperformanceRisks reads back the KPI values the contract TARGET reported
// over the deployment callback channel and flags every one already marked as
// violating its contract's obligations. It deliberately does not re-evaluate
// anything: callback.go evaluates each reported value against the contract's
// own ODRL when it arrives (validation.EvaluateKPIViolation) and persists the
// verdict, so the monitor reports the same judgement rather than forming a
// second, possibly divergent one.
//
// Reading reported values is the whole point: the inline values carried on the
// contract's fields cannot change once it is in force, and a contract already
// violating them could never have been approved — so evaluating those would
// produce an alert that never fires.
func (h *ComplianceMonitor) underperformanceRisks(ctx context.Context, tx *sqlx.Tx, checkedAt time.Time) ([]event2.ComplianceRisk, error) {
	const violatingKPIQuery = `
        SELECT COALESCE(did, '') AS did,
               COALESCE(metric, '') AS metric,
               COALESCE(value, '') AS value,
               observed_at
        FROM contract_kpis
        WHERE violation = true
        ORDER BY observed_at, id
    `
	var reported []violatingKPI
	if err := tx.SelectContext(ctx, &reported, violatingKPIQuery); err != nil {
		return nil, fmt.Errorf("could not read violating KPI reports: %w", err)
	}
	return underperformanceRisksFromKPIs(reported, checkedAt), nil
}

// deploymentFailureRisks reads back the dispatches already recorded as failed.
// Like the underperformance alert it reports a persisted outcome rather than
// re-deriving one: deploy.go marks the row when its outbound call returns an
// error, and this states that judgement.
func (h *ComplianceMonitor) deploymentFailureRisks(ctx context.Context, tx *sqlx.Tx, checkedAt time.Time) ([]event2.ComplianceRisk, error) {
	const failedDispatchQuery = `
        SELECT COALESCE(d.did, '') AS did,
               COALESCE(d.correlation_id, '') AS correlation_id,
               COALESCE(t.name, d.target_url) AS target,
               COALESCE(d.dispatch_error, '') AS reason,
               d.requested_at
        FROM contract_deployments d
        LEFT JOIN contract_targets t ON t.id = d.target_id
        WHERE d.status = 'DISPATCH_FAILED'
        ORDER BY d.requested_at, d.id
    `
	var failed []failedDispatch
	if err := tx.SelectContext(ctx, &failed, failedDispatchQuery); err != nil {
		return nil, fmt.Errorf("could not read failed deployment dispatches: %w", err)
	}
	return deploymentFailureRisksFrom(failed, checkedAt), nil
}

// failedDispatch is one persisted deployment whose outbound call never reached
// its target.
type failedDispatch struct {
	DID           string    `db:"did"`
	CorrelationID string    `db:"correlation_id"`
	TargetURL     *string   `db:"target"`
	Reason        string    `db:"reason"`
	RequestedAt   time.Time `db:"requested_at"`
}

// deploymentFailureRisksFrom maps failed dispatches to compliance risks. The
// detail names the target and the reason: an operator told only that a
// deployment failed knows neither which system to look at nor whether to retry.
func deploymentFailureRisksFrom(failed []failedDispatch, checkedAt time.Time) []event2.ComplianceRisk {
	risks := make([]event2.ComplianceRisk, 0, len(failed))
	for _, dispatch := range failed {
		if strings.TrimSpace(dispatch.DID) == "" {
			continue
		}
		target := "the configured target system"
		if dispatch.TargetURL != nil && strings.TrimSpace(*dispatch.TargetURL) != "" {
			target = *dispatch.TargetURL
		}
		risks = append(risks, event2.ComplianceRisk{
			DID:      dispatch.DID,
			RiskType: RiskTypeDeploymentFailed,
			Detail: fmt.Sprintf("deployment %s to %s was never received: %s (dispatched %s)",
				dispatch.CorrelationID, target, dispatch.Reason,
				dispatch.RequestedAt.UTC().Format(time.RFC3339)),
			DetectedAt: checkedAt,
		})
	}
	return risks
}

// violatingKPI is one persisted target-reported KPI already judged to breach
// its contract's obligations.
type violatingKPI struct {
	DID        string    `db:"did"`
	Metric     string    `db:"metric"`
	Value      string    `db:"value"`
	ObservedAt time.Time `db:"observed_at"`
}

// underperformanceRisksFromKPIs maps violating KPI reports to compliance risks.
// The detail names the metric and the value observed: an alert that only says
// "underperforming" tells an operator something is wrong without telling them
// what missed, or by how much.
func underperformanceRisksFromKPIs(reported []violatingKPI, checkedAt time.Time) []event2.ComplianceRisk {
	risks := make([]event2.ComplianceRisk, 0, len(reported))
	for _, kpi := range reported {
		if strings.TrimSpace(kpi.DID) == "" {
			continue
		}
		risks = append(risks, event2.ComplianceRisk{
			DID:      kpi.DID,
			RiskType: RiskTypeUnderperformance,
			Detail: fmt.Sprintf("reported KPI %q = %q violates the contract's agreed obligation (observed %s)",
				kpi.Metric, kpi.Value, kpi.ObservedAt.UTC().Format(time.RFC3339)),
			DetectedAt: checkedAt,
		})
	}
	return risks
}
