package qry

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/datatype"

	templaterepository "digital-contracting-service/gen/template_repository"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/processauditandcompliance"

	"digital-contracting-service/internal/processauditandcompliance/datatype/eventtype"

	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/templaterepository/db"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/userrole"
)

type GetTemplatePolicyTrailQry struct {
	DID         string
	RetrievedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}
type ContractTemplatePolicyTrailAuditor struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
}

func (h *ContractTemplatePolicyTrailAuditor) Handle(ctx context.Context, query GetTemplatePolicyTrailQry) ([]datatype.AuditLogEntry, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	template, err := h.CTRepo.ReadDataByID(ctx, tx, query.DID)
	if err != nil {
		return nil, err
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	findings, err := validation.AuditTemplatePolicies(template.TemplateData, validation.TemplatePolicyAuditMetadata{
		DID:          template.DID,
		TemplateType: template.TemplateType,
		State:        template.State,
	})

	if err != nil {
		return nil, templaterepository.MakeInternalError(err)
	}

	history := make([]datatype.AuditLogEntry, 0)
	for i, finding := range findings {

		data, err := json.Marshal(processauditandcompliance.TemplatePolicyFindingEventData(finding, template))
		if err != nil {
			return nil, err
		}

		history = append(history, datatype.AuditLogEntry{
			ID:        int64(-1 - i),
			Component: componenttype.ContractTemplateRepo.String(),
			EventType: eventtype.TemplatePolicyAuditFinding.String(),
			EventData: data,
			DID:       &query.DID,
			CreatedAt: time.Now().UTC(),
		})
	}

	return history, nil
}
