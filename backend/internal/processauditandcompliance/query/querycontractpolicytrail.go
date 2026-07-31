package qry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/processauditandcompliance"

	"digital-contracting-service/internal/processauditandcompliance/datatype/eventtype"

	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/templaterepository/db"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
)

type GetContractPolicyTrailQry struct {
	RetrievedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type ContractPolicyTrailAuditor struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
}

func (h *ContractPolicyTrailAuditor) Handle(ctx context.Context, query GetContractPolicyTrailQry) (map[string][]datatype.AuditLogEntry, error) {

	result := map[string][]datatype.AuditLogEntry{}
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	pagination := datatype.Pagination{}
	templates, err := h.CTRepo.ReadAllMetaData(ctx, tx, pagination)
	if err != nil {
		return nil, fmt.Errorf("could not read meta data: %w", err)
	}

	for templateIndex, metadata := range templates {
		template, err := h.CTRepo.ReadDataByID(ctx, tx, metadata.DID)
		if err != nil {
			return nil, fmt.Errorf("could not read data: %w", err)
		}

		if template.TemplateData == nil || !template.TemplateData.IsNotNullValue() {
			continue
		}
		findings, err := validation.AuditTemplatePolicies(template.TemplateData, validation.TemplatePolicyAuditMetadata{
			DID:          template.DID,
			TemplateType: template.TemplateType,
			State:        template.State,
		})
		if err != nil {
			return nil, fmt.Errorf("could not audit template policies: %w", err)
		}

		entries := make([]datatype.AuditLogEntry, 0, len(findings))
		for findingIndex, finding := range findings {

			data, err := json.Marshal(processauditandcompliance.TemplatePolicyFindingEventData(finding, template))
			if err != nil {
				return nil, err
			}

			entries = append(entries, datatype.AuditLogEntry{
				ID:        int64(-4000000 - (templateIndex * 10000) - findingIndex),
				Component: componenttype.ContractTemplateRepo.String(),
				EventType: eventtype.TemplatePolicyAuditFinding.String(),
				EventData: data,
				DID:       &template.DID,
				CreatedAt: time.Now().UTC(),
			})
		}
		result[template.DID] = entries
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return result, nil
}
