package qry

import (
	"context"
	"encoding/json"
	"time"

	"digital-contracting-service/internal/base/datatype"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/processauditandcompliance"

	"digital-contracting-service/internal/processauditandcompliance/datatype/eventtype"

	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/validation"
)

type GetTemplateApprovalProvenanceTrailQry struct {
	DID         string
	LogEntries  []datatype.AuditLogEntry
	RetrievedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type GetTemplateApprovalProvenanceTrailResult struct {
	ID               int64
	Component        componenttype.ComponentType
	EventType        eventtype.EventType
	EventData        any
	DID              *string
	CreatedAt        time.Time
	ResLogPredCID    *string
	GlobalLogPredCID *string
}

type TemplateApprovalProvenanceTrailAuditor struct {
}

func (h *TemplateApprovalProvenanceTrailAuditor) Handle(ctx context.Context, query GetTemplateApprovalProvenanceTrailQry) ([]datatype.AuditLogEntry, error) {

	findings := validation.AuditTemplateApprovalProvenance(query.DID, query.LogEntries)
	entries := make([]datatype.AuditLogEntry, 0, len(findings))
	for i, finding := range findings {

		data, err := json.Marshal(processauditandcompliance.TemplateApprovalProvenanceFindingEventData(finding, query.DID))
		if err != nil {
			return nil, err
		}

		entries = append(entries, datatype.AuditLogEntry{
			ID:        int64(-2000 - i),
			Component: componenttype.ContractTemplateRepo.String(),
			EventType: eventtype.TemplateApprovalProvenanceAuditFinding.String(),
			EventData: data,
			DID:       &query.DID,
			CreatedAt: time.Now().UTC(),
		})
	}
	return entries, nil
}
