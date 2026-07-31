// Package contracttemplate implements the contract workflow engine's
// read-only queries against contract templates (e.g. listing approved
// templates available to create a contract from) — not to be confused with
// the templaterepository domain, which owns template authoring/versioning.
package contracttemplate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	event2 "digital-contracting-service/internal/contractworkflowengine/event"

	"digital-contracting-service/internal/contractworkflowengine/datatype/contracttemplatestate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contracttemplatetype"
	"digital-contracting-service/internal/contractworkflowengine/db"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
)

type GetAllApprovedTemplatesQry struct {
	RetrievedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type GetApprovedTemplateResult struct {
	DID          string
	Version      int
	State        contracttemplatestate.ContractTemplateState
	TemplateType contracttemplatetype.ContractTemplateType
	Name         *string
	Description  *string
	CreatedBy    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	*db.Responsible
	MetaData datatype.JSON
}

type GetAllApprovedTemplateHandler struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
}

func (h *GetAllApprovedTemplateHandler) Handle(ctx context.Context, query GetAllApprovedTemplatesQry) ([]GetApprovedTemplateResult, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	contractTemplates, err := h.CTRepo.ReadAllMetaData(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("could not read all contract templates: %w", err)
	}

	evt := event2.RetrieveAllTemplatesEvent{
		RetrievedBy: query.RetrievedBy,
		OccurredAt:  time.Now().UTC(),
		HolderDID:   query.HolderDID,
		UserRoles:   query.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	var contractTemplatesItems []GetApprovedTemplateResult
	for _, data := range contractTemplates {

		state, err := contracttemplatestate.NewContractTemplateState(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create contract template state: %w", err)
		}

		templateType, err := contracttemplatetype.NewContractTemplateType(data.TemplateType)
		if err != nil {
			return nil, fmt.Errorf("could not create contract template type: %w", err)
		}

		metadata := GetApprovedTemplateResult{
			DID:          data.DID,
			Version:      data.Version,
			State:        state,
			TemplateType: templateType,
			Name:         data.Name,
			Description:  data.Description,
			CreatedBy:    data.CreatedBy,
			CreatedAt:    data.CreatedAt,
			UpdatedAt:    data.UpdatedAt,
		}
		contractTemplatesItems = append(contractTemplatesItems, metadata)
	}

	return contractTemplatesItems, nil
}
