// Package db holds the contract workflow engine's repository interfaces
// (Postgres implementations in db/pg). Fields such as Approver/Reviewer/
// Negotiator store peer DIDs: task ownership is checked per responsible
// peer (e.g. IsValidApprover), not per individual end user.
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
	RemoteCreate(ctx context.Context, tx *sqlx.Tx, data ApprovalTaskData) error
	RemoteUpdate(ctx context.Context, tx *sqlx.Tx, data ApprovalTaskData) error
	ReopenTasks(ctx context.Context, tx *sqlx.Tx, did string) error
	ReadAllByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]ApprovalTaskData, error)
	ReadAllByApprover(ctx context.Context, tx *sqlx.Tx, approver string) ([]ApprovalTaskData, error)
	ReadAllInState(ctx context.Context, tx *sqlx.Tx, state string) ([]ApprovalTaskData, error)
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, approver string, state string) error
	AnyTasksInState(ctx context.Context, tx *sqlx.Tx, did string, states ...string) (bool, error)
	IsValidApprover(ctx context.Context, tx *sqlx.Tx, did string, approver string) (bool, error)
	TaskExistsInState(ctx context.Context, tx *sqlx.Tx, did string, approver string, state string) (bool, error)
	TaskExists(ctx context.Context, tx *sqlx.Tx, did string) (bool, error)
}
