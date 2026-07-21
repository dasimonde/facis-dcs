package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/datatype/userrole"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/templaterepository/datatype/approvaltaskstate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"

	"github.com/jmoiron/sqlx"
)

type RejectCmd struct {
	DID        string
	UpdatedAt  time.Time
	RejectedBy string
	Reason     string
	HolderDID  string
	UserRoles  userrole.UserRoles
}

type Rejecter struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
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

	processData, err := h.CTRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	// Optimistic concurrency (see command package doc / ADR-0007).
	if cmd.UpdatedAt.Unix() < processData.ContentUpdatedAt.Unix() {
		return errors.New("contract template was updated elsewhere, please reload")
	}

	if processData.State != contracttemplatestate.Reviewed.String() {
		return errors.New("invalid contract template state")
	}

	exist, err := h.ATRepo.IsValidApprover(ctx, tx, cmd.DID, cmd.RejectedBy)
	if err != nil {
		return err
	}

	if !exist {
		return errors.New("invalid user")
	}

	err = h.ATRepo.UpdateState(ctx, tx, cmd.DID, cmd.RejectedBy, approvaltaskstate.Rejected.String())
	if err != nil {
		return fmt.Errorf("could not update approval task state: %w", err)
	}

	err = h.CTRepo.UpdateState(ctx, tx, cmd.DID, contracttemplatestate.Rejected.String())
	if err != nil {
		return fmt.Errorf("could not update current template state: %w", err)
	}

	evt := templateevents.RejectEvent{
		DID:            cmd.DID,
		DocumentNumber: processData.DocumentNumber,
		Version:        processData.Version,
		RejectedBy:     cmd.RejectedBy,
		Reason:         cmd.Reason,
		OccurredAt:     time.Now().UTC(),
		HolderDID:      cmd.HolderDID,
		UserRoles:      cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
