package pg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/contractworkflowengine/db"
)

type PostgresNegotiationTaskRepo struct {
}

func (r *PostgresNegotiationTaskRepo) Create(ctx context.Context, tx *sqlx.Tx, data db.NegotiationTaskData) (*time.Time, error) {
	statement := `
        INSERT INTO contract_negotiation_task (
            did, state, negotiator, created_by
        ) VALUES ($1, $2, $3, $4)
        RETURNING created_at
    `
	var createdAt time.Time
	err := tx.GetContext(ctx, &createdAt, statement,
		data.DID, data.State, data.Negotiator, data.CreatedBy)
	if err != nil {
		return nil, err
	}
	return &createdAt, nil
}

func (r *PostgresNegotiationTaskRepo) RemoteCreate(ctx context.Context, tx *sqlx.Tx, data db.NegotiationTaskData) error {
	statement := `
        INSERT INTO contract_negotiation_task (
            id, did, state, negotiator, created_by, created_at
        ) VALUES ($1, $2, $3, $4, $5, $6)
    `
	_, err := tx.ExecContext(ctx, statement,
		data.ID, data.DID, data.State, data.Negotiator, data.CreatedBy, data.CreatedAt)
	return err
}

func (r *PostgresNegotiationTaskRepo) RemoteUpdate(ctx context.Context, tx *sqlx.Tx, data db.NegotiationTaskData) error {
	statement := `
        INSERT INTO contract_negotiation_task (
            id, did, state, negotiator, created_by, created_at
        ) VALUES ($1, $2, $3, $4, $5, $6)
        ON CONFLICT (id) DO UPDATE SET
            state = EXCLUDED.state,
            negotiator = EXCLUDED.negotiator
    `
	_, err := tx.ExecContext(ctx, statement,
		data.ID, data.DID, data.State, data.Negotiator, data.CreatedBy, data.CreatedAt)
	return err
}

func (r *PostgresNegotiationTaskRepo) IsValidNegotiator(ctx context.Context, tx *sqlx.Tx, did string, negotiator string) (bool, error) {
	query := `
        SELECT COUNT(*) FROM contract_negotiation_task
        WHERE did = $1 AND negotiator = $2
    `
	var count int
	err := tx.GetContext(ctx, &count, query, did, negotiator)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresNegotiationTaskRepo) ReopenTasks(ctx context.Context, tx *sqlx.Tx, did string) error {
	statement := `
        UPDATE contract_negotiation_task SET state = 'OPEN'
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}

func (r *PostgresNegotiationTaskRepo) ReadAllByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]db.NegotiationTaskData, error) {
	query := `
        SELECT id, did, state, negotiator,
               created_by, created_at
        FROM contract_negotiation_task WHERE did = $1
    `
	var negotiationTasks []db.NegotiationTaskData
	err := tx.SelectContext(ctx, &negotiationTasks, query, did)
	if err != nil {
		return nil, err
	}
	return negotiationTasks, nil
}

func (r *PostgresNegotiationTaskRepo) ReadAllByNegotiator(ctx context.Context, tx *sqlx.Tx, negotiator string) ([]db.NegotiationTaskData, error) {
	query := `
        SELECT id, did, state, negotiator,
               created_by, created_at
        FROM contract_negotiation_task WHERE negotiator = $1
    `
	var negotiationTasks []db.NegotiationTaskData
	err := tx.SelectContext(ctx, &negotiationTasks, query, negotiator)
	if err != nil {
		return nil, err
	}
	return negotiationTasks, nil
}

func (r *PostgresNegotiationTaskRepo) ReadNegotiatorsForDID(ctx context.Context, tx *sqlx.Tx, did string) ([]string, error) {
	query := `
        SELECT negotiator
        FROM contract_negotiation_task WHERE did = $1
    `
	var reviewers []string
	err := tx.SelectContext(ctx, &reviewers, query, did)
	if err != nil {
		return nil, err
	}
	return reviewers, nil
}

func (r *PostgresNegotiationTaskRepo) UpdateState(ctx context.Context, tx *sqlx.Tx, did string, negotiator string, state string) error {
	statement := `
        UPDATE contract_negotiation_task SET state = $3
        WHERE did = $1 AND negotiator = $2
    `
	result, err := tx.ExecContext(ctx, statement, did, negotiator, state)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("user has no negotiation task for this contract")
	}
	return nil
}

func (r *PostgresNegotiationTaskRepo) AnyTasksInState(ctx context.Context, tx *sqlx.Tx, did string, states ...string) (bool, error) {
	placeholders := make([]string, len(states))
	args := []interface{}{did}

	for i, s := range states {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, s)
	}

	query := fmt.Sprintf(`
        SELECT COUNT(*) 
        FROM contract_negotiation_task 
        WHERE did = $1 AND state IN (%s)
    `, strings.Join(placeholders, ", "))

	var count int
	err := tx.GetContext(ctx, &count, query, args...)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresNegotiationTaskRepo) TaskExistsInState(ctx context.Context, tx *sqlx.Tx, did string, negotiator string, state string) (bool, error) {
	query := `
        SELECT COUNT(*) 
        FROM contract_negotiation_task 
        WHERE did = $1 AND negotiator = $2 AND state = $3
    `
	var count int
	err := tx.GetContext(ctx, &count, query, did, negotiator, state)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresNegotiationTaskRepo) TaskExist(ctx context.Context, tx *sqlx.Tx, did string) (bool, error) {
	query := `
        SELECT COUNT(*) 
        FROM contract_negotiation_task 
        WHERE did = $1
    `
	var count int
	err := tx.GetContext(ctx, &count, query, did)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
