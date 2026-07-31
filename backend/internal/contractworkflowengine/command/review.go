package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"digital-contracting-service/internal/base/datatype/userrole"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/db"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"

	"github.com/jmoiron/sqlx"
)

type ReviewCmd struct {
	DID        string             `json:"did"`
	ReviewedBy string             `json:"reviewed_by"`
	HolderDID  string             `json:"holder_did"`
	UserRoles  userrole.UserRoles `json:"user_roles"`
}

type Reviewer struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

// Handle does not return or mutate any contract data — despite being wired
// to a GET endpoint (design/contract_workflow_engine.go "review"), this is a
// pure audit-trail write: it records that the latest draft was opened for
// review. Use RetrieveByID to actually fetch contract data. It therefore has
// no entry in contractstate.Transitions: there is no contract state to gate
// on, since Review never calls ContractRepo.UpdateState.
func (h *Reviewer) Handle(ctx context.Context, cmd ReviewCmd) error {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	evt := contractevents.ReviewEvent{
		DID:        cmd.DID,
		ReviewedBy: cmd.ReviewedBy,
		HolderDID:  cmd.HolderDID,
		UserRoles:  cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractWorkflowEngine)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
