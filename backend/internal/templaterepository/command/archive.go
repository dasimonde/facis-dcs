package command

import (
	"context"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type ArchiveCmd struct {
	DID        string
	UpdatedAt  time.Time
	ArchivedBy string
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
	defer tx.Rollback()

	processData, err := h.CTRepo.ReadProcessData(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		return errors.New("contract template was updated elsewhere, please reload")
	}

	if processData.State == contracttemplatestate.Deprecated.String() ||
		processData.State == contracttemplatestate.Deleted.String() {
		return errors.New("invalid contract template state")
	}

	if processData.State == contracttemplatestate.Approved.String() || processData.State == contracttemplatestate.Published.String() {

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
