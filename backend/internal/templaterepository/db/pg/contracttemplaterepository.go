// Package pg is the Postgres implementation of the template repository's
// repository interfaces. CopyFromDID and ReadAllMetaData below implement
// the copy-on-version scheme described in ADR-0006: every template version
// is its own row/DID, chained via base_template, rather than a single
// mutable row with a version counter (contrast contractworkflowengine's
// contracts table).
package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"digital-contracting-service/internal/base/datatype"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/templaterepository/db"
)

type PostgresContractTemplateRepo struct {
}

// CopyFromDID creates copyDID as a copy of the source row did (note: did is
// the source and copyDID is the destination — inverted relative to the
// Copy command's own CopyDID/NewDID field names). The SQL CASE below
// decides versioning: if the source is not yet REGISTERED/PUBLISHED, the
// copy gets version=1 and starts its own base_template lineage (pointing at
// itself); if the source already is REGISTERED/PUBLISHED, the copy inherits
// the source's base_template and gets version+1. The WHERE/NOT EXISTS guard
// prevents two competing REGISTERED/PUBLISHED rows at the same version
// within one lineage.
func (r *PostgresContractTemplateRepo) CopyFromDID(ctx context.Context, tx *sqlx.Tx, did string, copyDID string) (int, error) {
	var exists bool
	if err := tx.GetContext(ctx, &exists, `SELECT EXISTS(SELECT 1 FROM contract_templates WHERE did = $1)`, did); err != nil {
		return 0, err
	}
	if !exists {
		return 0, fmt.Errorf("template with did %s not found", did)
	}

	statement := `
		WITH source AS (
			SELECT
				did, document_number, template_type, name, description, created_by, template_data,
				CASE
					WHEN state IN ('REGISTERED', 'PUBLISHED') THEN version + 1
					ELSE 1
				END AS new_version,
				CASE
					WHEN state NOT IN ('REGISTERED', 'PUBLISHED') THEN $1
					ELSE base_template
				END AS new_base_template
			FROM contract_templates
			WHERE did = $2
		)
		INSERT INTO contract_templates
			(did, document_number, version, state, template_type, name, description, created_by, created_at, updated_at,
			 template_data, base_template)
		SELECT
			$1,
			source.document_number,
			source.new_version,
			'DRAFT', source.template_type, source.name, source.description, source.created_by, NOW(), NOW(),
			source.template_data, source.new_base_template
		FROM source
		WHERE source.new_base_template IS NULL
		   OR NOT EXISTS (
			   SELECT 1
			   FROM contract_templates ct
			   WHERE ct.base_template = source.new_base_template
				 AND ct.version = source.new_version
				 AND ct.state IN ('REGISTERED', 'PUBLISHED')
		   )
		RETURNING version
	`
	var newVersion int
	err := tx.QueryRowContext(ctx, statement, copyDID, did).Scan(&newVersion)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("could not create new version for did %s: a template with the same version already exists in state REGISTERED or PUBLISHED", did)
		}
		return 0, err
	}
	return newVersion, nil
}

func (r *PostgresContractTemplateRepo) CreateHistoryEntryForDID(ctx context.Context, tx *sqlx.Tx, did string) error {
	statement := `
        INSERT INTO contract_templates_history 
            (did, document_number, version, state, template_type, name, description, created_by, created_at, updated_at, 
             template_data, base_template)
        SELECT 
            did, document_number, version, state, template_type, name, description, created_by, created_at, updated_at, 
            template_data, base_template
        FROM contract_templates 
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}

func (r *PostgresContractTemplateRepo) ReadHistoryByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]db.ContractTemplateHistory, error) {
	query := `
        SELECT did, document_number, version, state, name, description,
               created_by, created_at, updated_at, template_data, template_type, base_template
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
            description, template_data, template_type, base_template
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        RETURNING created_at
    `
	var createdAt time.Time
	err := tx.GetContext(ctx, &createdAt, statement,
		data.DID, data.DocumentNumber, data.CreatedBy, data.State, data.Name,
		data.Description, data.TemplateData, data.TemplateType, data.DID,
	)
	if err != nil {
		return nil, err
	}
	return &createdAt, nil
}

func (r *PostgresContractTemplateRepo) ReadDataByID(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractTemplate, error) {
	query := `
        SELECT did, document_number, version, state, name, description,
               created_by, created_at, updated_at, template_data, template_type,
               base_template
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

// ReadAllMetaData computes "is this the latest version?" at read time via a
// window function over each base_template lineage, rather than storing a
// flag: outdated is true for any REGISTERED/PUBLISHED row that isn't the
// highest version in its lineage, and latest_did then points readers to the
// actual newest version.
func (r *PostgresContractTemplateRepo) ReadAllMetaData(ctx context.Context, tx *sqlx.Tx, pagination datatype.Pagination) ([]db.ContractTemplateMetadata, error) {
	query := `
		WITH latest AS (
			SELECT DISTINCT ON (base_template)
				base_template,
				did     AS latest_did,
				version AS latest_version
			FROM contract_templates
			WHERE state IN ('REGISTERED', 'PUBLISHED')
			ORDER BY base_template, version DESC
		)
		SELECT
			t.did, t.document_number, t.version, t.state, t.template_type, t.name,
			t.description, t.created_by, t.created_at, t.updated_at,
			t.base_template,
			CASE
				WHEN t.state NOT IN ('REGISTERED', 'PUBLISHED') THEN FALSE
				ELSE t.version <> l.latest_version
			END AS outdated,
			CASE
				WHEN t.did != l.latest_did AND t.state IN ('REGISTERED', 'PUBLISHED') THEN l.latest_did
				ELSE NULL
			END AS latest_did
		FROM contract_templates t
		LEFT JOIN latest l ON l.base_template = t.base_template
	`

	var params []any
	if pagination.Limit > 0 {
		offset := (pagination.Offset - 1) * pagination.Limit
		query += ` ORDER BY created_at DESC LIMIT $1 OFFSET $2`
		params = append(params, pagination.Limit, offset)
	}

	var cts []db.ContractTemplateMetadata
	err := tx.SelectContext(ctx, &cts, query, params...)
	if err != nil {
		return []db.ContractTemplateMetadata{}, err
	}
	return cts, nil
}

func (r *PostgresContractTemplateRepo) ReadAllMetaDataByFilter(ctx context.Context, tx *sqlx.Tx, values db.SearchValues, pagination datatype.Pagination) ([]db.ContractTemplateMetadata, error) {
	query := `
        SELECT did, document_number, version, state, name, template_type, description,
               created_by, created_at, updated_at, base_template
        FROM contract_templates
    `

	conditions, params, err := createSearchConditions(values)
	if err != nil {
		return nil, err
	}
	if len(params) > 0 {
		query += " WHERE " + *conditions
	}

	if pagination.Limit > 0 {
		offset := (pagination.Offset - 1) * pagination.Limit
		n := len(params) + 1
		query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d OFFSET $%d", n, n+1)
		params = append(params, pagination.Limit, offset)
	}

	var cts []db.ContractTemplateMetadata
	err = tx.SelectContext(ctx, &cts, query, params...)
	if err != nil {
		return nil, err
	}
	return cts, nil
}

func (r *PostgresContractTemplateRepo) ReadProcessDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractTemplateProcessData, error) {
	query := `
        SELECT did, document_number, version, state, updated_at, content_updated_at, created_by
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
        UPDATE contract_templates ct
        SET state = $2::contract_template_state
        WHERE did = $1
          AND (
              $2::contract_template_state NOT IN ('REGISTERED', 'PUBLISHED')
              OR NOT EXISTS (
                  SELECT 1
                  FROM contract_templates other
                  WHERE other.version = ct.version
                    AND other.base_template = ct.base_template
                    AND other.state IN ('REGISTERED', 'PUBLISHED')
                    AND other.did <> ct.did
              )
          )
    `
	result, err := tx.ExecContext(ctx, statement, did, state)
	if err != nil {
		return fmt.Errorf("could not update registered state: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("could not update state for DID %q: not found or conflicting version already active", did)
	}
	return nil
}

func (r *PostgresContractTemplateRepo) ReadPDFState(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractTemplatePDFState, error) {
	var state db.ContractTemplatePDFState
	err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(pdf_ipfs_cid,''), COALESCE(pdf_renderer_version,''), COALESCE(pdf_c2pa_state,''), COALESCE(pdf_payload_hash,'') FROM contract_templates WHERE did=$1`, did,
	).Scan(&state.IPFSCID, &state.RendererVersion, &state.C2PAState, &state.PayloadHash)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (r *PostgresContractTemplateRepo) UpdatePDFState(ctx context.Context, tx *sqlx.Tx, did string, data db.ContractTemplatePDFState) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE contract_templates SET pdf_ipfs_cid=$1, pdf_renderer_version=$2, pdf_c2pa_state=$3, pdf_payload_hash=$4 WHERE did=$5`,
		data.IPFSCID, data.RendererVersion, data.C2PAState, data.PayloadHash, did,
	)
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

	contentChanged := false
	if data.DocumentNumber != nil && len(*data.DocumentNumber) > 0 {
		addParam("document_number", data.DocumentNumber)
		contentChanged = true
	}
	if len(data.State) > 0 {
		addParam("state", data.State)
	}
	if data.Name != nil {
		addParam("name", data.Name)
		contentChanged = true
	}
	if data.Description != nil {
		addParam("description", data.Description)
		contentChanged = true
	}
	if data.TemplateData != nil && data.TemplateData.IsNotNullValue() {
		addParam("template_data", data.TemplateData)
		contentChanged = true
	}
	if len(data.TemplateType) > 0 {
		addParam("template_type", data.TemplateType)
		contentChanged = true
	}
	if len(columns) == 0 {
		return nil, nil, errors.New("no fields to update")
	}

	// Invalidate the cached PDF only when rendered content changes.
	// Pure state transitions (submit, approve, etc.) must NOT clear pdf_ipfs_cid
	// because the C2PA chain logic relies on the prior CID to append the next manifest.
	if contentChanged {
		columns = append(columns,
			"pdf_ipfs_cid = NULL",
			"pdf_renderer_version = NULL",
		)
	}

	fullQuery := queryBase + strings.Join(columns, ", ")
	nextIdx := len(params) + 1
	fullQuery += fmt.Sprintf(" WHERE did = $%d;",
		nextIdx)
	params = append(params, data.DID)

	return &fullQuery, params, nil
}

// InsertProvenanceCredential stores one registered version's signed
// provenance VC (DCS-FR-TR-09). The UNIQUE (did, version) constraint makes a
// duplicate registration of the same version a hard error, not a silent
// overwrite of issued evidence.
func (r *PostgresContractTemplateRepo) InsertProvenanceCredential(ctx context.Context, tx *sqlx.Tx, data db.TemplateProvenanceCredential) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO template_provenance_credentials (did, version, vc_id, previous_vc_id, credential)
		 VALUES ($1, $2, $3, $4, $5)`,
		data.DID, data.Version, data.VCID, data.PreviousVCID, data.Credential,
	)
	if err != nil {
		return fmt.Errorf("insert template provenance credential for %s v%d: %w", data.DID, data.Version, err)
	}
	return nil
}

// ReadProvenanceCredentials returns all issued provenance credentials for a
// template, oldest version first — the walkable version history.
func (r *PostgresContractTemplateRepo) ReadProvenanceCredentials(ctx context.Context, tx *sqlx.Tx, did string) ([]db.TemplateProvenanceCredential, error) {
	var rows []db.TemplateProvenanceCredential
	err := tx.SelectContext(ctx, &rows,
		`SELECT did, version, vc_id, previous_vc_id, credential, created_at
		   FROM template_provenance_credentials
		  WHERE did = $1
		  ORDER BY version ASC`,
		did,
	)
	if err != nil {
		return nil, fmt.Errorf("read template provenance credentials for %s: %w", did, err)
	}
	return rows, nil
}

// ReadLatestProvenanceVCID returns the newest issued credential's vc_id for
// linkage, or nil when this is the first registered version.
func (r *PostgresContractTemplateRepo) ReadLatestProvenanceVCID(ctx context.Context, tx *sqlx.Tx, did string) (*string, error) {
	var vcID string
	err := tx.GetContext(ctx, &vcID,
		`SELECT vc_id
		   FROM template_provenance_credentials
		  WHERE did = $1
		  ORDER BY version DESC
		  LIMIT 1`,
		did,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read latest template provenance vc_id for %s: %w", did, err)
	}
	return &vcID, nil
}
