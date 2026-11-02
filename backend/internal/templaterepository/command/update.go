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
	"digital-contracting-service/internal/templaterepository/datatype/partytype"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"

	"github.com/jmoiron/sqlx"
)

type UpdateCmd struct {
	PartyType    string
	DID          string
	TemplateType *contracttemplatetype.ContractTemplateType
	UpdatedAt    time.Time
	UpdatedBy    string
	Name         *string
	Description  *string
	TemplateData *datatype.JSON
	Jurisdiction *string
	Industry     *string
	// TemplateLevel and RequiredTemplates are left unchanged when nil.
	TemplateLevel     *string
	RequiredTemplates []string
	HolderDID         string
	UserRoles         userrole.UserRoles
}

type Updater struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
}

func (h *Updater) Handle(ctx context.Context, cmd UpdateCmd) error {
	if cmd.PartyType != "" {
		if _, err := partytype.New(cmd.PartyType); err != nil {
			return err
		}
	}
	if cmd.TemplateData != nil && cmd.TemplateData.IsNotNullValue() {
		normalizedTemplateData, err := validation.NormalizeTemplateDataForPersistence(cmd.TemplateData, cmd.DID)
		if err != nil {
			return fmt.Errorf("%w: template data validation failed: %w", ErrInvalidRequest, err)
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
	if cmd.UpdatedAt.Unix() < oldData.ContentUpdatedAt.Unix() {
		return fmt.Errorf("contract template %w, please reload", base.ErrUpdatedElsewhere)
	}

	if oldData.State == contracttemplatestate.Draft.String() || oldData.State == contracttemplatestate.Rejected.String() {

		if !cmd.UserRoles.HasRoles(userrole.TemplateCreator, userrole.TemplateManager) {
			return fmt.Errorf("%w: invalid user permission", ErrInvalidRequest)
		}

	} else if oldData.State == contracttemplatestate.Submitted.String() {

		if !cmd.UserRoles.HasRoles(userrole.TemplateReviewer, userrole.TemplateManager) {
			return fmt.Errorf("%w: invalid user permission", ErrInvalidRequest)
		}

		if cmd.UserRoles.HasRoles(userrole.TemplateReviewer) {
			isValidReviewer, err := h.RTRepo.IsValidReviewer(ctx, tx, cmd.DID, cmd.UpdatedBy)
			if err != nil {
				return err
			}
			if !isValidReviewer {
				return fmt.Errorf("%w: user is not a valid reviewer for that contract template", ErrInvalidRequest)
			}
		}

	} else {
		return ErrInvalidTemplateState
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
		PartyType:    cmd.PartyType,
		Name:         cmd.Name,
		Description:  cmd.Description,
		TemplateData: cmd.TemplateData,
		Jurisdiction: cmd.Jurisdiction,
		Industry:     cmd.Industry,

		TemplateLevel:     cmd.TemplateLevel,
		RequiredTemplates: cmd.RequiredTemplates,
	}
	err = h.CTRepo.Update(ctx, tx, newData)
	if err != nil {
		return fmt.Errorf("could not update template data: %w", err)
	}
	updated, err := h.CTRepo.ReadDataByID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read template data: %w", err)
	}

	evt := templateevents.UpdateEvent{
		DID:               cmd.DID,
		OldName:           oldData.Name,
		NewName:           cmd.Name,
		OldDescription:    oldData.Description,
		NewDescription:    cmd.Description,
		OldTemplateData:   oldData.TemplateData,
		NewTemplateData:   cmd.TemplateData,
		OldClassification: classification(oldData),
		NewClassification: classification(updated),
		UpdatedBy:         cmd.UpdatedBy,
		OccurredAt:        time.Now().UTC(),
		HolderDID:         cmd.HolderDID,
		UserRoles:         cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}

// classification of a template revision for the update events (DCS-FR-TR-21).
func classification(template *db.ContractTemplate) templateevents.Classification {
	return templateevents.Classification{
		TemplateType:      template.TemplateType,
		PartyType:         template.PartyType,
		Jurisdiction:      template.Jurisdiction,
		Industry:          template.Industry,
		TemplateLevel:     template.TemplateLevel,
		RequiredTemplates: template.RequiredTemplates,
	}
}
