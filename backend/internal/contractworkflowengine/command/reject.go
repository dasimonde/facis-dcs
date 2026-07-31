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

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/datatype/approvaltaskstate"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

type RejectCmd struct {
	DID        string             `json:"did"`
	UpdatedAt  time.Time          `json:"updated_at"`
	RejectedBy string             `json:"rejected_by"`
	Reason     string             `json:"reason"`
	HolderDID  string             `json:"holder_did"`
	UserRoles  userrole.UserRoles `json:"user_roles"`
	CauserDID  string             `json:"causer_did"`
}

type Rejecter struct {
	DB          *sqlx.DB
	CRepo       db.ContractRepo
	RTRepo      db.ReviewTaskRepo
	ATRepo      db.ApprovalTaskRepo
	SRepo       db2.SyncRepository
	DIDDocument identity.DIDDocument
}

func (h *Rejecter) Handle(ctx context.Context, cmd RejectCmd) error {

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

	if err := contractstate.ValidateTransition(contractstate.ContractState(processData.State), contractstate.EventReject); err != nil {
		return err
	}

	exist, err := h.ATRepo.IsValidApprover(ctx, tx, cmd.DID, cmd.CauserDID)
	if err != nil {
		return err
	}

	if !exist {
		return errors.New("invalid user")
	}

	err = h.ATRepo.UpdateState(ctx, tx, cmd.DID, cmd.CauserDID, approvaltaskstate.Rejected.String())
	if err != nil {
		return fmt.Errorf("could not update approval task state: %w", err)
	}

	err = h.CRepo.UpdateState(ctx, tx, cmd.DID, contractstate.Rejected.String())
	if err != nil {
		return fmt.Errorf("could not update current state: %w", err)
	}

	evt := contractevents.RejectEvent{
		DID:             cmd.DID,
		ContractVersion: processData.ContractVersion,
		RejectedBy:      cmd.RejectedBy,
		Reason:          cmd.Reason,
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
