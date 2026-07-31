package contract

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/datatype/userrole"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

type GetHistoryByIDQry struct {
	DID         string
	RetrievedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type GetHistoryByIDResult struct {
	ID              string
	DID             string
	ContractVersion int
	State           contractstate.ContractState
	Name            *string
	Description     *string
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ContractData    *datatype.JSON
	TemplateDID     string
	TemplateVersion int
	StartDate       *time.Time
	ExpDate         *time.Time
	ExpPolicy       *expirationpolicy.ExpirationPolicy
	ExpNoticePeriod *int
	Responsible     *db.Responsible
}

type GetHistoryByIDHandler struct {
	Ctx   context.Context
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

func (h *GetHistoryByIDHandler) Handle(ctx context.Context, query GetHistoryByIDQry) ([]GetHistoryByIDResult, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	entries, err := h.CRepo.ReadHistoryByDID(ctx, tx, query.DID)
	if err != nil {
		return nil, fmt.Errorf("could not get contract history data: %w", err)
	}

	evt := contractevents.RetrieveHistoryByDIDEvent{
		DID:         query.DID,
		RetrievedBy: query.RetrievedBy,
		OccurredAt:  time.Now().UTC(),
		HolderDID:   query.HolderDID,
		UserRoles:   query.UserRoles,
	}
	err = event.Create(h.Ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	result := make([]GetHistoryByIDResult, len(entries))
	for idx, entry := range entries {

		state, err := contractstate.NewContractState(entry.State)
		if err != nil {
			return nil, fmt.Errorf("could not create contract state: %w", err)
		}

		var expPolicy *expirationpolicy.ExpirationPolicy
		if entry.ExpPolicy != nil {
			policy, err := expirationpolicy.NewExpirationPolicy(*entry.ExpPolicy)
			if err != nil {
				return nil, contractworkflowengine.MakeInternalError(err)
			}
			expPolicy = &policy
		}

		result[idx] = GetHistoryByIDResult{
			ID:              entry.ID,
			DID:             entry.DID,
			ContractVersion: entry.ContractVersion,
			State:           state,
			Name:            entry.Name,
			Description:     entry.Description,
			CreatedBy:       entry.CreatedBy,
			CreatedAt:       entry.CreatedAt,
			UpdatedAt:       entry.UpdatedAt,
			ContractData:    entry.ContractData,
			StartDate:       entry.StartDate,
			ExpDate:         entry.ExpDate,
			ExpPolicy:       expPolicy,
			ExpNoticePeriod: entry.ExpNoticePeriod,
			Responsible:     entry.Responsible,
		}
	}

	return result, nil
}
