package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"digital-contracting-service/internal/base/contractreadpolicy"

	"digital-contracting-service/internal/base/datatype"

	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"

	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
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
            description, contract_data, template_did, template_version, responsible,
            start_date, exp_date, exp_policy, exp_notice_period
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
    `
	_, err := tx.ExecContext(ctx, statement,
		data.DID, data.Origin, data.CreatedAt, data.CreatedBy, data.UpdatedAt, data.State, data.Name,
		data.Description, data.ContractData, data.TemplateDID, data.TemplateVersion, data.Responsible,
		data.StartDate, data.ExpDate, data.ExpPolicy, data.ExpNoticePeriod)
	return err
}

func (r *PostgresContractRepo) CreateHistoryEntryForDID(ctx context.Context, tx *sqlx.Tx, did string, modifiedBy string) error {
	statement := `
        INSERT INTO contract_history
            (did, origin, state, name, description, created_by, created_at, updated_at,
             contract_version, contract_data, start_date, exp_date, exp_policy,
             exp_notice_period, responsible, template_did, template_version,
             modified_by, modified_at)
        SELECT
            did, origin, state, name, description, created_by, created_at, updated_at,
            contract_version, contract_data, start_date, exp_date, exp_policy,
            exp_notice_period, responsible, template_did, template_version,
            $2, CURRENT_TIMESTAMP
        FROM contracts_effective
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did, modifiedBy)
	return err
}

func (r *PostgresContractRepo) ReadLastHistoryEntryByDID(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractHistory, error) {
	query := `
        SELECT did, origin, state, name, description,
               created_by, created_at, updated_at, contract_version, contract_data, start_date,
               exp_date, exp_policy, exp_notice_period, responsible, template_did, template_version,
               modified_by, modified_at
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
        SELECT id, did, origin, state, name, description,
               created_by, created_at, updated_at, contract_version, contract_data, start_date,
               exp_date, exp_policy, exp_notice_period, responsible, template_did, template_version,
               modified_by, modified_at
        FROM contract_history
        WHERE did = $1
        ORDER BY modified_at NULLS FIRST, contract_version, updated_at, id
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

// listSortColumns maps ListOptions.SortBy to its SQL expression. Only these
// expressions reach the ORDER BY clause.
var listSortColumns = map[string]string{
	"":           "cem.created_at",
	"created_at": "cem.created_at",
	"updated_at": "cem.updated_at",
	"state":      "cem.state::text",
	"name":       "COALESCE(cem.name, ce.contract_data->'dcs:metadata'->>'dcs:title')",
}

func (r *PostgresContractRepo) ReadOpenRenewalDID(ctx context.Context, tx *sqlx.Tx, originalDID string) (string, error) {
	var dids []string
	err := tx.SelectContext(ctx, &dids, `
		SELECT did FROM contracts_effective
		WHERE regexp_replace(contract_data->'dcs:renewsContract'->>'@id', '^.*/', '') = $1
		  AND state::text NOT IN ('WITHDRAWN', 'REVOKED', 'TERMINATED', 'EXPIRED')
		ORDER BY did LIMIT 1`, originalDID)
	if err != nil || len(dids) == 0 {
		return "", err
	}
	return dids[0], nil
}

func (r *PostgresContractRepo) ReadAllMetaData(ctx context.Context, tx *sqlx.Tx, pagination datatype.Pagination, options db.ListOptions) ([]db.ContractMetadata, error) {
	sortColumn, ok := listSortColumns[options.SortBy]
	if !ok {
		return nil, fmt.Errorf("unknown sort key %q", options.SortBy)
	}
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
			regexp_replace(ce.contract_data->'dcs:renewsContract'->>'@id', '^.*/', '') AS renews_contract_did,
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
	query += " WHERE " + contractreadpolicy.SQL(ctx, "cem.did", &params)
	if len(options.States) > 0 {
		params = append(params, pq.Array(options.States))
		query += fmt.Sprintf(" AND cem.state::text = ANY($%d)", len(params))
	}
	if options.ParentDID != "" {
		params = append(params, options.ParentDID)
		query += fmt.Sprintf(" AND regexp_replace(ce.contract_data->'dcs:parentContract'->>'@id', '^.*/', '') = $%d", len(params))
	}
	direction := "DESC"
	if options.Ascending {
		direction = "ASC"
	}
	query += fmt.Sprintf(" ORDER BY %s %s, cem.did", sortColumn, direction)
	if pagination.Limit > 0 {
		offset := (pagination.Offset - 1) * pagination.Limit
		query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(params)+1, len(params)+2)
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
	query += " WHERE TRUE"
	if len(params) > 0 {
		query += " AND " + *conditions
	}
	query += " AND " + contractreadpolicy.SQL(ctx, "contracts_effective_metadata.did", &params)

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

// nonExpiringStates are the states the expiry cron never flips to EXPIRED:
// DRAFT, EXPIRED itself, and the terminal states. WITHDRAWN and REVOKED are
// already-final/frozen states (see contracts_effective's matching exclusion
// list).
const nonExpiringStates = `('DRAFT', 'TERMINATED', 'REJECTED', 'EXPIRED', 'WITHDRAWN', 'REVOKED')`

func (r *PostgresContractRepo) ReadExpiredContracts(ctx context.Context, tx *sqlx.Tx) ([]db.ContractMetadata, error) {
	query := `
    SELECT did, origin, state, name, description, created_by, created_at, updated_at, contract_version, start_date,
           exp_date, exp_policy, exp_notice_period, responsible, template_did, template_version
    FROM contracts
    WHERE exp_date IS NOT NULL
    AND exp_date < NOW()
    AND state NOT IN ` + nonExpiringStates
	var cts []db.ContractMetadata
	err := tx.SelectContext(ctx, &cts, query)
	if err != nil {
		return nil, err
	}

	return cts, nil
}

func (r *PostgresContractRepo) ReadRenewalDueContracts(ctx context.Context, tx *sqlx.Tx) ([]db.ContractMetadata, error) {
	query := `
    SELECT c.did, c.origin, c.state, c.name, c.description, c.created_by, c.created_at, c.updated_at, c.contract_version,
           c.start_date, c.exp_date, c.exp_policy, c.exp_notice_period, c.responsible, c.template_did, c.template_version
    FROM contracts c
    WHERE c.exp_date IS NOT NULL
    AND c.exp_notice_period > 0
    AND c.state IN ('SIGNED', 'ACTIVE')
    AND c.exp_date >= NOW()
    AND c.exp_date - make_interval(days => c.exp_notice_period) <= NOW()
    AND NOT EXISTS (
        SELECT 1 FROM contract_renewal_notices n WHERE n.did = c.did AND n.exp_date = c.exp_date
    )
`
	var cts []db.ContractMetadata
	if err := tx.SelectContext(ctx, &cts, query); err != nil {
		return nil, err
	}
	return cts, nil
}

func (r *PostgresContractRepo) ClaimRenewalNotice(ctx context.Context, tx *sqlx.Tx, did string, expDate time.Time) (bool, error) {
	res, err := tx.ExecContext(ctx, `
        INSERT INTO contract_renewal_notices (did, exp_date) VALUES ($1, $2)
        ON CONFLICT (did, exp_date) DO NOTHING
    `, did, expDate)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
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

func (r *PostgresContractRepo) AppendArchivePeerSignatures(ctx context.Context, tx *sqlx.Tx, did string, peerSignatures []byte) (bool, error) {
	statement := `
        WITH entry AS (
            SELECT id, COALESCE(signature_metadata -> 'peer_signatures', '[]'::jsonb) AS held
            FROM contract_archive_entries
            WHERE did = $1 AND archive_status IN ('STORED', 'RETAINED')
            ORDER BY contract_version DESC
            LIMIT 1
        ), fresh AS (
            SELECT received.element, min(received.position) AS position
            FROM entry, jsonb_array_elements($2::jsonb) WITH ORDINALITY AS received(element, position)
            WHERE NOT entry.held @> jsonb_build_array(received.element)
            GROUP BY received.element
        )
        UPDATE contract_archive_entries a
        SET signature_metadata = jsonb_set(a.signature_metadata, '{peer_signatures}',
            entry.held || (SELECT jsonb_agg(fresh.element ORDER BY fresh.position) FROM fresh))
        FROM entry
        WHERE a.id = entry.id AND EXISTS (SELECT 1 FROM fresh)
    `
	res, err := tx.ExecContext(ctx, statement, did, peerSignatures)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

func (r *PostgresContractRepo) ReadLatestArchiveEntry(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractArchiveEntry, error) {
	var entry db.ContractArchiveEntry
	err := tx.GetContext(ctx, &entry, `
        SELECT did, contract_version, archive_status, content_hash, signature_metadata
        FROM contract_archive_entries
        WHERE did = $1 AND archive_status IN ('STORED', 'RETAINED')
        ORDER BY contract_version DESC
        LIMIT 1`, did)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

func (r *PostgresContractRepo) StoreExecutionProof(ctx context.Context, tx *sqlx.Tx, proof db.ExecutionProof) (bool, error) {
	res, err := tx.ExecContext(ctx, `
        INSERT INTO contract_execution_proofs (did, content_hash, proof, notary_receipt, tsa_receipt)
        VALUES ($1, $2, $3, $4, $5)
        ON CONFLICT (did, content_hash) DO NOTHING`,
		proof.DID, proof.ContentHash, proof.Proof, proof.NotaryReceipt, proof.TSAReceipt)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (r *PostgresContractRepo) ReadArchiveEntries(ctx context.Context, tx *sqlx.Tx) ([]db.ContractArchiveEntry, error) {
	query := `
        SELECT did, contract_version, archive_status, stored_by, stored_at, contract_snapshot,
               content_hash, snapshot_cid, snapshot_cid_created_at, signature_metadata, credential_hashes, tsa_receipt, evidence, retention_until,
               deleted_at, deleted_by, deletion_reason
        FROM contract_archive_entries

    `
	var params []any
	query += " WHERE " + contractreadpolicy.SQL(ctx, "contract_archive_entries.did", &params) + " ORDER BY stored_at, did, contract_version"
	var entries []db.ContractArchiveEntry
	err := tx.SelectContext(ctx, &entries, query, params...)
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

func (r *PostgresContractRepo) ReadSignedSignatures(ctx context.Context, tx *sqlx.Tx, did string) ([]db.SignedSignature, error) {
	var signatures []db.SignedSignature
	err := tx.SelectContext(ctx, &signatures, `
        SELECT signer_did, credential_type, COALESCE(field_name, '') AS field_name, signed_at,
               COALESCE(pdf_hash, '') AS pdf_hash, COALESCE(jades_signature, '') AS jades_signature
        FROM contract_signatures
        WHERE contract_did = $1 AND status = 'SIGNED'
        ORDER BY created_at
    `, did)
	if err != nil {
		return nil, err
	}
	return signatures, nil
}

// archivedContractColumns and archivedContractSource read an archived
// contract with its termination record (DCS-FR-CSA-16). USING keeps the
// unqualified did of the search conditions unambiguous.
const archivedContractColumns = `did, state, name, description, created_by, created_at, updated_at, contract_version, start_date, exp_date,
	exp_policy, exp_notice_period, responsible, evidence, archive_summary, archive_tags,
	reason AS termination_reason, effective_at AS terminated_at, initiating_role AS termination_role`
const archivedContractSource = `contracts_archive_metadata LEFT JOIN contract_terminations USING (did)`

func (r *PostgresContractRepo) ReadArchivedContracts(ctx context.Context, tx *sqlx.Tx) ([]db.ContractMetadata, error) {
	query := `SELECT ` + archivedContractColumns + ` FROM ` + archivedContractSource
	var params []any
	query += " WHERE " + contractreadpolicy.SQL(ctx, "contracts_archive_metadata.did", &params)
	var cts []db.ContractMetadata
	err := tx.SelectContext(ctx, &cts, query, params...)
	if err != nil {
		return nil, err
	}

	return cts, nil
}

func (r *PostgresContractRepo) ReadArchivedContractsByFilter(ctx context.Context, tx *sqlx.Tx, values db.SearchValues) ([]db.ContractMetadata, error) {
	query := `SELECT ` + archivedContractColumns + ` FROM ` + archivedContractSource
	conditions, params, err := createSearchConditions(values)
	if err != nil {
		return nil, err
	}
	query += " WHERE TRUE"
	if len(params) > 0 {
		query += " AND " + *conditions
	}
	query += " AND " + contractreadpolicy.SQL(ctx, "contracts_archive_metadata.did", &params)

	var cts []db.ContractMetadata
	err = tx.SelectContext(ctx, &cts, query, params...)
	if err != nil {
		return nil, err
	}
	return cts, nil
}

func (r *PostgresContractRepo) UpdateState(ctx context.Context, tx *sqlx.Tx, did, from, to string) error {
	return UpdateContractState(ctx, tx, did, from, to)
}

// UpdateContractState is the write of a contract's lifecycle state: it moves
// did from state from to state to, and fails with
// contractstate.ErrInvalidTransition when another transaction changed the
// state after the caller read it.
func UpdateContractState(ctx context.Context, tx *sqlx.Tx, did, from, to string) error {
	res, err := tx.ExecContext(ctx, `UPDATE contracts SET state = $3 WHERE did = $1 AND state = $2`, did, from, to)
	if err != nil {
		return fmt.Errorf("update state of contract %s: %w", did, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("update state of contract %s: %w", did, err)
	}
	if n == 0 {
		return fmt.Errorf("%w: contract %s is no longer %s", contractstate.ErrInvalidTransition, did, from)
	}
	return nil
}

func (r *PostgresContractRepo) ExpireContract(ctx context.Context, tx *sqlx.Tx, did string) (bool, error) {
	statement := `
        UPDATE contracts SET state = 'EXPIRED'
        WHERE did = $1
        AND exp_date IS NOT NULL
        AND exp_date < NOW()
        AND state NOT IN ` + nonExpiringStates
	res, err := tx.ExecContext(ctx, statement, did)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (r *PostgresContractRepo) ReadPDFState(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractPDFState, error) {
	var state db.ContractPDFState
	err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(pdf_ipfs_cid,''), COALESCE(pdf_renderer_version,''), COALESCE(pdf_c2pa_state,''), COALESCE(pdf_payload_hash,''), pdf_signed FROM contracts WHERE did=$1`, did,
	).Scan(&state.IPFSCID, &state.RendererVersion, &state.C2PAState, &state.PayloadHash, &state.Signed)
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
	//
	// The last arm selects a signed PDF that does not yet assert the lifecycle
	// state its contract reached after signing (provenance.
	// SignedLifecycleAppendPending), so a lost suspend, terminate or expiry
	// event still gets its lifecycle manifest appended.
	err := tx.SelectContext(ctx, &dids,
		`SELECT c.did FROM contracts c
		 WHERE ((
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
		    OR (
		           c.pdf_signed
		       AND COALESCE(c.pdf_ipfs_cid, '') <> ''
		       AND COALESCE(c.pdf_c2pa_state, '') <> CASE c.state
		               WHEN 'REVOKED' THEN 'suspended'
		               WHEN 'TERMINATED' THEN 'terminated'
		               WHEN 'EXPIRED' THEN 'expired'
		               ELSE COALESCE(c.pdf_c2pa_state, '') END
		       ))
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
		`UPDATE contracts SET pdf_ipfs_cid=$1, pdf_renderer_version=$2, pdf_c2pa_state=$3, pdf_payload_hash=$4, pdf_signed = pdf_signed OR $6 WHERE did=$5`,
		data.IPFSCID, data.RendererVersion, data.C2PAState, data.PayloadHash, did, data.Signed,
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
	if len(values.States) > 0 {
		conditions += ` state::text = ANY($` + strconv.Itoa(paramIndex) + `) AND`
		states := make([]string, len(values.States))
		for i, state := range values.States {
			states[i] = strings.ToUpper(state)
		}
		params = append(params, pq.Array(states))
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
	if len(values.ContractData) > 0 {
		conditions += ` did IN (SELECT did FROM contracts WHERE search_vector @@ plainto_tsquery('english', $` + strconv.Itoa(paramIndex) + `)) AND`
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
		paramIndex++
	}
	if len(values.TemplateDID) > 0 {
		conditions += ` template_did = $` + strconv.Itoa(paramIndex) + ` AND`
		params = append(params, values.TemplateDID)
		paramIndex++
	}
	if len(values.Jurisdiction) > 0 {
		conditions += ` did IN (SELECT c.did FROM contracts_effective c,
			jsonb_array_elements(CASE WHEN jsonb_typeof(c.contract_data->'dcs:contractFields') = 'array' THEN c.contract_data->'dcs:contractFields' ELSE '[]'::jsonb END) f
			WHERE f->'dcs:shape'->>'@id' LIKE '%#field-contract-jurisdiction'
			AND COALESCE(f->'dcs:value'->>'@value', f->>'dcs:value') ILIKE $` + strconv.Itoa(paramIndex) + ` ESCAPE '\') AND`
		params = append(params, containsPattern(values.Jurisdiction))
	}
	l := len(" AND")
	if len(conditions) > l {
		conditions = conditions[:len(conditions)-l]
	}

	return &conditions, params, nil
}

// likeEscaper escapes the LIKE wildcards, so a search value matches as text.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// containsPattern is the ILIKE pattern for "contains value" (ESCAPE '\').
func containsPattern(value string) string {
	return "%" + likeEscaper.Replace(value) + "%"
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
	// created_at is set on insert only. PeerPdfReceiver's re-ship path leaves
	// CreatedAt as the zero time; writing it here used to overwrite a real
	// creation timestamp with 0001-01-01 and bury the contract at the end of
	// created_at DESC lists.
	if !data.CreatedAt.IsZero() {
		addParam("created_at", data.CreatedAt)
	}
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
