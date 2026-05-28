package pg

import (
	"context"
	"database/sql"
	"digital-contracting-service/internal/templaterepository/db"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

type PostgresContractTemplateRepo struct {
}

func (r *PostgresContractTemplateRepo) CopyFromDID(ctx context.Context, tx *sqlx.Tx, did string, copyDID string) (int, error) {
	statement := `
        INSERT INTO contract_templates 
            (did, document_number, version, state, template_type, name, description, created_by, created_at, updated_at, 
             responsible_persons, template_data)
        SELECT 
            $1,
            document_number,
            CASE 
                WHEN state IN ('APPROVED', 'PUBLISHED') THEN version + 1
                ELSE 1
            END,
            'DRAFT', template_type, name, description, created_by, NOW(), NOW(), 
            responsible_persons, template_data
        FROM contract_templates 
        WHERE did = $2
        RETURNING version
    `
	var newVersion int
	err := tx.QueryRowContext(ctx, statement, copyDID, did).Scan(&newVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("template with did %s not found", did)
		}
		return 0, err
	}
	return newVersion, nil
}

func (r *PostgresContractTemplateRepo) CreateHistoryEntryForDID(ctx context.Context, tx *sqlx.Tx, did string) error {
	statement := `
        INSERT INTO contract_templates_history 
            (did, document_number, version, state, template_type, name, description, created_by, created_at, updated_at, 
             responsible_persons, template_data)
        SELECT 
            did, document_number, version, state, template_type, name, description, created_by, created_at, updated_at, 
            responsible_persons, template_data
        FROM contract_templates 
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}

func (r *PostgresContractTemplateRepo) ReadHistoryByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]db.ContractTemplateHistory, error) {
	query := `
        SELECT did, document_number, version, state, name, description,
               created_by, created_at, updated_at, template_data, template_type, responsible_persons
        FROM contract_templates_history WHERE did = $1
    `
	var ct []db.ContractTemplateHistory
	err := tx.SelectContext(ctx, &ct, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []db.ContractTemplateHistory{}, fmt.Errorf("template contract with DID %s not found", did)
		}
		return []db.ContractTemplateHistory{}, err
	}
	return ct, nil
}

func (r *PostgresContractTemplateRepo) Create(ctx context.Context, tx *sqlx.Tx, data db.ContractTemplate) (*time.Time, error) {
	statement := `
        INSERT INTO contract_templates (
            did, document_number, created_by, state, name,
            description, template_data, template_type
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
        RETURNING created_at
    `
	var createdAt time.Time
	err := tx.GetContext(ctx, &createdAt, statement,
		data.DID, data.DocumentNumber, data.CreatedBy, data.State, data.Name,
		data.Description, data.TemplateData, data.TemplateType,
	)
	if err != nil {
		return nil, err
	}
	return &createdAt, nil
}

func (r *PostgresContractTemplateRepo) ReadDataByID(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractTemplate, error) {
	query := `
        SELECT did, document_number, version, state, name, description,
               created_by, created_at, updated_at, template_data, template_type, responsible_persons
        FROM contract_templates WHERE did = $1
    `
	var ct db.ContractTemplate
	err := tx.GetContext(ctx, &ct, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: did=%s", db.ErrContractTemplateNotFound, did)
		}
		return nil, err
	}
	return &ct, nil
}

func (r *PostgresContractTemplateRepo) ReadAllMetaData(ctx context.Context, tx *sqlx.Tx) ([]db.ContractTemplateMetadata, error) {
	query := `
        SELECT did, document_number, version, state, template_type, name, description, created_by, created_at, updated_at, responsible_persons
        FROM contract_templates
    `
	var cts []db.ContractTemplateMetadata
	err := tx.SelectContext(ctx, &cts, query)
	if err != nil {
		return []db.ContractTemplateMetadata{}, err
	}
	return cts, nil
}

func (r *PostgresContractTemplateRepo) ReadAllMetaDataByFilter(ctx context.Context, tx *sqlx.Tx, values db.SearchValues) ([]db.ContractTemplateMetadata, error) {
	query := `
        SELECT did, document_number, version, state, name, template_type, description, created_by, created_at, updated_at, responsible_persons
        FROM contract_templates
    `
	conditions, params, err := createSearchConditions(values)
	if err != nil {
		return nil, err
	}
	if len(params) > 0 {
		query += " WHERE " + *conditions
	}

	var cts []db.ContractTemplateMetadata
	err = tx.SelectContext(ctx, &cts, query, params...)
	if err != nil {
		return []db.ContractTemplateMetadata{}, err
	}
	return cts, nil
}

func (r *PostgresContractTemplateRepo) ReadProcessData(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractTemplateProcessData, error) {
	query := `
        SELECT did, document_number, version, state, updated_at, created_by
        FROM contract_templates WHERE did = $1
    `
	var processData db.ContractTemplateProcessData
	err := tx.GetContext(ctx, &processData, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("contract template with DID %s", did)
		}
		return nil, err
	}
	return &processData, nil
}

func (r *PostgresContractTemplateRepo) UpdateStateForAllTasks(ctx context.Context, tx *sqlx.Tx, did string, state string) error {
	statement := `
        UPDATE contract_templates SET state = $2
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did, state)
	return err
}

func (r *PostgresContractTemplateRepo) UpdateState(ctx context.Context, tx *sqlx.Tx, did string, state string) error {
	statement := `
        UPDATE contract_templates SET state = $2
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did, state)
	return err
}

func (r *PostgresContractTemplateRepo) Update(ctx context.Context, tx *sqlx.Tx, data db.ContractTemplateUpdateData) error {
	query, params, err := createQuery(data)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, *query, params...)
	return err
}

func createSearchConditions(values db.SearchValues) (*string, []interface{}, error) {
	conditions := ""
	var params []interface{}
	paramIndex := 1

	if len(values.DID) > 0 {
		conditions += ` did = $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, values.DID)
		paramIndex++
	}
	if len(values.DocumentNumber) > 0 {
		conditions += ` document_number = $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, values.DocumentNumber)
		paramIndex++
	}
	if values.Version > 0 {
		conditions += ` version = $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, values.Version)
		paramIndex++
	}
	if len(values.State) > 0 {
		conditions += ` state = $` + strconv.Itoa(paramIndex) + ` AND`
		state := strings.ToUpper(values.State)
		params = append(params, state)
		paramIndex++
	}
	if len(values.TemplateType) > 0 {
		conditions += ` template_type = $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, "%"+values.TemplateType+"%")
		paramIndex++
	}
	if len(values.Name) > 0 {
		conditions += ` name ILIKE $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, "%"+values.Name+"%")
		paramIndex++
	}
	if len(values.Description) > 0 {
		conditions += ` description ILIKE $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, "%"+values.Description+"%")
		paramIndex++
	}
	if len(values.TemplateData) > 0 {
		conditions += ` search_vector @@ plainto_tsquery('english', $` + strconv.Itoa(paramIndex) + `) AND`
		params = append(params, values.TemplateData)
		paramIndex++
	}
	l := len(" AND")
	if len(conditions) > l {
		conditions = conditions[:len(conditions)-l]
	}

	return &conditions, params, nil
}

func createQuery(data db.ContractTemplateUpdateData) (*string, []interface{}, error) {
	queryBase := `UPDATE contract_templates SET `
	var columns []string
	var params []interface{}

	addParam := func(columnName string, value interface{}) {
		columns = append(columns, fmt.Sprintf("%s = $%d", columnName, len(params)+1))
		params = append(params, value)
	}

	if data.DocumentNumber != nil && len(*data.DocumentNumber) > 0 {
		addParam("document_number", data.DocumentNumber)
	}
	if len(data.State) > 0 {
		addParam("state", data.State)
	}
	if data.Name != nil {
		addParam("name", data.Name)
	}
	if data.Description != nil {
		addParam("description", data.Description)
	}
	if data.TemplateData != nil && data.TemplateData.IsNotNullValue() {
		addParam("template_data", data.TemplateData)
	}
	if len(data.TemplateType) > 0 {
		addParam("template_type", data.TemplateType)
	}
	if data.ResponsiblePersons != nil {
		addParam("responsible_persons", data.ResponsiblePersons)
	}
	if len(columns) == 0 {
		return nil, nil, errors.New("no fields to update")
	}

	fullQuery := queryBase + strings.Join(columns, ", ")
	nextIdx := len(params) + 1
	fullQuery += fmt.Sprintf(" WHERE did = $%d;",
		nextIdx)
	params = append(params, data.DID)

	return &fullQuery, params, nil
}
