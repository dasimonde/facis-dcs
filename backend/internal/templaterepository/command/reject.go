package command

import (
	"context"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/templaterepository/datatype/approvaltaskstate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type RejectCmd struct {
	DID        string
	UpdatedAt  time.Time
	RejectedBy string
	Reason     string
}

type Rejecter struct {
	Ctx    context.Context
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
	RTRepo db.ReviewTaskRepo
	ATRepo db.ApprovalTaskRepo
}

func (h *Rejecter) Handle(cmd RejectCmd) error {

	ctx, cancel := context.WithTimeout(h.Ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer tx.Rollback()

	processData, err := h.CTRepo.ReadProcessData(tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		return errors.New("contract template was updated elsewhere, please reload")
	}

	if processData.State != contracttemplatestate.Reviewed.String() {
		return errors.New("invalid contract template state")
	}

	exist, err := h.ATRepo.IsValidApprover(tx, cmd.DID, cmd.RejectedBy)
	if err != nil {
		return err
	}

	if !exist {
		return errors.New("invalid user")
	}

	err = h.ATRepo.UpdateState(tx, cmd.DID, cmd.RejectedBy, approvaltaskstate.Rejected.String())
	if err != nil {
		return fmt.Errorf("could not update approval task state: %w", err)
	}

	err = h.CTRepo.UpdateState(tx, cmd.DID, contracttemplatestate.Rejected.String())
	if err != nil {
		return fmt.Errorf("could not update current template state: %w", err)
	}

	evt := templateevents.RejectEvent{
		DID:            cmd.DID,
		DocumentNumber: processData.DocumentNumber,
		Version:        processData.Version,
		RejectedBy:     cmd.RejectedBy,
		Reason:         cmd.Reason,
		OccurredAt:     time.Now(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
