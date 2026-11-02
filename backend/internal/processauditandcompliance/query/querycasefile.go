package qry

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	event2 "digital-contracting-service/internal/processauditandcompliance/event"
)

// ErrCaseNotFound is returned when no incident report carries the case ID.
var ErrCaseNotFound = errors.New("no incident report has this case ID")

// CaseFile is the export of one incident case for regulatory review
// (DCS-FR-PACM-05): the reported findings, and the compliance risks, the
// workflow gate runs and the event timeline of the linked contract.
type CaseFile struct {
	CaseID           string        `json:"case_id"`
	DID              string        `json:"did"`
	GeneratedAt      time.Time     `json:"generated_at"`
	GeneratedBy      string        `json:"generated_by"`
	Findings         []CaseFinding `json:"findings"`
	ComplianceRisks  []CaseRisk    `json:"compliance_risks"`
	WorkflowGateRuns []CaseGateRun `json:"workflow_gate_runs"`
	Timeline         []CaseEvent   `json:"timeline"`
}

type CaseFinding struct {
	RiskType   string    `json:"risk_type" db:"risk_type"`
	Detail     string    `json:"detail" db:"detail"`
	ReportedBy string    `json:"reported_by" db:"reported_by"`
	ReportedAt time.Time `json:"reported_at" db:"reported_at"`
	DID        string    `json:"-" db:"did"`
}

type CaseRisk struct {
	RiskType        string     `json:"risk_type" db:"risk_type"`
	Detail          string     `json:"detail" db:"detail"`
	FirstDetectedAt time.Time  `json:"first_detected_at" db:"first_detected_at"`
	LastSeenAt      time.Time  `json:"last_seen_at" db:"last_seen_at"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty" db:"resolved_at"`
}

type CaseGateRun struct {
	RunID           string          `json:"run_id" db:"run_id"`
	Gate            string          `json:"gate" db:"gate"`
	Status          string          `json:"status" db:"status"`
	ContractVersion int             `json:"contract_version" db:"contract_version"`
	CreatedAt       time.Time       `json:"created_at" db:"created_at"`
	FailureReason   *string         `json:"failure_reason,omitempty" db:"failure_reason"`
	LocalEvaluation json.RawMessage `json:"local_evaluation" db:"local_evaluation"`
}

type CaseEvent struct {
	Component string    `json:"component" db:"component"`
	EventType string    `json:"event_type" db:"event_type"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// CaseSummary is one incident case in the case list.
type CaseSummary struct {
	CaseID     string     `db:"case_id"`
	DID        string     `db:"did"`
	LinkedTo   string     `db:"linked_to"`
	ReportedBy string     `db:"reported_by"`
	ReportedAt time.Time  `db:"reported_at"`
	Findings   int        `db:"findings"`
	ExportedAt *time.Time `db:"exported_at"`
}

// Cases lists every incident case, newest first.
func Cases(ctx context.Context, db sqlx.QueryerContext) ([]CaseSummary, error) {
	var cases []CaseSummary
	if err := sqlx.SelectContext(ctx, db, &cases, `
        WITH reported AS (
            SELECT event_data->>'case_id' AS case_id, COALESCE(min(did), '') AS did,
                   COALESCE(min(event_data->>'reported_by'), '') AS reported_by, min(created_at) AS reported_at,
                   count(*) AS findings
            FROM outbox_events WHERE event_type = 'PAC_INCIDENT_REPORT'
            GROUP BY event_data->>'case_id')
        SELECT r.case_id, r.did, r.reported_by, r.reported_at, r.findings,
               CASE WHEN EXISTS (SELECT 1 FROM contracts c WHERE c.did = r.did) THEN 'contract' ELSE 'template' END AS linked_to,
               (SELECT max(x.created_at) FROM outbox_events x
                 WHERE x.event_type = 'PAC_CASE_FILE_EXPORTED' AND x.event_data->>'case_id' = r.case_id) AS exported_at
        FROM reported r
        ORDER BY r.reported_at DESC, r.case_id`); err != nil {
		return nil, fmt.Errorf("could not read incident cases: %w", err)
	}
	return cases, nil
}

type CaseFileQry struct {
	CaseID        string
	ExportedBy    string
	Justification string
	HolderDID     string
	UserRoles     userrole.UserRoles
}

type CaseFileExporter struct {
	DB *sqlx.DB
	// Authorize refuses a caller who may not read the DID the case is linked
	// to. Its error is returned unchanged.
	Authorize func(ctx context.Context, did string) error
}

// Handle assembles the case file and records its export with the
// justification in the audit trail. It returns the case file as JSON.
func (h *CaseFileExporter) Handle(ctx context.Context, query CaseFileQry) ([]byte, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	file := CaseFile{CaseID: query.CaseID, GeneratedAt: time.Now().UTC(), GeneratedBy: query.ExportedBy}
	if err := tx.SelectContext(ctx, &file.Findings, `
        SELECT COALESCE(event_data->>'risk_type', '') AS risk_type,
               COALESCE(event_data->>'detail', '') AS detail,
               COALESCE(event_data->>'reported_by', '') AS reported_by,
               created_at AS reported_at, COALESCE(did, '') AS did
        FROM outbox_events
        WHERE event_type = 'PAC_INCIDENT_REPORT' AND event_data->>'case_id' = $1
        ORDER BY id`, query.CaseID); err != nil {
		return nil, fmt.Errorf("could not read case findings: %w", err)
	}
	if len(file.Findings) == 0 {
		return nil, ErrCaseNotFound
	}
	file.DID = file.Findings[0].DID
	if err := h.Authorize(ctx, file.DID); err != nil {
		return nil, err
	}
	if err := tx.SelectContext(ctx, &file.ComplianceRisks, `
        SELECT risk_type, detail, first_detected_at, last_seen_at, resolved_at
        FROM compliance_risk_findings WHERE contract_did = $1
        ORDER BY first_detected_at`, file.DID); err != nil {
		return nil, fmt.Errorf("could not read compliance risks: %w", err)
	}
	if err := tx.SelectContext(ctx, &file.WorkflowGateRuns, `
        SELECT run_id::text AS run_id, gate, status, contract_version, created_at, failure_reason, local_evaluation
        FROM pac_workflow_gate_runs WHERE contract_did = $1
        ORDER BY created_at`, file.DID); err != nil {
		return nil, fmt.Errorf("could not read workflow gate runs: %w", err)
	}
	if err := tx.SelectContext(ctx, &file.Timeline, `
        SELECT component, event_type, created_at FROM outbox_events
        WHERE did = $1 ORDER BY id`, file.DID); err != nil {
		return nil, fmt.Errorf("could not read event timeline: %w", err)
	}

	content, err := json.Marshal(file)
	if err != nil {
		return nil, fmt.Errorf("could not encode case file: %w", err)
	}
	sum := sha256.Sum256(content)
	if err := event.Create(ctx, tx, event2.CaseFileExportedEvent{
		CaseID: query.CaseID, DID: file.DID, ExportedBy: query.ExportedBy, Justification: query.Justification,
		ContentHash: "sha256:" + hex.EncodeToString(sum[:]), OccurredAt: file.GeneratedAt,
		HolderDID: query.HolderDID, UserRoles: query.UserRoles,
	}, componenttype.ProcessAuditAndCompliance); err != nil {
		return nil, fmt.Errorf("could not record case file export: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}
	return content, nil
}
