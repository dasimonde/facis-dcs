package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

type NegotiationTaskData struct {
	ID         string    `db:"id"`
	DID        string    `db:"did"`
	State      string    `db:"state"`
	Negotiator string    `db:"negotiator"`
	CreatedBy  string    `db:"created_by"`
	CreatedAt  time.Time `db:"created_at"`
}

type NegotiationTaskRepo interface {
	Create(ctx context.Context, tx *sqlx.Tx, data NegotiationTaskData) (*time.Time, error)
	RemoteCreate(ctx context.Context, tx *sqlx.Tx, data NegotiationTaskData) error
	RemoteUpdate(ctx context.Context, tx *sqlx.Tx, data NegotiationTaskData) error
	IsValidNegotiator(ctx context.Context, tx *sqlx.Tx, did string, negotiator string) (bool, error)
	ReopenTasks(ctx context.Context, tx *sqlx.Tx, did string) error
	ReadAllByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]NegotiationTaskData, error)
	ReadAllByNegotiator(ctx context.Context, tx *sqlx.Tx, negotiator string) ([]NegotiationTaskData, error)
	ReadNegotiatorsForDID(ctx context.Context, tx *sqlx.Tx, did string) ([]string, error)
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, negotiator string, state string) error
	AnyTasksInState(ctx context.Context, tx *sqlx.Tx, did string, states ...string) (bool, error)
	TaskExistsInState(ctx context.Context, tx *sqlx.Tx, did string, negotiator string, state string) (bool, error)
	TaskExist(ctx context.Context, tx *sqlx.Tx, did string) (bool, error)
}
