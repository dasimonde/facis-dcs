package db

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
)

// NegotiationTaskData is one negotiator's outstanding response to one
// negotiation ROUND of one contract. The round is contract_version: every
// accepted redline bumps it and starts a new one, so (did, negotiator,
// contract_version) identifies a task uniquely. Tasks never cross an instance
// boundary (ADR-13) — negotiator is always this instance's own did:web.
type NegotiationTaskData struct {
	ID              string    `db:"id"`
	DID             string    `db:"did"`
	ContractVersion int       `db:"contract_version"`
	State           string    `db:"state"`
	Negotiator      string    `db:"negotiator"`
	CreatedBy       string    `db:"created_by"`
	CreatedAt       time.Time `db:"created_at"`
}

type NegotiationTaskRepo interface {
	// Create mints the round's task and is idempotent: a second mint for the
	// same (did, negotiator, contract_version) writes nothing and reports the
	// existing task's creation time.
	Create(ctx context.Context, tx *sqlx.Tx, data NegotiationTaskData) (*time.Time, error)
	IsValidNegotiator(ctx context.Context, tx *sqlx.Tx, did string, negotiator string, contractVersion int) (bool, error)
	ReopenTasks(ctx context.Context, tx *sqlx.Tx, did string, contractVersion int) error
	// RollForward carries this contract's tasks from one round to the next and
	// reopens them: a party that engaged with round n still owes a response to
	// round n+1. It never creates a task, so a party that never engaged stays
	// without one.
	RollForward(ctx context.Context, tx *sqlx.Tx, did string, fromVersion int, toVersion int) error
	ReadAllByNegotiator(ctx context.Context, tx *sqlx.Tx, negotiator string) ([]NegotiationTaskData, error)
	ReadNegotiatorsForDID(ctx context.Context, tx *sqlx.Tx, did string) ([]string, error)
	UpdateState(ctx context.Context, tx *sqlx.Tx, did string, negotiator string, contractVersion int, state string) error
	AnyTasksInState(ctx context.Context, tx *sqlx.Tx, did string, contractVersion int, states ...string) (bool, error)
}
