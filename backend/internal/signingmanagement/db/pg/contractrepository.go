package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"

	"digital-contracting-service/internal/base/datatype"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/signingmanagement/db"
)

type PostgresContractRepo struct {
	Artifacts *artifactstore.Store
	PDFCore   *pdfcore.Client
}

// ReadDataByDID reads the contract regardless of lifecycle state — like
// ReadProcessDataByDID, state gating is decided in Go against
// contractstate.Transitions (command/apply.go), not by a hardcoded SQL state
// literal. Signature evidence in particular stays retrievable for the
// contract's whole post-signing life (ACTIVE after deployment, REVOKED after
// revocation, ...), not only while it sits in APPROVED/SIGNED.
func (r *PostgresContractRepo) ReadDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*db.Contract, error) {
	query := `
        SELECT did, state, name, description,
               created_by, created_at, updated_at, contract_version, contract_data, start_date, exp_date, exp_policy, exp_notice_period, responsible
        FROM contracts
        WHERE did = $1
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

func (r *PostgresContractRepo) ReadAllMetaData(ctx context.Context, tx *sqlx.Tx, pagination datatype.Pagination) ([]db.ContractMetadata, error) {
	// ACTIVE belongs here too: signing completion auto-deploys the contract
	// (SIGNED -> ACTIVE), so a fully-signed contract that the Signature Compliance
	// Viewer must still inspect lives in ACTIVE, not SIGNED. REVOKED stays
	// listed as well (DCS-FR-SM-22): the signer dashboard and the compliance
	// viewer must show a revoked signing status, not hide the contract.
	query := `
        SELECT did, state, name, description, created_by, created_at, updated_at, contract_version, start_date, exp_date, exp_policy, exp_notice_period, responsible
        FROM contracts
        WHERE state IN ('APPROVED', 'SIGNED', 'ACTIVE', 'REVOKED')
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

// ReadProcessDataByDID reads the contract's current state regardless of what
// that state is — the SIGNED transition gate is decided in Go by
// contractstate.ValidateTransition (the single-source-of-truth transition
// table), not by a hardcoded SQL state literal (see command/apply.go).
func (r *PostgresContractRepo) ReadProcessDataByDID(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractProcessData, error) {
	query := `
        SELECT did, state, updated_at, created_by, contract_version
        FROM contracts
        WHERE did = $1
    `
	var processData db.ContractProcessData
	err := tx.GetContext(ctx, &processData, query, did)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("could not found contract with DID %s", did)
		}
		return nil, err
	}
	return &processData, nil
}

// UpdateState is unconditional: the caller (command/apply.go) already
// validated the transition against contractstate.Transitions before calling
// this, so no hardcoded SQL state guard is needed here anymore.
func (r *PostgresContractRepo) UpdateState(ctx context.Context, tx *sqlx.Tx, did string, state string) error {
	statement := `
        UPDATE contracts SET state = $2
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did, state)
	return err
}

// UpdateContractData persists the sealed contract document the first
// signature commits to (command/apply.go's Offer-to-Agreement seal); it
// runs inside the signing transaction, before the content hash and the
// PAdES signature are computed over the same bytes.
func (r *PostgresContractRepo) UpdateContractData(ctx context.Context, tx *sqlx.Tx, did string, contractData datatype.JSON) error {
	statement := `
        UPDATE contracts SET contract_data = $2
        WHERE did = $1
    `
	_, err := tx.ExecContext(ctx, statement, did, contractData)
	return err
}

// ---------------------------------------------------------------------------------------------------------------------

func (r *PostgresContractRepo) CreateSignature(ctx context.Context, tx *sqlx.Tx, signature db.ContractSignature) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO contract_signatures
			(contract_did, signer_did, credential_type, signature_bytes, status,
			 ipfs_cid, ceremony_id, pdf_hash, content_hash, field_name, jades_signature)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		signature.ContractDID, signature.SignerDID, signature.CredentialType, signature.SignatureBytes, signature.Status,
		signature.IpfsCID, signature.CeremonyID, signature.PDFHash, signature.ContentHash, signature.FieldName, signature.JAdESSignature,
	)
	if err != nil {
		return fmt.Errorf("could not create contract signature: %w", err)
	}

	return nil
}

// SetSignedPDF records the PAdES-signed PDF artefact CID, its C2PA lifecycle
// state, and its payload hash on the contract so ExportContractPdf/
// VerifyContractPdf recognize it as already up to date and serve it frozen —
// without that payload hash, the very first export/verify after signing would
// see a "" -> real-hash mismatch and append a post-signature revision to the
// PDF, which breaks standards-compliant PAdES validation even though the CMS
// signature itself stays intact.
func (r *PostgresContractRepo) SetSignedPDF(ctx context.Context, tx *sqlx.Tx, did, ipfsCID, rendererVersion, c2paState, payloadHash string) error {
	// NULLIF/COALESCE: a later multi-signer signature skips lifecycle
	// stamping (no fresh renderer version) — keep the stored one.
	_, err := tx.ExecContext(ctx, `
		UPDATE contracts
		   SET pdf_ipfs_cid = $2, pdf_renderer_version = COALESCE(NULLIF($3, ''), pdf_renderer_version), pdf_c2pa_state = $4, pdf_payload_hash = $5
		 WHERE did = $1`,
		did, ipfsCID, rendererVersion, c2paState, payloadHash,
	)
	if err != nil {
		return fmt.Errorf("could not set signed PDF for %s: %w", did, err)
	}
	return nil
}

func (r *PostgresContractRepo) RevokeSignature(ctx context.Context, tx *sqlx.Tx, did string, signerDID string) error {
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx,
		`UPDATE contract_signatures
		    SET status = 'REVOKED', revoked_at = $1
		  WHERE contract_did = $2 AND signer_did = $3 AND status != 'REVOKED'`,
		now, did, signerDID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: no revocable signature by signer %s on contract %s", db.ErrSignatureNotFound, signerDID, did)
	}
	return nil
}

// ReadLatestEnvelopeByContractDID retrieves the most recent non-revoked signature record for did.
func (r *PostgresContractRepo) ReadLatestEnvelopeByContractDID(ctx context.Context, tx *sqlx.Tx, did string) (*db.ContractSignatureEnvelope, error) {
	var signature db.ContractSignature
	err := tx.GetContext(ctx, &signature,
		`SELECT contract_did, signer_did, credential_type, status, signed_at, revoked_at, ipfs_cid
		   FROM contract_signatures
		  WHERE contract_did = $1
		  ORDER BY created_at DESC
		  LIMIT 1`,
		did,
	)
	if err != nil {
		return nil, err
	}
	env := &db.ContractSignatureEnvelope{
		ContractDID:    did,
		SignerDID:      signature.SignerDID,
		CredentialType: signature.CredentialType,
		Status:         signature.Status,
		IpfsCID:        signature.IpfsCID,
	}
	if signature.SignedAt != nil {
		t := signature.SignedAt.Format(time.RFC3339)
		env.SignedAt = &t
	}
	if signature.RevokedAt != nil {
		t := signature.RevokedAt.Format(time.RFC3339)
		env.RevokedAt = &t
	}
	return env, nil
}

func (r *PostgresContractRepo) ReadAllSigningTasks(ctx context.Context, tx *sqlx.Tx) ([]db.ContractSigningTask, error) {

	var tasks []db.ContractSigningTask
	err := tx.SelectContext(ctx, &tasks,
		`SELECT cs.contract_did, c.contract_version, cs.signer_did, cs.created_at
		   FROM contract_signatures cs
		   JOIN contracts c ON c.did = cs.contract_did
		  WHERE c.state = 'APPROVED' AND cs.status = 'PENDING'
		  ORDER BY cs.created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	return tasks, nil
}

func (r *PostgresContractRepo) CountSignatureForContractDID(ctx context.Context, tx *sqlx.Tx, did string) (int, error) {
	var count int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM contract_signatures WHERE contract_did=$1 AND status != 'REVOKED'`,
		did,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("count signatures: %w", err)
	}
	return count, nil
}

// FetchContractPDFBytes fetches the stored PDF bytes for a contract from IPFS.
func (r *PostgresContractRepo) FetchContractPDFBytes(ctx context.Context, tx *sqlx.Tx, did string) ([]byte, error) {
	var cidStr string
	_ = tx.QueryRowContext(ctx,
		`SELECT COALESCE(pdf_ipfs_cid, '') FROM contracts WHERE did = $1`, did,
	).Scan(&cidStr)
	if cidStr == "" {
		return nil, nil
	}
	return r.Artifacts.Get(ctx, artifactstore.ContractScope(did), cidStr)
}

func (r *PostgresContractRepo) CollectValidationFindings(ctx context.Context, tx *sqlx.Tx, did string) ([]string, error) {
	records, err := r.LoadSignatures(ctx, tx, did)
	if err != nil {
		return nil, fmt.Errorf("load signatures: %w", err)
	}

	findings := make([]string, 0)
	if len(records) == 0 {
		findings = append(findings, "No signatures found for the contract")
	}

	active := 0
	for _, rec := range records {
		status := strings.ToUpper(strings.TrimSpace(rec.Status))
		switch status {
		case "SIGNED":
			active++
		case "PENDING":
			findings = append(findings, "Pending signature detected")
		case "REVOKED":
			findings = append(findings, "Revoked signature detected")
		default:
			findings = append(findings, fmt.Sprintf("Unknown signature status: %s", rec.Status))
		}

		// A signing certificate revoked in the CRL invalidates the signature
		// regardless of its business-level status (DCS-OR-C2PA-007).
		if rec.CertRevokedAt != nil {
			findings = append(findings, fmt.Sprintf(
				"Signing certificate revoked (CRL): signer %s certificate was revoked at %s",
				rec.SignerDID, rec.CertRevokedAt.UTC().Format(time.RFC3339)))
		}
	}
	if active == 0 {
		findings = append(findings, "No active signatures available for validation")
	}

	pdfBytes, fetchErr := r.FetchContractPDFBytes(ctx, tx, did)
	if artifactstore.IsTampered(fetchErr) {
		// The embedded signing evidence lives inside the stored PDF, so bytes
		// that fail authenticated decryption invalidate the evidence itself —
		// report that, not a fetch problem.
		findings = append(findings,
			"Embedded signing evidence is invalid: the stored contract PDF failed authenticated decryption, so it was altered or substituted at rest")
	} else if fetchErr != nil {
		findings = append(findings, fmt.Sprintf("Could not fetch contract PDF for integrity check: %v", fetchErr))
	} else if len(pdfBytes) == 0 {
		findings = append(findings, "No contract PDF available for MR/HR integrity check")
	} else {
		// pdf-core /verify: re-renders JSON-LD and compares, validates C2PA chain.
		_, verifyErr := r.PDFCore.Verify(ctx, pdfBytes)
		if verifyErr != nil {
			findings = append(findings, fmt.Sprintf("Integrity check failed: %v", verifyErr))
		} else {
			findings = append(findings, "Document integrity check passed")
		}
	}

	if len(findings) == 0 {
		findings = append(findings, "Validation passed")
	}

	return findings, nil
}

func (r *PostgresContractRepo) LoadSignatures(ctx context.Context, tx *sqlx.Tx, did string) ([]db.SignatureRecord, error) {
	var records []db.SignatureRecord
	err := tx.SelectContext(ctx, &records,
		`SELECT signer_did, credential_type, status, signed_at, revoked_at, cert_revoked_at, field_name, jades_signature, ceremony_id
		   FROM contract_signatures
		  WHERE contract_did = $1
		  ORDER BY created_at`, did,
	)
	if err != nil {
		return nil, err
	}
	return records, nil
}

func (r *PostgresContractRepo) CollectComplianceFindings(ctx context.Context, tx *sqlx.Tx, did string) ([]string, error) {
	records, err := r.LoadSignatures(ctx, tx, did)
	if err != nil {
		return nil, fmt.Errorf("load signatures: %w", err)
	}

	findings := make([]string, 0)
	if len(records) == 0 {
		findings = append(findings, "No signatures found for compliance evaluation")
		return findings, nil
	}

	active := 0
	for _, rec := range records {
		status := strings.ToUpper(strings.TrimSpace(rec.Status))
		cred := strings.ToUpper(strings.TrimSpace(rec.CredentialType))

		if status == "REVOKED" {
			findings = append(findings, fmt.Sprintf("Signer %s has a revoked signature", rec.SignerDID))
			continue
		}
		if status != "SIGNED" {
			findings = append(findings, fmt.Sprintf("Signer %s signature not finalized (status=%s)", rec.SignerDID, rec.Status))
			continue
		}

		active++
		switch cred {
		case "SES", "AES", "QES":
			// Supported compliance levels.
		case "STUB", "":
			findings = append(findings, fmt.Sprintf("Signer %s uses non-production credential type '%s'", rec.SignerDID, rec.CredentialType))
		default:
			findings = append(findings, fmt.Sprintf("Signer %s uses unknown credential type '%s'", rec.SignerDID, rec.CredentialType))
		}
	}

	if active == 0 {
		findings = append(findings, "No active signed credentials satisfy compliance checks")
	}

	if len(findings) == 0 {
		findings = append(findings, "Compliance checks passed")
	}

	return findings, nil
}
