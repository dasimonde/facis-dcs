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
	"github.com/lib/pq"

	"digital-contracting-service/internal/contractworkflowengine/db"
)

type PostgresContractRepo struct {
}

func (r *PostgresContractRepo) Create(ctx context.Context, tx *sqlx.Tx, data db.Contract) error {

	statement := `
        INSERT INTO contracts (
            did, origin, created_at, created_by, state, name,
            description, contract_data, template_did, template_version, responsible
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
    `
	_, err := tx.ExecContext(ctx, statement,
		data.DID, data.Origin, data.CreatedAt, data.CreatedBy, data.State, data.Name,
		data.Description, data.ContractData, data.TemplateDID, data.TemplateVersion, data.Responsible)
	return err
}

func (r *PostgresContractRepo) RemoteCreate(ctx context.Context, tx *sqlx.Tx, data db.Contract) error {

	if data.CreatedAt.IsZero() {
		data.CreatedAt = time.Now()
	}

	statement := `
        INSERT INTO contracts (
            did, origin, created_at, created_by, updated_at, state, name,
            description, contract_data, template_did, template_version, responsible
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
    `
	_, err := tx.ExecContext(ctx, statement,
		data.DID, data.Origin, data.CreatedAt, data.CreatedBy, data.UpdatedAt, data.State, data.Name,
		data.Description, data.ContractData, data.TemplateDID, data.TemplateVersion, data.Responsible)
	return err
}

func (r *PostgresContractRepo) CreateHistoryEntryForDID(ctx context.Context, tx *sqlx.Tx, did string) error {
	statement := `
        INSERT INTO contract_history
            (did, origin, state, name, description, created_by, created_at, updated_at,
             contract_version, contract_data, start_date, exp_date, exp_policy,
             exp_notice_period, responsible, template_did, template_version)
        SELECT
            did, origin, state, name, description, created_by, created_at, updated_at,
            contract_version, contract_data, start_date, exp_date, exp_policy,
            exp_notice_period, responsible, template_did, template_version
        FROM contracts_effective
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did)
	return err
}

func (r *PostgresContractRepo) ReadLastHistoryEntryByDID(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractHistory, error) {
	query := `
        SELECT did, origin, state, name, description,
               created_by, created_at, updated_at, contract_version, contract_data, start_date,
               exp_date, exp_policy, exp_notice_period, responsible, template_did, template_version
        FROM contract_history
        WHERE did = $1
        ORDER BY contract_version DESC NULLS LAST
    	LIMIT 1
    `
	var ct db.ContractHistory
	err := tx.GetContext(ctx, &ct, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("contract with DID %s not found", did)
		}
		return nil, err
	}
	return &ct, nil
}

func (r *PostgresContractRepo) ReadHistoryByDID(ctx context.Context, tx *sqlx.Tx, did string) ([]db.ContractHistory, error) {
	query := `
        SELECT did, origin, state, name, description,
               created_by, created_at, updated_at, contract_version, contract_data, start_date,
               exp_date, exp_policy, exp_notice_period, responsible, template_did, template_version
        FROM contract_history
        WHERE did = $1
    `
	var ct []db.ContractHistory
	err := tx.SelectContext(ctx, &ct, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return []db.ContractHistory{}, fmt.Errorf("contract with DID %s not found", did)
		}
		return []db.ContractHistory{}, err
	}
	return ct, nil
}

func (r *PostgresContractRepo) ReadDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*db.Contract, error) {
	query := `
        SELECT ce.did, ce.origin, ce.state, ce.name, ce.description,
               ce.created_by, ce.created_at, ce.updated_at, ce.contract_version, ce.contract_data,
               ce.start_date, ce.exp_date, ce.exp_policy, ce.exp_notice_period, ce.responsible,
               ce.template_did, ce.template_version,
               -- Joined rather than carried by contracts_effective: that view
               -- has dependents, so widening it would mean dropping and
               -- rebuilding them (ADR-25).
               c.target_id
        FROM contracts_effective ce
        JOIN contracts c ON c.did = ce.did
        WHERE ce.did = $1
    `
	var ct db.Contract
	err := tx.GetContext(ctx, &ct, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("contract with DID %s not found", did)
		}
		return nil, err
	}
	return &ct, nil
}

func (r *PostgresContractRepo) ExistsByDID(ctx context.Context, tx *sqlx.Tx, did string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM contracts_effective WHERE did = $1)`
	if err := tx.GetContext(ctx, &exists, query, did); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *PostgresContractRepo) ReadChildrenDIDs(ctx context.Context, tx *sqlx.Tx, did string) ([]string, error) {
	query := `
        SELECT did
        FROM contracts_effective
        WHERE regexp_replace(contract_data->'dcs:parentContract'->>'@id', '^.*/', '') = $1
        ORDER BY did
    `
	children := []string{}
	if err := tx.SelectContext(ctx, &children, query, did); err != nil {
		return nil, err
	}
	return children, nil
}

func (r *PostgresContractRepo) ReadAllMetaData(ctx context.Context, tx *sqlx.Tx, pagination datatype.Pagination) ([]db.ContractMetadata, error) {
	query := `
		SELECT
			cem.did, cem.origin, cem.state, cem.name, cem.description, cem.created_by, cem.created_at, cem.updated_at,
			cem.contract_version, cem.start_date, cem.exp_date, cem.exp_policy, cem.exp_notice_period, cem.responsible,
			cem.template_did, cem.template_version,
			cem.state IN ('DRAFT', 'REJECTED', 'SUBMITTED', 'NEGOTIATION', 'REVIEWED', 'APPROVED')
			AND COALESCE(latest.version > cem.template_version, FALSE) AS outdated,
			latest.did AS latest_template_did,
			COALESCE(tpl.state = 'DEPRECATED', FALSE) AS template_is_deprecated,
			regexp_replace(ce.contract_data->'dcs:parentContract'->>'@id', '^.*/', '') AS parent_contract_did,
			COALESCE(cem.name, ce.contract_data->'dcs:metadata'->>'dcs:title') AS name,
			c.target_id,
			tgt.name AS target_name
		FROM contracts_effective_metadata cem
		LEFT JOIN contracts_effective ce ON ce.did = cem.did
		LEFT JOIN contracts c ON c.did = cem.did
		LEFT JOIN contract_targets tgt ON tgt.id = c.target_id
		LEFT JOIN contract_templates tpl
			ON tpl.did = cem.template_did
		LEFT JOIN LATERAL (
			SELECT ct.did, ct.version
			FROM contract_templates ct
			WHERE ct.base_template = tpl.base_template
			  AND ct.state IN ('REGISTERED', 'PUBLISHED')
			ORDER BY ct.version DESC
			LIMIT 1
		) latest ON true
	`
	var params []any
	if pagination.Limit > 0 {
		offset := (pagination.Offset - 1) * pagination.Limit
		query += ` ORDER BY created_at DESC LIMIT $1 OFFSET $2`
		params = append(params, pagination.Limit, offset)
	}

	var cts []db.ContractMetadata
	err := tx.SelectContext(ctx, &cts, query, params...)
	if err != nil {
		return []db.ContractMetadata{}, err
	}
	return cts, nil
}

func (r *PostgresContractRepo) ReadAllMetaDataByFilter(ctx context.Context, tx *sqlx.Tx, values db.SearchValues, pagination datatype.Pagination) ([]db.ContractMetadata, error) {
	query := `
        SELECT did, origin, state, name, description, created_by, created_at, updated_at, contract_version, start_date,
               exp_date, exp_policy, exp_notice_period, responsible, template_did, template_version
        FROM contracts_effective_metadata
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

	var cts []db.ContractMetadata
	err = tx.SelectContext(ctx, &cts, query, params...)
	if err != nil {
		return nil, err
	}
	return cts, nil
}

func (r *PostgresContractRepo) ReadProcessDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractProcessData, error) {
	query := `
        SELECT did, origin,  state, updated_at, content_updated_at, created_by, contract_version, start_date, exp_date, exp_policy, exp_notice_period
        FROM contracts_effective_process_data WHERE did = $1
    `
	var processData db.ContractProcessData
	err := tx.GetContext(ctx, &processData, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("contract with DID %s", did)
		}
		return nil, err
	}
	return &processData, nil
}

func (r *PostgresContractRepo) ReadProcessDataByDIDOrNil(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractProcessData, error) {
	query := `
        SELECT did, origin,  state, updated_at, content_updated_at, created_by, contract_version, start_date, exp_date, exp_policy, exp_notice_period
        FROM contracts_effective_process_data WHERE did = $1
    `
	var processData db.ContractProcessData
	err := tx.GetContext(ctx, &processData, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &processData, nil
}

func (r *PostgresContractRepo) ReadExpiredContracts(ctx context.Context, tx *sqlx.Tx) ([]db.ContractMetadata, error) {
	// WITHDRAWN and REVOKED are excluded alongside the other terminal
	// states: both are already-final/frozen states, so the
	// expiry cron must not force-flip them to EXPIRED (see
	// contracts_effective's matching exclusion list).
	query := `
    SELECT did, origin, state, name, description, created_by, created_at, updated_at, contract_version, start_date,
           exp_date, exp_policy, exp_notice_period, responsible, template_did, template_version
    FROM contracts
    WHERE exp_date IS NOT NULL
    AND exp_date < NOW()
    AND state NOT IN ('DRAFT', 'TERMINATED', 'REJECTED', 'EXPIRED', 'WITHDRAWN', 'REVOKED')
`
	var cts []db.ContractMetadata
	err := tx.SelectContext(ctx, &cts, query)
	if err != nil {
		return nil, err
	}

	return cts, nil
}

func (r *PostgresContractRepo) StoreArchiveEntry(ctx context.Context, tx *sqlx.Tx, data db.ContractArchiveEntry) error {
	statement := `
        INSERT INTO contract_archive_entries (
            did, contract_version, stored_by, stored_at, contract_snapshot, content_hash, snapshot_cid, signature_metadata,
            credential_hashes, tsa_receipt, evidence, retention_until
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, COALESCE($8::jsonb, '{}'::jsonb), COALESCE($9::jsonb, '{}'::jsonb), COALESCE($10::jsonb, '{}'::jsonb), COALESCE($11::jsonb, '{}'::jsonb), $12)
        ON CONFLICT (did, contract_version) DO NOTHING
    `
	_, err := tx.ExecContext(ctx, statement,
		data.DID,
		data.ContractVersion,
		data.StoredBy,
		data.StoredAt,
		data.ContractSnapshot,
		data.ContentHash,
		data.SnapshotCID,
		data.SignatureMeta,
		data.CredentialHashes,
		data.TSAReceipt,
		data.Evidence,
		data.RetentionUntil,
	)
	return err
}

func (r *PostgresContractRepo) ReadArchiveEntries(ctx context.Context, tx *sqlx.Tx) ([]db.ContractArchiveEntry, error) {
	query := `
        SELECT did, contract_version, archive_status, stored_by, stored_at, contract_snapshot,
               content_hash, snapshot_cid, snapshot_cid_created_at, signature_metadata, credential_hashes, tsa_receipt, evidence, retention_until,
               deleted_at, deleted_by, deletion_reason
        FROM contract_archive_entries
        ORDER BY stored_at, did, contract_version
    `
	var entries []db.ContractArchiveEntry
	err := tx.SelectContext(ctx, &entries, query)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

func (r *PostgresContractRepo) MarkArchiveEntryDeleted(ctx context.Context, tx *sqlx.Tx, did string, deletedBy string, reason string) (int, error) {
	// archive_status must flip to DELETED together with the deletion
	// metadata: the contract_archive_entries trigger
	// (migrations/sql/20260305_create_contract_repository.sql) rejects
	// deletion metadata on rows whose status is still STORED/RETAINED.
	statement := `
        UPDATE contract_archive_entries
        SET archive_status = 'DELETED', deleted_at = NOW(), deleted_by = $1, deletion_reason = $2
        WHERE did = $3 AND deleted_at IS NULL
    `
	result, err := tx.ExecContext(ctx, statement, deletedBy, reason, did)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

func (r *PostgresContractRepo) AnnotateArchiveEntry(ctx context.Context, tx *sqlx.Tx, did string, summary string, tags *datatype.JSON) (int, error) {
	// Only the annotation columns are updated; the immutable-fields trigger
	// on contract_archive_entries guards the snapshot/evidence columns, and
	// DELETED entries are excluded so a soft-deleted entry can never be
	// re-labelled.
	statement := `
        UPDATE contract_archive_entries
        SET summary = $1, tags = COALESCE($2, tags)
        WHERE did = $3 AND archive_status <> 'DELETED'
    `
	result, err := tx.ExecContext(ctx, statement, summary, tags, did)
	if err != nil {
		return 0, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	return int(affected), nil
}

func (r *PostgresContractRepo) ReadSignedSignatureFieldNames(ctx context.Context, tx *sqlx.Tx, did string) ([]string, error) {
	var fields []string
	err := tx.SelectContext(ctx, &fields, `
        SELECT field_name FROM contract_signatures
        WHERE contract_did = $1 AND status = 'SIGNED' AND field_name IS NOT NULL
    `, did)
	if err != nil {
		return nil, err
	}
	return fields, nil
}

func (r *PostgresContractRepo) ReadArchivedContracts(ctx context.Context, tx *sqlx.Tx) ([]db.ContractMetadata, error) {
	query := `
	    SELECT did, state, name, description, created_by, created_at, updated_at, contract_version, start_date, exp_date, exp_policy, exp_notice_period, responsible, evidence, archive_summary, archive_tags
    FROM contracts_archive_metadata
	`
	var cts []db.ContractMetadata
	err := tx.SelectContext(ctx, &cts, query)
	if err != nil {
		return nil, err
	}

	return cts, nil
}

func (r *PostgresContractRepo) ReadArchivedContractsByFilter(ctx context.Context, tx *sqlx.Tx, values db.SearchValues) ([]db.ContractMetadata, error) {
	query := `
	        SELECT did, state, name, description, created_by, created_at, updated_at, contract_version, start_date, exp_date, exp_policy, exp_notice_period, responsible, evidence, archive_summary, archive_tags
        FROM contracts_archive_metadata
    `
	conditions, params, err := createSearchConditions(values)
	if err != nil {
		return nil, err
	}
	if len(params) > 0 {
		query += " WHERE " + *conditions
	}

	var cts []db.ContractMetadata
	err = tx.SelectContext(ctx, &cts, query, params...)
	if err != nil {
		return nil, err
	}
	return cts, nil
}

func (r *PostgresContractRepo) UpdateState(ctx context.Context, tx *sqlx.Tx, did string, state string) error {
	statement := `
        UPDATE contracts SET state = $2
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did, state)
	return err
}

func (r *PostgresContractRepo) ReadPDFState(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractPDFState, error) {
	var state db.ContractPDFState
	err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(pdf_ipfs_cid,''), COALESCE(pdf_renderer_version,''), COALESCE(pdf_c2pa_state,''), COALESCE(pdf_payload_hash,'') FROM contracts WHERE did=$1`, did,
	).Scan(&state.IPFSCID, &state.RendererVersion, &state.C2PAState, &state.PayloadHash)
	if err != nil {
		return nil, err
	}
	return &state, nil
}

func (r *PostgresContractRepo) CountSignedSignatures(ctx context.Context, tx *sqlx.Tx, did string) (int, error) {
	var count int
	err := tx.GetContext(ctx, &count,
		`SELECT COUNT(*) FROM contract_signatures WHERE contract_did = $1 AND status = 'SIGNED'`, did)
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *PostgresContractRepo) ReadDIDsNeedingRegeneration(ctx context.Context, tx *sqlx.Tx, limit int, excludeDIDs []string) ([]string, error) {
	var dids []string
	// The superseded arm hashes contract_data exactly as the regenerator and the
	// ship gate hash it in Go: the jsonb column rendered to its text form, then
	// SHA-256, then hex. A contract with no data hashes the empty string on both
	// sides. Should the two ever disagree, the sweep over-selects and
	// appendC2PA — which recomputes the hash itself and short-circuits when
	// nothing changed — declines the work for the cost of two reads; it can
	// never re-render a document that is already current.
	//
	// COALESCE on the exclude list: an empty Go slice binds as SQL NULL, and
	// "did <> ALL (NULL)" is NULL — which would filter out every row and stall
	// the sweep.
	err := tx.SelectContext(ctx, &dids,
		`SELECT c.did FROM contracts c
		 WHERE (
		         (c.pdf_ipfs_cid IS NULL OR c.pdf_ipfs_cid = '')
		      OR (
		             COALESCE(c.pdf_c2pa_state, '') IN ('', 'draft')
		         AND COALESCE(c.pdf_payload_hash, '') <> ''
		         AND c.pdf_payload_hash <> encode(
		                 sha256(convert_to(COALESCE(c.contract_data::text, ''), 'UTF8')), 'hex')
		         )
		       )
		   AND NOT EXISTS (
		       SELECT 1 FROM contract_signatures s
		        WHERE s.contract_did = c.did AND s.status = 'SIGNED')
		   AND c.did <> ALL (COALESCE($2::text[], '{}'))
		 ORDER BY c.created_at, c.did
		 LIMIT $1`, limit, pq.Array(excludeDIDs),
	)
	if err != nil {
		return nil, err
	}
	return dids, nil
}

func (r *PostgresContractRepo) UpdatePDFState(ctx context.Context, tx *sqlx.Tx, did string, data db.ContractPDFState) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE contracts SET pdf_ipfs_cid=$1, pdf_renderer_version=$2, pdf_c2pa_state=$3, pdf_payload_hash=$4 WHERE did=$5`,
		data.IPFSCID, data.RendererVersion, data.C2PAState, data.PayloadHash, did,
	)
	return err
}

func (r *PostgresContractRepo) Update(ctx context.Context, tx *sqlx.Tx, data db.ContractUpdateData) error {
	query, params, err := createQuery(data)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, *query, params...)
	return err
}

func (r *PostgresContractRepo) RemoteUpdate(ctx context.Context, tx *sqlx.Tx, data db.Contract) error {
	query, params, err := createRemoteQuery(data)
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
	if values.ContractVersion > 0 {
		conditions += ` contract_version = $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, values.ContractVersion)
		paramIndex++
	}
	if len(values.State) > 0 {
		conditions += ` state = $` + strconv.Itoa(paramIndex) + ` AND`
		state := strings.ToUpper(values.State)
		params = append(params, state)
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
	if len(values.ContractData) > 0 {
		conditions += ` search_vector @@ plainto_tsquery('english', $` + strconv.Itoa(paramIndex) + `) AND`
		params = append(params, values.ContractData)
		paramIndex++
	}
	if len(values.Tag) > 0 {
		// Annotation-tag filter (DCS-FR-CSA-11): archive_tags is a JSONB
		// string array on the archive view, GIN-indexed for containment.
		conditions += ` archive_tags @> jsonb_build_array($` + strconv.Itoa(paramIndex) + `::text) AND`
		params = append(params, values.Tag)
		paramIndex++
	}
	if len(values.Party) > 0 {
		// Party filter (DCS-FR-CSA-10, DCS-FR-CSA-13): the contract's two
		// parties live in the responsible JSONB as creator/counterparty.
		conditions += ` (responsible->>'creator' = $` + strconv.Itoa(paramIndex) + ` OR responsible->>'counterparty' = $` + strconv.Itoa(paramIndex+1) + `) AND`
		params = append(params, values.Party, values.Party)
		paramIndex += 2
	}
	if values.ValidFrom != nil {
		conditions += ` start_date >= $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, *values.ValidFrom)
		paramIndex++
	}
	if values.ValidUntil != nil {
		conditions += ` exp_date <= $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, *values.ValidUntil)
		paramIndex++
	}
	if len(values.ParentDID) > 0 {
		// Reverse-index over locally held children: match the child's stored
		// dcs:parentContract @id in contracts_effective. Kept as a DID-scoped
		// subquery so it composes with any outer metadata/archive table.
		conditions += ` did IN (SELECT did FROM contracts_effective WHERE regexp_replace(contract_data->'dcs:parentContract'->>'@id', '^.*/', '') = $` + strconv.Itoa(paramIndex) + `) AND`
		params = append(params, values.ParentDID)
	}
	l := len(" AND")
	if len(conditions) > l {
		conditions = conditions[:len(conditions)-l]
	}

	return &conditions, params, nil
}

func createQuery(data db.ContractUpdateData) (*string, []interface{}, error) {
	queryBase := `UPDATE contracts SET `
	var columns []string
	var params []interface{}

	addParam := func(columnName string, value interface{}) {
		columns = append(columns, fmt.Sprintf("%s = $%d", columnName, len(params)+1))
		params = append(params, value)
	}

	if data.ContractVersion > 0 {
		addParam("contract_version", data.ContractVersion)
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
	if data.ContractData != nil && data.ContractData.IsNotNullValue() {
		addParam("contract_data", data.ContractData)
	}
	if data.StartDate != nil {
		addParam("start_date", data.StartDate)
	}
	if data.ExpDate != nil {
		addParam("exp_date", data.ExpDate)
	}
	if data.ExpPolicy != nil {
		addParam("exp_policy", data.ExpPolicy)
	}
	if data.ExpNoticePeriod != nil {
		addParam("exp_notice_period", data.ExpNoticePeriod)
	}
	if data.Responsible != nil {
		addParam("responsible", data.Responsible)
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

func createRemoteQuery(data db.Contract) (*string, []interface{}, error) {
	queryBase := `UPDATE contracts SET `
	var columns []string
	var params []interface{}

	addParam := func(columnName string, value interface{}) {
		columns = append(columns, fmt.Sprintf("%s = $%d", columnName, len(params)+1))
		params = append(params, value)
	}

	if data.ContractVersion > 0 {
		addParam("contract_version", data.ContractVersion)
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
	if data.ContractData != nil && data.ContractData.IsNotNullValue() {
		addParam("contract_data", data.ContractData)
	}
	if data.StartDate != nil {
		addParam("start_date", data.StartDate)
	}
	if data.ExpDate != nil {
		addParam("exp_date", data.ExpDate)
	}
	if data.ExpPolicy != nil {
		addParam("exp_policy", data.ExpPolicy)
	}
	if data.ExpNoticePeriod != nil {
		addParam("exp_notice_period", data.ExpNoticePeriod)
	}

	addParam("template_did", data.TemplateDID)
	addParam("template_version", data.TemplateVersion)
	addParam("created_at", data.CreatedAt)
	addParam("created_by", data.CreatedBy)
	addParam("updated_at", data.UpdatedAt)
	addParam("origin", data.Origin)

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
