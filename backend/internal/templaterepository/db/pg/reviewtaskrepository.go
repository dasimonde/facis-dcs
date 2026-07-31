package pg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/templaterepository/db"
)

type PostgresReviewTaskRepo struct {
}

func (r *PostgresReviewTaskRepo) Create(ctx context.Context, tx *sqlx.Tx, data db.ReviewTaskData) (*time.Time, error) {
	statement := `
        INSERT INTO contract_templates_review_task (
            did, state, reviewer, created_by
        ) VALUES ($1, $2, $3, $4)
        RETURNING created_at
    `
	var createdAt time.Time
	err := tx.GetContext(ctx, &createdAt, statement,
		data.DID,
		data.State, data.Reviewer, data.CreatedBy,
	)
	if err != nil {
		return nil, err
	}
	return &createdAt, nil
}

func (r *PostgresReviewTaskRepo) IsValidReviewer(ctx context.Context, tx *sqlx.Tx, did string, reviewer string) (bool, error) {
	query := `
        SELECT COUNT(*) FROM contract_templates_review_task
        WHERE did = $1 AND reviewer = $2
    `
	var count int
	err := tx.GetContext(ctx, &count, query, did, reviewer)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresReviewTaskRepo) UpdateStateForAllTasks(ctx context.Context, tx *sqlx.Tx, did string, state string) error {
	statement := `
        UPDATE contract_templates_review_task SET state = $2
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did, state)
	return err
}

func (r *PostgresReviewTaskRepo) ReopenTasks(ctx context.Context, tx *sqlx.Tx, did string) error {
	statement := `
        UPDATE contract_templates_review_task SET state = 'OPEN'
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}

func (r *PostgresReviewTaskRepo) ReadAllByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]db.ReviewTaskData, error) {
	query := `
        SELECT id, did, state, reviewer,
               created_by, created_at
        FROM contract_templates_review_task WHERE did = $1
    `
	var reviewTasks []db.ReviewTaskData
	err := tx.SelectContext(ctx, &reviewTasks, query, did)
	if err != nil {
		return nil, err
	}
	return reviewTasks, nil
}

func (r *PostgresReviewTaskRepo) ReadAllByReviewer(ctx context.Context, tx *sqlx.Tx, reviewer string) ([]db.ReviewTaskData, error) {
	query := `
        SELECT id, did, state, reviewer,
               created_by, created_at
        FROM contract_templates_review_task WHERE reviewer = $1
    `
	var reviewTasks []db.ReviewTaskData
	err := tx.SelectContext(ctx, &reviewTasks, query, reviewer)
	if err != nil {
		return nil, err
	}
	return reviewTasks, nil
}

func (r *PostgresReviewTaskRepo) UpdateState(ctx context.Context, tx *sqlx.Tx, did string, reviewer string, state string) error {
	statement := `
        UPDATE contract_templates_review_task SET state = $3
        WHERE did = $1 AND reviewer = $2
    `
	result, err := tx.ExecContext(ctx, statement, did, reviewer, state)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("user has no review task for this contract template")
	}
	return nil
}

func (r *PostgresReviewTaskRepo) AnyTasksInState(ctx context.Context, tx *sqlx.Tx, did string, states ...string) (bool, error) {
	placeholders := make([]string, len(states))
	args := []interface{}{did}

	for i, s := range states {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, s)
	}

	query := fmt.Sprintf(`
        SELECT COUNT(*) 
        FROM contract_templates_review_task 
        WHERE did = $1 AND state IN (%s)
    `, strings.Join(placeholders, ", "))

	var count int
	err := tx.GetContext(ctx, &count, query, args...)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresReviewTaskRepo) TaskExistsInState(ctx context.Context, tx *sqlx.Tx, did string, reviewer string, state string) (bool, error) {
	query := `
        SELECT COUNT(*) 
        FROM contract_templates_review_task 
        WHERE did = $1 AND reviewer = $2 AND state = $3
    `
	var count int
	err := tx.GetContext(ctx, &count, query, did, reviewer, state)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresReviewTaskRepo) TaskExist(ctx context.Context, tx *sqlx.Tx, did string) (bool, error) {
	query := `
        SELECT COUNT(*) 
        FROM contract_templates_review_task 
        WHERE did = $1
    `
	var count int
	err := tx.GetContext(ctx, &count, query, did)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *PostgresReviewTaskRepo) Delete(ctx context.Context, tx *sqlx.Tx, did string) error {
	statement := `
        DELETE FROM contract_templates_review_task
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}
