package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"
	"digital-contracting-service/internal/signingmanagement/datatype/signingstatus"
	"digital-contracting-service/internal/signingmanagement/db"
	signingmanagementevents "digital-contracting-service/internal/signingmanagement/event"
)

type GetAllMetadataQry struct {
	RetrievedBy string
	HolderDID   string
	Pagination  datatype.Pagination
	UserRoles   userrole.UserRoles
}

type MetadataItem struct {
	DID             string
	ContractVersion int
	Name            *string
	Description     *string
	State           contractstate.ContractState
	CreatedAt       time.Time
	UpdatedAt       time.Time
	MetaData        datatype.JSON
	CreatedBy       string
	StartDate       *time.Time
	ExpDate         *time.Time
	ExpPolicy       *expirationpolicy.ExpirationPolicy
	ExpNoticePeriod *int
	Responsible     *db.Responsible
}

type SigningTaskItem struct {
	DID             string
	ContractVersion int
	State           signingstatus.SigningStatus
	SignerDID       string
	CreatedAt       time.Time
}

type GetAllMetadataResult struct {
	Contracts    []MetadataItem
	SigningTasks []SigningTaskItem
}

type GetAllMetadataHandler struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

func (h *GetAllMetadataHandler) Handle(ctx context.Context, query GetAllMetadataQry) (*GetAllMetadataResult, error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not create transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	var contractsMetadata []db.ContractMetadata
	if query.Pagination.Limit >= 0 {
		contractsMetadata, err = h.CRepo.ReadAllMetaData(ctx, tx, query.Pagination)
		if err != nil {
			return nil, fmt.Errorf("could not read all contracts: %w", err)
		}
	}

	signingTasks, err := h.CRepo.ReadAllSigningTasks(ctx, tx)
	if err != nil {
		return nil, fmt.Errorf("could not read all signing tasks: %w", err)
	}

	evt := signingmanagementevents.RetrieveAllEvent{
		RetrievedBy: query.RetrievedBy,
		OccurredAt:  time.Now().UTC(),
		HolderDID:   query.HolderDID,
		UserRoles:   query.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.SignatureManagement)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	didToMetadata := make(map[string]MetadataItem)
	var contractItems []MetadataItem
	for _, data := range contractsMetadata {

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

		metadata := MetadataItem{
			DID:             data.DID,
			ContractVersion: data.ContractVersion,
			State:           state,
			Name:            data.Name,
			Description:     data.Description,
			CreatedBy:       data.CreatedBy,
			CreatedAt:       data.CreatedAt,
			UpdatedAt:       data.UpdatedAt,
			StartDate:       data.StartDate,
			ExpDate:         data.ExpDate,
			ExpPolicy:       expPolicy,
			ExpNoticePeriod: data.ExpNoticePeriod,
			Responsible:     data.Responsible,
		}
		contractItems = append(contractItems, metadata)

		didToMetadata[data.DID] = metadata
	}

	var signingTaskItems []SigningTaskItem
	for _, data := range signingTasks {

		state, err := signingstatus.NewSigningStatus(data.State)
		if err != nil {
			return nil, fmt.Errorf("could not create signing status: %w", err)
		}

		metadata, exists := didToMetadata[data.ContractDID]
		var contractVersion int
		if exists {
			contractVersion = metadata.ContractVersion
		}

		signingTaskItems = append(signingTaskItems, SigningTaskItem{
			DID:             data.ContractDID,
			State:           state,
			ContractVersion: contractVersion,
			SignerDID:       data.SignerDID,
			CreatedAt:       data.CreatedAt,
		})
	}

	return &GetAllMetadataResult{
		Contracts:    contractItems,
		SigningTasks: signingTaskItems,
	}, nil
}
