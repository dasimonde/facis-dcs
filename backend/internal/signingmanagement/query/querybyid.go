package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/signingmanagement/datatype/signingstatus"

	contractworkflowengine "digital-contracting-service/gen/contract_workflow_engine"

	"digital-contracting-service/internal/contractworkflowengine/datatype/expirationpolicy"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/signingmanagement/db"
	signingmanagementevents "digital-contracting-service/internal/signingmanagement/event"
)

type GetByIDQry struct {
	DID         string
	RetrievedBy string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type Contract struct {
	DID             string
	ContractVersion int
	State           contractstate.ContractState
	Name            *string
	Description     *string
	CreatedBy       string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	ContractData    *datatype.JSON
	StartDate       *time.Time
	ExpDate         *time.Time
	ExpPolicy       *expirationpolicy.ExpirationPolicy
	ExpNoticePeriod *int
	Responsible     *db.Responsible
}

type SignatureEnvelope struct {
	ContractDID    string
	SignerDID      string
	CredentialType string
	Status         signingstatus.SigningStatus
	SignedAt       *string
	RevokedAt      *string
	IpfsCID        *string
}

type GetByIDResult struct {
	Contract Contract
	// SignatureEnvelope is nil for an APPROVED-unsigned contract: no signature
	// has been submitted yet, so no envelope exists.
	SignatureEnvelope *SignatureEnvelope
}

type GetByIDHandler struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

func (h *GetByIDHandler) Handle(ctx context.Context, query GetByIDQry) (*GetByIDResult, error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	contractResult, err := h.CRepo.ReadDataByDID(ctx, tx, query.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read contract data: %w", err)
	}

	envelopResult, err := h.CRepo.ReadLatestEnvelopeByContractDID(ctx, tx, query.DID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("could not read signature envelope: %w", err)
	}
	envelopeMissing := errors.Is(err, sql.ErrNoRows)

	evt := signingmanagementevents.RetrieveByIDEvent{
		DID:         query.DID,
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

	state, err := contractstate.NewContractState(contractResult.State)
	if err != nil {
		return nil, fmt.Errorf("could not create contract state: %w", err)
	}

	var expPolicy *expirationpolicy.ExpirationPolicy
	if contractResult.ExpPolicy != nil {
		policy, err := expirationpolicy.NewExpirationPolicy(*contractResult.ExpPolicy)
		if err != nil {
			return nil, contractworkflowengine.MakeInternalError(err)
		}
		expPolicy = &policy
	}

	contract := Contract{
		DID:             query.DID,
		ContractVersion: contractResult.ContractVersion,
		State:           state,
		Name:            contractResult.Name,
		Description:     contractResult.Description,
		CreatedBy:       contractResult.CreatedBy,
		CreatedAt:       contractResult.CreatedAt,
		UpdatedAt:       contractResult.UpdatedAt,
		ContractData:    contractResult.ContractData,
		StartDate:       contractResult.StartDate,
		ExpDate:         contractResult.ExpDate,
		ExpPolicy:       expPolicy,
		ExpNoticePeriod: contractResult.ExpNoticePeriod,
		Responsible:     contractResult.Responsible,
	}

	if envelopeMissing {
		return &GetByIDResult{Contract: contract}, nil
	}

	signingStatus, err := signingstatus.NewSigningStatus(envelopResult.Status)
	if err != nil {
		return nil, fmt.Errorf("could not create signing status: %w", err)
	}

	envelop := SignatureEnvelope{
		ContractDID:    envelopResult.ContractDID,
		SignerDID:      envelopResult.SignerDID,
		CredentialType: envelopResult.CredentialType,
		Status:         signingStatus,
		SignedAt:       envelopResult.SignedAt,
		RevokedAt:      envelopResult.RevokedAt,
		IpfsCID:        envelopResult.IpfsCID,
	}

	return &GetByIDResult{
		Contract:          contract,
		SignatureEnvelope: &envelop,
	}, nil
}
