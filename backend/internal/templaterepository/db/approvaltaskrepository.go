// Package db holds the template repository's repository interfaces
// (Postgres implementations in db/pg), including the copy-on-version
// template store (base_template lineage, see ContractTemplateRepo.CopyFromDID).
package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type ApprovalTaskData struct {
	ID        string    `db:"id"`
	DID       string    `db:"did"`
	State     string    `db:"state"`
	Approver  string    `db:"approver"`
	CreatedBy string    `db:"created_by"`
	CreatedAt time.Time `db:"created_at"`
}

type ApprovalTaskRepo interface {
	Create(ctx context.Context, tx *sqlx.Tx, data ApprovalTaskData) (*time.Time, error)
	ReopenTasks(ctx context.Context, tx *sqlx.Tx, did string) error
	ReadAllByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]ApprovalTaskData, error)
	ReadAllByApprover(ctx context.Context, tx *sqlx.Tx, approver string) ([]ApprovalTaskData, error)
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, approver string, state string) error
	IsValidApprover(ctx context.Context, tx *sqlx.Tx, did string, approver string) (bool, error)
	TaskExistsInState(ctx context.Context, tx *sqlx.Tx, did string, approver string, state string) (bool, error)
	TaskExists(ctx context.Context, tx *sqlx.Tx, did string) (bool, error)
	Delete(ctx context.Context, tx *sqlx.Tx, did string) error
}
