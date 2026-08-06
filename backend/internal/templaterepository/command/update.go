package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"

	"github.com/jmoiron/sqlx"
)

type UpdateCmd struct {
	DID          string
	TemplateType *contracttemplatetype.ContractTemplateType
	UpdatedAt    time.Time
	UpdatedBy    string
	Name         *string
	Description  *string
	TemplateData *datatype.JSON
	HolderDID    string
	UserRoles    userrole.UserRoles
}

type Updater struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
}

func (h *Updater) Handle(ctx context.Context, cmd UpdateCmd) error {
	if cmd.TemplateData != nil && cmd.TemplateData.IsNotNullValue() {
		normalizedTemplateData, err := validation.NormalizeTemplateDataForPersistence(cmd.TemplateData, cmd.DID)
		if err != nil {
			return fmt.Errorf("template data validation failed: %w", err)
		}
		cmd.TemplateData = normalizedTemplateData
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	oldData, err := h.CTRepo.ReadDataByID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read template data: %w", err)
	}

	// Optimistic concurrency (see command package doc / ADR-0007).
	if cmd.UpdatedAt.Unix() < oldData.UpdatedAt.Unix() {
		return fmt.Errorf("contract template %w, please reload", base.ErrUpdatedElsewhere)
	}

	if oldData.State == contracttemplatestate.Draft.String() || oldData.State == contracttemplatestate.Rejected.String() {

		if !cmd.UserRoles.HasRoles(userrole.TemplateCreator, userrole.TemplateManager) {
			return errors.New("invalid user permission")
		}

	} else if oldData.State == contracttemplatestate.Submitted.String() {

		if !cmd.UserRoles.HasRoles(userrole.TemplateReviewer, userrole.TemplateManager) {
			return errors.New("invalid user permission")
		}

		if cmd.UserRoles.HasRoles(userrole.TemplateReviewer) {
			isValidReviewer, err := h.RTRepo.IsValidReviewer(ctx, tx, cmd.DID, cmd.UpdatedBy)
			if err != nil {
				return err
			}
			if !isValidReviewer {
				return errors.New("user is not a valid reviewer for that contract template")
			}
		}

	} else {
		return errors.New("current contract template state is invalid")
	}

	err = h.RTRepo.ReopenTasks(ctx, tx, cmd.DID)
	if err != nil {
		return err
	}

	err = h.ATRepo.ReopenTasks(ctx, tx, cmd.DID)
	if err != nil {
		return err
	}

	var templateType string
	if cmd.TemplateType != nil {
		templateType = cmd.TemplateType.String()
	}

	err = h.CTRepo.CreateHistoryEntryForDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not create history entry: %w", err)
	}

	newData := db.ContractTemplateUpdateData{
		DID:          cmd.DID,
		TemplateType: templateType,
		Name:         cmd.Name,
		Description:  cmd.Description,
		TemplateData: cmd.TemplateData,
	}
	err = h.CTRepo.Update(ctx, tx, newData)
	if err != nil {
		return fmt.Errorf("could not update template data: %w", err)
	}

	evt := templateevents.UpdateEvent{
		DID:             cmd.DID,
		OldName:         oldData.Name,
		NewName:         cmd.Name,
		OldDescription:  oldData.Description,
		NewDescription:  cmd.Description,
		OldTemplateData: oldData.TemplateData,
		NewTemplateData: cmd.TemplateData,
		UpdatedBy:       cmd.UpdatedBy,
		OccurredAt:      time.Now().UTC(),
		HolderDID:       cmd.HolderDID,
		UserRoles:       cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
