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
	"digital-contracting-service/internal/templaterepository/datatype/actionflag"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/reviewtaskstate"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"

	"github.com/jmoiron/sqlx"
)

type SubmitCmd struct {
	DID         string
	UpdatedAt   time.Time
	SubmittedBy string
	ActionFlag  *actionflag.ActionFlag
	Comments    []string
	HolderDID   string
	UserRoles   userrole.UserRoles
}

type Submitter struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
}

func createTasks(ctx context.Context, tx *sqlx.Tx, rtRepo db.ReviewTaskRepo, atRepo db.ApprovalTaskRepo, cmd SubmitCmd) error {
	reviewTask := db.ReviewTaskData{
		DID:       cmd.DID,
		Reviewer:  cmd.SubmittedBy,
		State:     reviewtaskstate.Open.String(),
		CreatedBy: cmd.SubmittedBy,
	}
	_, err := rtRepo.Create(ctx, tx, reviewTask)
	if err != nil {
		return fmt.Errorf("could not create review tasks: %w", err)
	}

	data := db.ApprovalTaskData{
		DID:       cmd.DID,
		CreatedBy: cmd.SubmittedBy,
		Approver:  cmd.SubmittedBy,
		State:     reviewtaskstate.Open.String(),
	}

	_, err = atRepo.Create(ctx, tx, data)
	if err != nil {
		return fmt.Errorf("could not create approval task: %w", err)
	}

	return nil
}

func (h *Submitter) Handle(ctx context.Context, cmd SubmitCmd) error {

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
		return fmt.Errorf("could not process core data: %w", err)
	}

	// Optimistic concurrency: reject if the caller's view of the template is
	// older than what's stored (see command package doc / ADR-0007).
	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		return errors.New("contract template was updated elsewhere, please reload")
	}

	var responsible *any
	var nextTemplateState contracttemplatestate.ContractTemplateState
	if processData.State == contracttemplatestate.Draft.String() {

		if !cmd.UserRoles.HasRoles(userrole.TemplateCreator, userrole.TemplateManager) {
			return errors.New("invalid user permission")
		}

		err = createTasks(ctx, tx, h.RTRepo, h.ATRepo, cmd)
		if err != nil {
			return err
		}

		nextTemplateState = contracttemplatestate.Submitted

	} else if processData.State == contracttemplatestate.Rejected.String() {

		if !cmd.UserRoles.HasRoles(userrole.TemplateCreator, userrole.TemplateManager) {
			return errors.New("invalid user permission")
		}

		err := h.RTRepo.ReopenTasks(ctx, tx, cmd.DID)
		if err != nil {
			return errors.New("could not reopen review tasks")
		}

		err = h.ATRepo.ReopenTasks(ctx, tx, cmd.DID)
		if err != nil {
			return errors.New("could not reopen approval tasks")
		}

		nextTemplateState = contracttemplatestate.Submitted

	} else if processData.State == contracttemplatestate.Submitted.String() {

		if !cmd.UserRoles.HasRoles(userrole.TemplateReviewer, userrole.TemplateManager) {
			return errors.New("invalid user permission")
		}

		isValidReviewer, err := h.RTRepo.IsValidReviewer(ctx, tx, cmd.DID, cmd.SubmittedBy)
		if err != nil {
			return err
		}
		if !isValidReviewer {
			return errors.New("user is not a valid reviewer for that contract template")
		}

		if cmd.ActionFlag != nil {
			switch *cmd.ActionFlag {
			case actionflag.Approval:
				exist, err := h.RTRepo.TaskExistsInState(ctx, tx, processData.DID, cmd.SubmittedBy, reviewtaskstate.Open.String())
				if err != nil {
					return err
				}
				if exist {
					return errors.New("contract template needs to be verified before")
				}
				err = h.RTRepo.UpdateState(ctx, tx, processData.DID, cmd.SubmittedBy, contracttemplatestate.Approved.String())
				if err != nil {
					return fmt.Errorf("could not update review task: %w", err)
				}
				existOpenTasks, err := h.RTRepo.AnyTasksInState(ctx, tx, processData.DID, reviewtaskstate.Open.String(), reviewtaskstate.Verified.String())
				if err != nil {
					return fmt.Errorf("could not check if review task exists: %w", err)
				}
				if !existOpenTasks {
					nextTemplateState = contracttemplatestate.Reviewed
				}

			case actionflag.Draft:
				err = h.RTRepo.ReopenTasks(ctx, tx, cmd.DID)
				if err != nil {
					return err
				}
				err = h.ATRepo.ReopenTasks(ctx, tx, cmd.DID)
				if err != nil {
					return err
				}
				nextTemplateState = contracttemplatestate.Rejected
			}
		} else {
			return errors.New("action flag is missing")
		}

	} else if processData.State == contracttemplatestate.Reviewed.String() {

		if !cmd.UserRoles.HasRoles(userrole.TemplateApprover, userrole.TemplateManager) {
			return errors.New("invalid user permission")
		}

		isValid, err := h.ATRepo.IsValidApprover(ctx, tx, processData.DID, cmd.SubmittedBy)
		if err != nil {
			return err
		}
		if !isValid {
			return errors.New("user is not a valid approver for that contract template")
		}

		err = h.RTRepo.ReopenTasks(ctx, tx, cmd.DID)
		if err != nil {
			return err
		}

		err = h.ATRepo.ReopenTasks(ctx, tx, cmd.DID)
		if err != nil {
			return err
		}

		nextTemplateState = contracttemplatestate.Submitted

	} else {
		return errors.New("current contract template state is invalid")
	}

	if len(nextTemplateState) > 0 && processData.State != nextTemplateState.String() {
		err = h.CTRepo.UpdateState(ctx, tx, cmd.DID, nextTemplateState.String())
		if err != nil {
			return fmt.Errorf("could not update contract template state: %w", err)
		}

		evt := templateevents.SubmitEvent{
			DID:            cmd.DID,
			DocumentNumber: processData.DocumentNumber,
			Version:        processData.Version,
			SubmittedBy:    cmd.SubmittedBy,
			PreviousState:  processData.State,
			NewState:       nextTemplateState.String(),
			ActionFlag:     cmd.ActionFlag,
			Comments:       cmd.Comments,
			OccurredAt:     time.Now().UTC(),
			Responsible:    responsible,
			HolderDID:      cmd.HolderDID,
			UserRoles:      cmd.UserRoles,
		}
		err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
		if err != nil {
			return fmt.Errorf("could not create event: %w", err)
		}
	}

	return tx.Commit()
}
