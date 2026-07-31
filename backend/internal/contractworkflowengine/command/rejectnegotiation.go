package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/identity"

	db2 "digital-contracting-service/internal/dcstodcs/db"

	"digital-contracting-service/internal/base/datatype/userrole"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

type RejectNegotiationCmd struct {
	ID              string             `json:"id"`
	DID             string             `json:"did"`
	RejectedBy      string             `json:"rejected_by"`
	RejectionReason *string            `json:"rejection_reason"`
	HolderDID       string             `json:"holder_did"`
	UserRoles       userrole.UserRoles `json:"user_roles"`
	CauserDID       string             `json:"causer_did"`
}

type NegotiationRejector struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	NRepo       db.NegotiationRepo
	NTRepo      db.NegotiationTaskRepo
	SRepo       db2.SyncRepository
	DIDDocument identity.DIDDocument
}

func (h *NegotiationRejector) Handle(ctx context.Context, cmd RejectNegotiationCmd) error {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	processData, err := h.CRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not process core data: %w", err)
	}

	if err := contractstate.ValidateTransition(contractstate.ContractState(processData.State), contractstate.EventRejectNegotiation); err != nil {
		return err
	}

	isValidNegotiator, err := h.NTRepo.IsValidNegotiator(ctx, tx, cmd.DID, cmd.CauserDID)
	if err != nil {
		return fmt.Errorf("could not validate negotiator: %w", err)
	}

	if !isValidNegotiator {
		return ErrNotAParty
	}

	err = h.NRepo.Reject(ctx, tx, cmd.ID, cmd.CauserDID, cmd.RejectionReason)
	if err != nil {
		return fmt.Errorf("could not reject negotiation %w", err)
	}

	evt := contractevents.RejectNegotiationEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		RejectedBy:      cmd.RejectedBy,
		RejectionReason: cmd.RejectionReason,
		OccurredAt:      time.Now().UTC(),
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
