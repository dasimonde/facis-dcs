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
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/semantichub"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

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
				did, template_type, party_type, name, description, created_by, updated_by, template_data, search_projection,
				template_level, required_templates,
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
			(did, version, state, template_type, party_type, name, description, created_by, updated_by, created_at, updated_at,
			 template_data, search_projection, base_template, template_level, required_templates)
		SELECT
			$1,
			source.new_version,
			'DRAFT', source.template_type, source.party_type, source.name, source.description, source.created_by, source.updated_by, NOW(), NOW(),
			source.template_data, source.search_projection, source.new_base_template,
			source.template_level, source.required_templates
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
            (did, version, state, template_type, party_type, name, description, created_by, updated_by, created_at, updated_at,
             template_data, base_template, compliance_status, template_level, required_templates)
        SELECT
            did, version, state, template_type, party_type, name, description, created_by, updated_by, created_at, updated_at,
            template_data, base_template, compliance_status, template_level, required_templates
        FROM contract_templates
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}

func (r *PostgresContractTemplateRepo) ReadHistoryByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]db.ContractTemplateHistory, error) {
	query := `
        SELECT did, version, state, name, description,
               created_by, updated_by, created_at, updated_at, template_data, template_type, party_type, base_template,
               compliance_status, template_level, required_templates
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
	var raw []byte
	if data.TemplateData != nil {
		raw = *data.TemplateData
	}
	projection, err := validation.SearchProjectionJSON(ctx, raw)
	if err != nil {
		return nil, err
	}
	statement := `
        INSERT INTO contract_templates (
            did, created_by, state, name,
            description, template_data, template_type, party_type, base_template,
            template_level, required_templates, search_projection
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9,
                  COALESCE(NULLIF($10, ''), 'AGREEMENT'), COALESCE($11::text[], '{}'), $12)
        RETURNING created_at
    `
	var createdAt time.Time
	err = tx.GetContext(ctx, &createdAt, statement,
		data.DID, data.CreatedBy, data.State, data.Name,
		data.Description, data.TemplateData, data.TemplateType, data.PartyType, data.DID,
		data.TemplateLevel, data.RequiredTemplates, string(projection),
	)
	if err != nil {
		return nil, err
	}
	return &createdAt, nil
}

func (r *PostgresContractTemplateRepo) ReadDataByID(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractTemplate, error) {
	query := `
        SELECT did, version, state, name, description,
               created_by, updated_by, created_at, updated_at, content_updated_at, template_data, template_type, party_type,
               base_template, compliance_status, template_level, required_templates
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
			t.did, t.version, t.state, t.template_type, t.party_type, t.name,
			t.description, t.created_by, t.updated_by, t.created_at, t.updated_at,
			t.base_template, t.compliance_status, t.template_level, t.required_templates,
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
		offset := max(pagination.Offset-1, 0) * pagination.Limit
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
        SELECT did, version, state, name, template_type, party_type, description,
               created_by, updated_by, created_at, updated_at, base_template, compliance_status,
               template_level, required_templates
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
		offset := max(pagination.Offset-1, 0) * pagination.Limit
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
		SELECT did, version, state, updated_at, content_updated_at, created_by, catalogue_asset_hash
        FROM contract_templates WHERE did = $1
    `
	var processData db.ContractTemplateProcessData
	err := tx.GetContext(ctx, &processData, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: did=%s", db.ErrContractTemplateNotFound, did)
		}
		return nil, err
	}
	return &processData, nil
}

// UpdateState keeps the replaced state as a history entry, so the template
// provenance lists every lifecycle step. A refused change returns an
// error and the caller's rollback removes the entry. The update applies only
// while the stored state is from: of two concurrent transitions, the second
// waits for the first and then finds a changed state.
func (r *PostgresContractTemplateRepo) UpdateState(ctx context.Context, tx *sqlx.Tx, did string, from string, to string, actor string) error {
	if err := r.CreateHistoryEntryForDID(ctx, tx, did); err != nil {
		return fmt.Errorf("could not create history entry: %w", err)
	}
	statement := `
        UPDATE contract_templates ct
        SET state = $2::contract_template_state, updated_by = $4
        WHERE did = $1
          AND state = $3::contract_template_state
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
	result, err := tx.ExecContext(ctx, statement, did, to, from, actor)
	if err != nil {
		return fmt.Errorf("could not update registered state: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("%w: %s cannot change from %s to %s: the state changed or a version with that number is already active", db.ErrInvalidTemplateState, did, from, to)
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

func (r *PostgresContractTemplateRepo) ReadDIDsMissingStoredPDF(ctx context.Context, tx *sqlx.Tx, limit int, excludeDIDs []string) ([]string, error) {
	var dids []string
	// COALESCE: an empty Go slice binds as SQL NULL, and "did <> ALL (NULL)" is
	// NULL — which would filter out every row and stall the sweep.
	err := tx.SelectContext(ctx, &dids,
		`SELECT DISTINCT did FROM contract_templates
		 WHERE (pdf_ipfs_cid IS NULL OR pdf_ipfs_cid = '')
		   AND did <> ALL (COALESCE($2::text[], '{}'))
		 ORDER BY did
		 LIMIT $1`, limit, pq.Array(excludeDIDs),
	)
	if err != nil {
		return nil, err
	}
	return dids, nil
}

func (r *PostgresContractTemplateRepo) UpdatePDFState(ctx context.Context, tx *sqlx.Tx, did string, data db.ContractTemplatePDFState) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE contract_templates SET pdf_ipfs_cid=$1, pdf_renderer_version=$2, pdf_c2pa_state=$3, pdf_payload_hash=$4 WHERE did=$5`,
		data.IPFSCID, data.RendererVersion, data.C2PAState, data.PayloadHash, did,
	)
	return err
}

func (r *PostgresContractTemplateRepo) Update(ctx context.Context, tx *sqlx.Tx, data db.ContractTemplateUpdateData) error {
	var projection []byte
	var err error
	if data.TemplateData != nil && data.TemplateData.IsNotNullValue() {
		projection, err = validation.SearchProjectionJSON(ctx, *data.TemplateData)
		if err != nil {
			return err
		}
	}
	query, params, err := createQuery(data, projection)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, *query, params...)
	return err
}

// likeEscaper escapes the LIKE wildcards, so a search value matches as text.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// containsPattern is the ILIKE pattern for "contains value" (ESCAPE '\').
func containsPattern(value string) string {
	return "%" + likeEscaper.Replace(value) + "%"
}

func createSearchConditions(values db.SearchValues) (*string, []interface{}, error) {
	conditions := ""
	var params []interface{}
	paramIndex := 1
	if semantic, args := semantichub.SearchFilterSQL(values.SemanticFilters, "search_projection", paramIndex); semantic != "" {
		conditions += semantic + " AND"
		params = append(params, args...)
		paramIndex += len(args)
	}

	if len(values.DID) > 0 {
		conditions += ` did = $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, values.DID)
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
		params = append(params, values.TemplateType)
		paramIndex++
	}
	if values.PartyType != "" {
		conditions += ` party_type = $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, values.PartyType)
		paramIndex++
	}
	if len(values.Name) > 0 {
		conditions += ` name ILIKE $` + strconv.Itoa(paramIndex) + ` ESCAPE '\' AND`
		params = append(params, containsPattern(values.Name))
		paramIndex++
	}
	if len(values.Description) > 0 {
		conditions += ` description ILIKE $` + strconv.Itoa(paramIndex) + ` ESCAPE '\' AND`
		params = append(params, containsPattern(values.Description))
		paramIndex++
	}
	if values.CreatedBy != "" {
		conditions += ` created_by ILIKE $` + strconv.Itoa(paramIndex) + ` ESCAPE '\' AND`
		params = append(params, containsPattern(values.CreatedBy))
		paramIndex++
	}
	if values.CreatedFrom != "" {
		conditions += ` created_at >= $` + strconv.Itoa(paramIndex) + `::date AND`
		params = append(params, values.CreatedFrom)
		paramIndex++
	}
	if values.CreatedTo != "" {
		conditions += ` created_at < $` + strconv.Itoa(paramIndex) + `::date + 1 AND`
		params = append(params, values.CreatedTo)
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

func createQuery(data db.ContractTemplateUpdateData, projection []byte) (*string, []interface{}, error) {
	queryBase := `UPDATE contract_templates SET `
	var columns []string
	var params []interface{}

	addParam := func(columnName string, value interface{}) {
		columns = append(columns, fmt.Sprintf("%s = $%d", columnName, len(params)+1))
		params = append(params, value)
	}

	contentChanged := false
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
		addParam("search_projection", string(projection))
		contentChanged = true
	}
	if len(data.TemplateType) > 0 {
		addParam("template_type", data.TemplateType)
		contentChanged = true
	}
	if data.PartyType != "" {
		addParam("party_type", data.PartyType)
	}
	if data.TemplateLevel != nil {
		addParam("template_level", data.TemplateLevel)
	}
	if data.RequiredTemplates != nil {
		addParam("required_templates", pq.Array(data.RequiredTemplates))
	}
	if len(columns) == 0 {
		return nil, nil, errors.New("no fields to update")
	}
	if data.UpdatedBy != "" {
		addParam("updated_by", data.UpdatedBy)
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

// ReadLatestProvenanceVCID returns the vc_id of the newest credential issued
// in the template's version lineage (base_template), for linkage, or nil when
// this is the first registered version.
func (r *PostgresContractTemplateRepo) ReadLatestProvenanceVCID(ctx context.Context, tx *sqlx.Tx, did string) (*string, error) {
	var vcID string
	err := tx.GetContext(ctx, &vcID,
		`SELECT p.vc_id
		   FROM template_provenance_credentials p
		   JOIN contract_templates t ON t.did = p.did
		  WHERE t.base_template = (SELECT base_template FROM contract_templates WHERE did = $1)
		  ORDER BY p.version DESC
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

// UpdateComplianceStatus records the result of the template policy audit (DCS-FR-TR-10).
func (r *PostgresContractTemplateRepo) UpdateComplianceStatus(ctx context.Context, tx *sqlx.Tx, did string, status string) error {
	_, err := tx.ExecContext(ctx, `UPDATE contract_templates SET compliance_status = $2 WHERE did = $1`, did, status)
	return err
}

// ReadEvents reads the template's events from the transactional outbox, which
// keeps every event of the audit trail.
func (r *PostgresContractTemplateRepo) ReadEvents(ctx context.Context, tx *sqlx.Tx, did string, eventTypes ...string) ([]db.TemplateEvent, error) {
	var events []db.TemplateEvent
	err := tx.SelectContext(ctx, &events,
		`SELECT event_type, event_data, created_at FROM outbox_events
		  WHERE did = $1 AND component = 'CONTRACT_TEMPLATE_REPOSITORY' AND event_type = ANY($2)
		  ORDER BY id`,
		did, pq.Array(eventTypes),
	)
	if err != nil {
		return nil, fmt.Errorf("read events of template %s: %w", did, err)
	}
	return events, nil
}

// CountOpenContracts reads the effective contract state, so an expired
// contract counts as terminal.
func (r *PostgresContractTemplateRepo) CountOpenContracts(ctx context.Context, tx *sqlx.Tx, did string) (int, error) {
	var count int
	err := tx.GetContext(ctx, &count,
		`SELECT count(*) FROM contracts_effective_metadata
		  WHERE template_did = $1 AND state NOT IN ('TERMINATED', 'REJECTED', 'EXPIRED', 'WITHDRAWN', 'REVOKED')`,
		did,
	)
	if err != nil {
		return 0, fmt.Errorf("count open contracts of template %s: %w", did, err)
	}
	return count, nil
}

func (r *PostgresContractTemplateRepo) ReadPredecessorDID(ctx context.Context, tx *sqlx.Tx, did string) (*string, error) {
	var predecessor string
	err := tx.GetContext(ctx, &predecessor,
		`SELECT older.did FROM contract_templates older
		   JOIN contract_templates t ON t.did = $1
		  WHERE older.base_template = t.base_template
		    AND older.version < t.version
		    AND older.state IN ('REGISTERED', 'PUBLISHED', 'DEPRECATED')
		  ORDER BY older.version DESC
		  LIMIT 1`,
		did,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read predecessor of template %s: %w", did, err)
	}
	return &predecessor, nil
}

// UpdateCatalogueAssetHash records the Federated Catalogue asset of the published version (DCS-NFR-BR-09).
func (r *PostgresContractTemplateRepo) UpdateCatalogueAssetHash(ctx context.Context, tx *sqlx.Tx, did string, hash string) error {
	_, err := tx.ExecContext(ctx, `UPDATE contract_templates SET catalogue_asset_hash = $2 WHERE did = $1`, did, hash)
	return err
}
