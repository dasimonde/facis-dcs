package qry

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	event2 "digital-contracting-service/internal/processauditandcompliance/event"
)

// IncidentFinding is one non-compliance finding submitted through
// POST /pac/report (DCS-IR-PACM-04), linked to the affected contract or
// template DID.
type IncidentFinding struct {
	RiskType string
	Detail   string
}

type IncidentReportQry struct {
	DID        string
	Findings   []IncidentFinding
	ReportedBy string
	HolderDID  string
	UserRoles  userrole.UserRoles
}

type IncidentReporter struct {
	DB *sqlx.DB
}

// Handle persists each submitted finding as a PAC_INCIDENT_REPORT event
// anchored against the linked contract/template DID (mirrors the
// ComplianceRiskEvent anchoring pattern in querymonitor.go), so a
// PROCESS_AUDIT_AND_COMPLIANCE-scope audit read can prove the finding was
// recorded.
func (h *IncidentReporter) Handle(ctx context.Context, query IncidentReportQry) error {
	if query.DID == "" {
		return errors.New("incident report is missing a linked contract or template DID")
	}
	if len(query.Findings) == 0 {
		return errors.New("incident report has no findings")
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		_ = tx.Rollback()
	}(tx)

	occurredAt := time.Now().UTC()
	for _, finding := range query.Findings {
		evt := event2.IncidentReportEvent{
			DID:        query.DID,
			RiskType:   finding.RiskType,
			Detail:     finding.Detail,
			ReportedBy: query.ReportedBy,
			OccurredAt: occurredAt,
			HolderDID:  query.HolderDID,
			UserRoles:  query.UserRoles,
		}
		if err := event.Create(ctx, tx, evt, componenttype.ProcessAuditAndCompliance); err != nil {
			return fmt.Errorf("could not create incident report event: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit transaction: %w", err)
	}
	return nil
}
