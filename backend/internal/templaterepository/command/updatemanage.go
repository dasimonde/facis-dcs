package command

import (
	"context"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/datatype/reviewtaskstate"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type UpdateManageCmd struct {
	DID            string
	DocumentNumber *string
	State          *contracttemplatestate.ContractTemplateState
	TemplateType   *contracttemplatetype.ContractTemplateType
	UpdatedAt      time.Time
	UpdatedBy      string
	Name           *string
	Description    *string
	TemplateData   *datatype.JSON
	IsManager      bool
}

type UpdateManager struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
}

func (h *UpdateManager) Handle(ctx context.Context, cmd UpdateManageCmd) error {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer tx.Rollback()

	oldData, err := h.CTRepo.ReadDataByID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read template data: %w", err)
	}

	if cmd.UpdatedAt.Unix() < oldData.UpdatedAt.Unix() {
		return errors.New("contract template was updated elsewhere, please reload")
	}

	if oldData.State == contracttemplatestate.Published.String() ||
		oldData.State == contracttemplatestate.Deleted.String() ||
		oldData.State == contracttemplatestate.Deprecated.String() ||
		oldData.State == contracttemplatestate.Approved.String() {
		return errors.New("invalid contract template state")
	}

	if cmd.State != nil {
		isValidState := *cmd.State == contracttemplatestate.Draft || *cmd.State == contracttemplatestate.Deleted
		if oldData.State == contracttemplatestate.Draft.String() && !isValidState {
			reviewTasksExist, err := h.RTRepo.TaskExist(ctx, tx, cmd.DID)
			if err != nil {
				return fmt.Errorf("could not check existing review tasks: %w", err)
			}

			approvalTaskExists, err := h.ATRepo.TaskExists(ctx, tx, cmd.DID)
			if err != nil {
				return fmt.Errorf("could not check existing approval tasks: %w", err)
			}

			if !reviewTasksExist || !approvalTaskExists {
				return errors.New("invalid state change")
			}
		}
	}

	newState := oldData.State
	if cmd.State != nil {
		if *cmd.State == contracttemplatestate.Draft || *cmd.State == contracttemplatestate.Deleted || *cmd.State == contracttemplatestate.Deprecated {

			err = h.RTRepo.Delete(ctx, tx, cmd.DID)
			if err != nil {
				return fmt.Errorf("could not delete review tasks: %w", err)
			}

			err = h.ATRepo.Delete(ctx, tx, cmd.DID)
			if err != nil {
				return fmt.Errorf("could not delete approval tasks: %w", err)
			}

		} else if *cmd.State == contracttemplatestate.Rejected || *cmd.State == contracttemplatestate.Submitted {
			err = h.RTRepo.ReopenTasks(ctx, tx, cmd.DID)
			if err != nil {
				return err
			}

			err = h.ATRepo.ReopenTasks(ctx, tx, cmd.DID)
			if err != nil {
				return err
			}
		} else if *cmd.State == contracttemplatestate.Reviewed {
			err = h.RTRepo.UpdateStateForAllTasks(ctx, tx, cmd.DID, reviewtaskstate.Approved.String())
			if err != nil {
				return err
			}

			err = h.ATRepo.ReopenTasks(ctx, tx, cmd.DID)
			if err != nil {
				return err
			}
		} else {
			return errors.New("contract invalid state")
		}

		newState = cmd.State.String()
	}

	var state string
	if cmd.State != nil {
		state = cmd.State.String()
	}

	var templateType string
	if cmd.TemplateType != nil {
		templateType = cmd.TemplateType.String()
	}

	newData := db.ContractTemplateUpdateData{
		DID:            cmd.DID,
		DocumentNumber: cmd.DocumentNumber,
		State:          state,
		TemplateType:   templateType,
		Name:           cmd.Name,
		Description:    cmd.Description,
		TemplateData:   cmd.TemplateData,
	}
	err = h.CTRepo.Update(ctx, tx, newData)
	if err != nil {
		return fmt.Errorf("could not update template data: %w", err)
	}

	evt := templateevents.UpdateManageEvent{
		DID:               cmd.DID,
		OldDocumentNumber: oldData.DocumentNumber,
		NewDocumentNumber: cmd.DocumentNumber,
		OldState:          &oldData.State,
		NewState:          &newState,
		OldName:           oldData.Name,
		NewName:           cmd.Name,
		OldDescription:    oldData.Description,
		NewDescription:    cmd.Description,
		OldTemplateData:   oldData.TemplateData,
		NewTemplateData:   cmd.TemplateData,
		UpdatedBy:         cmd.UpdatedBy,
		OccurredAt:        time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
