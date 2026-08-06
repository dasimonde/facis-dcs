package pg

import (
	"context"
	"database/sql"
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
            did, contract_version, state, negotiator, created_by
        ) VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (did, negotiator, contract_version) DO NOTHING
        RETURNING created_at
    `
	var createdAt time.Time
	err := tx.GetContext(ctx, &createdAt, statement,
		data.DID, data.ContractVersion, data.State, data.Negotiator, data.CreatedBy)
	// DO NOTHING returns no row when the round's task already exists — the mint
	// is idempotent, so the task already on record answers instead.
	if errors.Is(err, sql.ErrNoRows) {
		err = tx.GetContext(ctx, &createdAt, `
            SELECT created_at FROM contract_negotiation_task
            WHERE did = $1 AND negotiator = $2 AND contract_version = $3
        `, data.DID, data.Negotiator, data.ContractVersion)
	}
	if err != nil {
		return nil, err
	}
	return &createdAt, nil
}

func (r *PostgresNegotiationTaskRepo) IsValidNegotiator(ctx context.Context, tx *sqlx.Tx, did string, negotiator string, contractVersion int) (bool, error) {
	query := `
        SELECT COUNT(*) FROM contract_negotiation_task
        WHERE did = $1 AND negotiator = $2 AND contract_version = $3
    `
	var count int
	err := tx.GetContext(ctx, &count, query, did, negotiator, contractVersion)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresNegotiationTaskRepo) ReopenTasks(ctx context.Context, tx *sqlx.Tx, did string, contractVersion int) error {
	statement := `
        UPDATE contract_negotiation_task SET state = 'OPEN'
        WHERE did = $1 AND contract_version = $2
    `
	_, err := tx.ExecContext(ctx, statement, did, contractVersion)
	return err
}

func (r *PostgresNegotiationTaskRepo) RollForward(ctx context.Context, tx *sqlx.Tx, did string, fromVersion int, toVersion int) error {
	statement := `
        UPDATE contract_negotiation_task SET contract_version = $3, state = 'OPEN'
        WHERE did = $1 AND contract_version = $2
    `
	_, err := tx.ExecContext(ctx, statement, did, fromVersion, toVersion)
	return err
}

func (r *PostgresNegotiationTaskRepo) ReadAllByNegotiator(ctx context.Context, tx *sqlx.Tx, negotiator string) ([]db.NegotiationTaskData, error) {
	query := `
        SELECT id, did, contract_version, state, negotiator,
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
        SELECT DISTINCT negotiator
        FROM contract_negotiation_task WHERE did = $1
    `
	var reviewers []string
	err := tx.SelectContext(ctx, &reviewers, query, did)
	if err != nil {
		return nil, err
	}
	return reviewers, nil
}

func (r *PostgresNegotiationTaskRepo) UpdateState(ctx context.Context, tx *sqlx.Tx, did string, negotiator string, contractVersion int, state string) error {
	statement := `
        UPDATE contract_negotiation_task SET state = $4
        WHERE did = $1 AND negotiator = $2 AND contract_version = $3
    `
	result, err := tx.ExecContext(ctx, statement, did, negotiator, contractVersion, state)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("user has no negotiation task for this negotiation round")
	}
	return nil
}

func (r *PostgresNegotiationTaskRepo) AnyTasksInState(ctx context.Context, tx *sqlx.Tx, did string, contractVersion int, states ...string) (bool, error) {
	placeholders := make([]string, len(states))
	args := []interface{}{did, contractVersion}

	for i, s := range states {
		placeholders[i] = fmt.Sprintf("$%d", i+3)
		args = append(args, s)
	}

	query := fmt.Sprintf(`
        SELECT COUNT(*)
        FROM contract_negotiation_task
        WHERE did = $1 AND contract_version = $2 AND state IN (%s)
    `, strings.Join(placeholders, ", "))

	var count int
	err := tx.GetContext(ctx, &count, query, args...)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
