package contract

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	"digital-contracting-service/internal/contractworkflowengine/db"

	"github.com/jmoiron/sqlx"
)

type GetProcessDataByIDQry struct {
	DID         string
	RetrievedBy string
	HolderDID   string
}

type GetProcessDataByIDResult struct {
	DID             string
	ContractVersion int
	State           contractstate.ContractState
	CreatedBy       string
	UpdatedAt       time.Time
	StartDate       *time.Time
	ExpDate         *time.Time
	ExpPolicy       *expirationpolicy.ExpirationPolicy
	ExpNoticePeriod *int
}

type GetProcessDataByIDHandler struct {
	Ctx   context.Context
	DB    *sqlx.DB
	CRepo db.ContractRepo
	NRepo db.NegotiationRepo
}

func (h *GetProcessDataByIDHandler) Handle(ctx context.Context, query GetProcessDataByIDQry) (*GetProcessDataByIDResult, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	data, err := h.CRepo.ReadProcessDataByDID(ctx, tx, query.DID)
	if err != nil {
		return nil, fmt.Errorf("could not get contract process data: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	state, err := contractstate.NewContractState(data.State)
	if err != nil {
		return nil, fmt.Errorf("could not create contract state: %w", err)
	}

	var expPolicy *expirationpolicy.ExpirationPolicy
	if data.ExpPolicy != nil {
		policy, err := expirationpolicy.NewExpirationPolicy(*data.ExpPolicy)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		expPolicy = &policy
	}

	return &GetProcessDataByIDResult{
		DID:             query.DID,
		ContractVersion: data.ContractVersion,
		State:           state,
		CreatedBy:       data.CreatedBy,
		UpdatedAt:       data.UpdatedAt,
		StartDate:       data.StartDate,
		ExpDate:         data.ExpDate,
		ExpPolicy:       expPolicy,
		ExpNoticePeriod: data.ExpNoticePeriod,
	}, nil
}
