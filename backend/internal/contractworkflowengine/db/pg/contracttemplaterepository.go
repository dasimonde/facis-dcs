package pg

import (
	"context"
	"database/sql"
	"digital-contracting-service/internal/base/datatype"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type PostgresContractTemplateRepo struct {
}

func (r *PostgresContractTemplateRepo) ReadFrameContractTemplateDataByID(ctx context.Context, tx *sqlx.Tx, did string) (*datatype.JSON, error) {
	statement := `
        SELECT template_data
        FROM contract_templates
        WHERE
            did = $1
            AND template_type = 'FRAME_CONTRACT'
            AND (state = 'APPROVED' OR state = 'PUBLISHED')
        LIMIT 1
    `
	var templateData datatype.JSON
	err := tx.GetContext(ctx, &templateData, statement, did)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return nil, fmt.Errorf("could not find frame contract template with DID %q", did)
	case err != nil:
		return nil, err
	}
	return &templateData, nil
}
