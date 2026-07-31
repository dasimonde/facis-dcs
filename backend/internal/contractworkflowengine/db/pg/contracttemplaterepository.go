package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"digital-contracting-service/internal/contractworkflowengine/db"

	"github.com/jmoiron/sqlx"
)

type PostgresContractTemplateRepo struct {
}

func (r *PostgresContractTemplateRepo) ReadContractTemplateDataByID(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractTemplateQueryResult, error) {
	statement := `
        SELECT template_data, version, name, description
        FROM contract_templates
        WHERE
            did = $1
            AND template_type = 'CONTRACT_TEMPLATE'
            AND (state = 'REGISTERED' OR state = 'PUBLISHED')
        ORDER BY version DESC
        LIMIT 1
    `
	var result db.ContractTemplateQueryResult
	err := tx.GetContext(ctx, &result, statement, did)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("could not find contract template with DID %q", did)
	case err != nil:
		return nil, err
	}
	return &result, nil
}

func (r *PostgresContractTemplateRepo) ReadAllMetaData(ctx context.Context, tx *sqlx.Tx) ([]db.ContractTemplateMetadata, error) {
	query := `
    WITH ranked_templates AS (
        SELECT did, version, state, template_type, name, description,
               created_by, created_at, updated_at, base_template,
               ROW_NUMBER() OVER (
                   PARTITION BY COALESCE(base_template, did)
                   ORDER BY version DESC
               ) AS rn
        FROM contract_templates
        WHERE state = 'REGISTERED' OR state = 'PUBLISHED'
    )
    SELECT did, version, state, template_type, name, description,
           created_by, created_at, updated_at, base_template
    FROM ranked_templates
    WHERE rn = 1
`
	var cts []db.ContractTemplateMetadata
	err := tx.SelectContext(ctx, &cts, query)
	if err != nil {
		return []db.ContractTemplateMetadata{}, err
	}
	return cts, nil
}
