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
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"

	"github.com/jmoiron/sqlx"
)

type CopyCmd struct {
	NewDID    string
	CopyDID   string
	CopiedBy  string
	HolderDID string
	UserRoles userrole.UserRoles
}

type Copier struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
}

// Handle creates cmd.NewDID as a copy of cmd.CopyDID. The actual versioning
// decision is made inside CTRepo.CopyFromDID (SQL, see db/pg): if the source
// is not yet REGISTERED/PUBLISHED, the copy starts a brand-new version
// lineage (version=1); if the source already is, the copy becomes the next
// version of the same lineage (version+1, same base_template) — this
// single command backs both "duplicate a draft" and "create the next
// version of a published template".
func (h *Copier) Handle(ctx context.Context, cmd CopyCmd) error {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	version, err := h.CTRepo.CopyFromDID(ctx, tx, cmd.CopyDID, cmd.NewDID)
	if err != nil {
		return fmt.Errorf("could not copy contract template: %w", err)
	}

	copiedTemplate, err := h.CTRepo.ReadDataByID(ctx, tx, cmd.NewDID)
	if err != nil {
		return fmt.Errorf("could not read copied contract template: %w", err)
	}
	normalizedTemplateData, err := validation.NormalizeTemplateDataForPersistence(copiedTemplate.TemplateData, cmd.NewDID)
	if err != nil {
		return fmt.Errorf("copied template data validation failed: %w", err)
	}
	err = h.CTRepo.Update(ctx, tx, db.ContractTemplateUpdateData{
		DID:          cmd.NewDID,
		TemplateData: normalizedTemplateData,
	})
	if err != nil {
		return fmt.Errorf("could not normalize copied contract template data: %w", err)
	}

	evt := templateevents.CopyEvent{
		NewDID:     cmd.NewDID,
		CopyDID:    cmd.CopyDID,
		CopiedBy:   cmd.CopiedBy,
		NewVersion: version,
		OccurredAt: time.Now().UTC(),
		HolderDID:  cmd.HolderDID,
		UserRoles:  cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
