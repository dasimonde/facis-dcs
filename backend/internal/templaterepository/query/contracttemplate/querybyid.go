package contracttemplate

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
)

type GetByIDQry struct {
	DID         string
	RetrievedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type GetByIDResult struct {
	DID          string
	Version      int
	State        contracttemplatestate.ContractTemplateState
	TemplateType contracttemplatetype.ContractTemplateType
	Name         *string
	Description  *string
	CreatedBy    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	TemplateData *datatype.JSON
}

type GetByIDHandler struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
}

func (h *GetByIDHandler) Handle(ctx context.Context, query GetByIDQry) (*GetByIDResult, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	data, err := h.CTRepo.ReadDataByID(ctx, tx, query.DID)
	if err != nil {
		return nil, fmt.Errorf("could not get contract template data: %w", err)
	}

	evt := templateevents.RetrieveByIDEvent{
		DID:         query.DID,
		Version:     data.Version,
		RetrievedBy: query.RetrievedBy,
		OccurredAt:  time.Now().UTC(),
		HolderDID:   query.HolderDID,
		UserRoles:   query.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	state, err := contracttemplatestate.NewContractTemplateState(data.State)
	if err != nil {
		return nil, fmt.Errorf("could not create contract template state: %w", err)
	}

	templateType, err := contracttemplatetype.NewContractTemplateType(data.TemplateType)
	if err != nil {
		return nil, fmt.Errorf("could not create contract template type: %w", err)
	}

	return &GetByIDResult{
		DID:          query.DID,
		Version:      data.Version,
		State:        state,
		TemplateType: templateType,
		Name:         data.Name,
		Description:  data.Description,
		CreatedBy:    data.CreatedBy,
		CreatedAt:    data.CreatedAt,
		UpdatedAt:    data.UpdatedAt,
		TemplateData: data.TemplateData,
	}, nil
}
