package command

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
)

type ArchiveCmd struct {
	DID        string
	UpdatedAt  time.Time
	ArchivedBy string
	HolderDID  string
	UserRoles  userrole.UserRoles
}

type Archiver struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
}

func (h *Archiver) Handle(ctx context.Context, cmd ArchiveCmd) error {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		err := tx.Rollback()
		if err != nil {
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

	if processData.State == contracttemplatestate.Deprecated.String() ||
		processData.State == contracttemplatestate.Deleted.String() {
		return errors.New("invalid contract template state")
	}

	if processData.State == contracttemplatestate.Registered.String() || processData.State == contracttemplatestate.Published.String() {

		err = h.CTRepo.UpdateState(ctx, tx, cmd.DID, contracttemplatestate.Deprecated.String())
		if err != nil {
			return fmt.Errorf("could not update state: %w", err)
		}

	} else {

		err = h.CTRepo.UpdateState(ctx, tx, cmd.DID, contracttemplatestate.Deleted.String())
		if err != nil {
			return fmt.Errorf("could not update state: %w", err)
		}
	}

	evt := templateevents.ArchiveEvent{
		DID:            cmd.DID,
		DocumentNumber: processData.DocumentNumber,
		Version:        processData.Version,
		ArchivedBy:     cmd.ArchivedBy,
		OccurredAt:     time.Now().UTC(),
		HolderDID:      cmd.HolderDID,
		UserRoles:      cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	err = h.RTRepo.Delete(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not delete review tasks: %w", err)
	}

	err = h.ATRepo.Delete(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not delete approval tasks: %w", err)
	}

	return tx.Commit()
}
