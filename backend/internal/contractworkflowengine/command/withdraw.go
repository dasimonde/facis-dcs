package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/identity"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

// WithdrawCmd carries the inputs for the initiator retracting a contract
// before it has been approved (SRS 1.2/2.2.6). Allowed from
// OFFERED/NEGOTIATION/SUBMITTED/REVIEWED — never once APPROVED.
type WithdrawCmd struct {
	DID         string             `json:"did"`
	UpdatedAt   time.Time          `json:"updated_at"`
	WithdrawnBy string             `json:"withdrawn_by"`
	HolderDID   string             `json:"holder_did"`
	UserRoles   userrole.UserRoles `json:"user_roles"`
	CauserDID   string             `json:"causer_did"`
}

// Withdrawer handles the WithdrawCmd command.
type Withdrawer struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	DIDDocument identity.DIDDocument
}

func (h *Withdrawer) Handle(ctx context.Context, cmd WithdrawCmd) error {

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
		return fmt.Errorf("could not read process data: %w", err)
	}

	localPeer, err := h.DIDDocument.GetID()
	if err != nil {
		return err
	}

	// Optimistic concurrency: reject if the caller's view of the contract is
	// older than what's stored (see package doc / ADR-0007).
	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		if localPeer != cmd.CauserDID {
			return errors.New("contract was updated elsewhere, please force synchronisation and reload")
		}
		return errors.New("contract was updated elsewhere, please reload")
	}

	if !cmd.UserRoles.HasRoles(userrole.ContractCreator, userrole.SystemContractCreator) {
		return errors.New("invalid user permission")
	}

	// Withdraw is initiator-only.
	if cmd.CauserDID == localPeer && cmd.WithdrawnBy != processData.CreatedBy {
		return errors.New("invalid participant")
	}

	currentState := contractstate.ContractState(processData.State)
	if err := contractstate.ValidateTransition(currentState, contractstate.EventWithdraw); err != nil {
		return err
	}

	err = h.CRepo.UpdateState(ctx, tx, cmd.DID, contractstate.Withdrawn.String())
	if err != nil {
		return fmt.Errorf("could not update contract state: %w", err)
	}

	evt := contractevents.WithdrawEvent{
		DID:             cmd.DID,
		HolderDID:       cmd.HolderDID,
		ContractVersion: processData.ContractVersion,
		WithdrawnBy:     cmd.WithdrawnBy,
		OccurredAt:      time.Now().UTC(),
		UserRoles:       cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
