package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type ReviewTaskData struct {
	ID        string    `db:"id"`
	DID       string    `db:"did"`
	State     string    `db:"state"`
	Reviewer  string    `db:"reviewer"`
	CreatedBy string    `db:"created_by"`
	CreatedAt time.Time `db:"created_at"`
}

type ReviewTaskRepo interface {
	Create(ctx context.Context, tx *sqlx.Tx, data ReviewTaskData) (*time.Time, error)
	IsValidReviewer(ctx context.Context, tx *sqlx.Tx, did string, reviewer string) (bool, error)
	ReopenTasks(ctx context.Context, tx *sqlx.Tx, did string) error
	ReadAllByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]ReviewTaskData, error)
	ReadAllByReviewer(ctx context.Context, tx *sqlx.Tx, reviewer string) ([]ReviewTaskData, error)
	UpdateStateForAllTasks(ctx context.Context, tx *sqlx.Tx, did string, state string) error
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, reviewer string, state string) error
	AnyTasksInState(ctx context.Context, tx *sqlx.Tx, did string, states ...string) (bool, error)
	TaskExistsInState(ctx context.Context, tx *sqlx.Tx, did string, reviewer string, state string) (bool, error)
	TaskExist(ctx context.Context, tx *sqlx.Tx, did string) (bool, error)
	Delete(ctx context.Context, tx *sqlx.Tx, did string) error
}
