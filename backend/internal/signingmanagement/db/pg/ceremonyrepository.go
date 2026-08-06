package pg

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/signingmanagement/db"
)

// PostgresCeremonyRepo persists signing ceremonies in Postgres.
type PostgresCeremonyRepo struct{}

func (r *PostgresCeremonyRepo) CreateCeremony(ctx context.Context, tx *sqlx.Tx, c db.SignatureCeremony) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO signature_ceremonies
			(id, contract_did, field_name, requested_by, status, wallet_uri, nonce, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		c.ID, c.ContractDID, c.FieldName, c.RequestedBy, c.Status, c.WalletURI, c.Nonce, c.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("create signature ceremony: %w", err)
	}
	return nil
}

func (r *PostgresCeremonyRepo) GetCeremonyByID(ctx context.Context, tx *sqlx.Tx, id string) (*db.SignatureCeremony, error) {
	var c db.SignatureCeremony
	err := tx.GetContext(ctx, &c, `
		SELECT id, contract_did, field_name, requested_by, status, wallet_uri, nonce,
		       signer_did, vp_token, pid_claims, kb_sd_hash, created_at, verified_at, expires_at,
		       prepared_pdf, prepared_pdf_sha256, request_nonce, request_expires_at, credential_type,
		       published_by, published_holder_did, published_roles, consumed_at,
		       poa_organization, poa_roles,
		       pinned_payload, pinned_payload_sha256, pinned_content_hash, pinned_renderer_version,
		       pinned_signed_count, pinned_contract_version, required_credential_type,
		       signer_cert_subject, signer_cert_serial
		  FROM signature_ceremonies
		 WHERE id = $1`, id,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get signature ceremony %s: %w", id, err)
	}
	return &c, nil
}

// StorePreparedRequest persists the OID4VP Document-Retrieval publish state
// (request nonce/expiry, the qualifier the JAR asked for, the publishing
// signer's context). It does NOT touch prepared_pdf/prepared_pdf_sha256 or the
// pinned_* columns — those are pinned once, by PinPreparedBytes, from inside
// the SAME Prepare() call that produced the document this request publishes
// (ADR-20): publish never re-derives or re-pins the to-be-signed bytes.
func (r *PostgresCeremonyRepo) StorePreparedRequest(ctx context.Context, tx *sqlx.Tx, req db.PreparedRequest) error {
	res, err := tx.ExecContext(ctx, `
		UPDATE signature_ceremonies
		   SET request_nonce = $2, request_expires_at = $3, credential_type = $4,
		       published_by = $5, published_holder_did = $6, published_roles = $7,
		       consumed_at = NULL
		 WHERE id = $1 AND status = $8`,
		req.CeremonyID, req.RequestNonce, req.RequestExpiresAt, req.CredentialType,
		req.PublishedBy, req.HolderDID, req.Roles, db.CeremonyVerified,
	)
	if err != nil {
		return fmt.Errorf("store prepared request for ceremony %s: %w", req.CeremonyID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("store prepared request for ceremony %s rows: %w", req.CeremonyID, err)
	}
	if affected == 0 {
		return fmt.Errorf("ceremony %s is not in %q state", req.CeremonyID, db.CeremonyVerified)
	}
	return nil
}

// PinPreparedBytes persists the exact to-be-signed PDF and JAdES payload, plus
// the finalize metadata derived alongside them, at every prepare (ADR-20). A
// later prepare on the same ceremony overwrites the pin with fresh bytes; it
// does not touch the publish-specific columns StorePreparedRequest owns.
func (r *PostgresCeremonyRepo) PinPreparedBytes(ctx context.Context, tx *sqlx.Tx, p db.PinnedBytes) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE signature_ceremonies
		   SET prepared_pdf = $2, prepared_pdf_sha256 = $3,
		       pinned_payload = $4, pinned_payload_sha256 = $5,
		       pinned_content_hash = $6, pinned_renderer_version = $7,
		       pinned_signed_count = $8, pinned_contract_version = $9,
		       required_credential_type = $10
		 WHERE id = $1`,
		p.CeremonyID, p.PreparedPDF, p.PreparedPDFSHA256,
		p.PinnedPayload, p.PinnedPayloadSHA256,
		p.PinnedContentHash, p.PinnedRendererVersion,
		p.PinnedSignedCount, p.PinnedContractVersion, p.RequiredCredentialType,
	)
	if err != nil {
		return fmt.Errorf("pin prepared bytes for ceremony %s: %w", p.CeremonyID, err)
	}
	return nil
}

// RecordSignerCertificate persists the validated signing certificate's
// subject and serial on the ceremony (sole control evidence, DCS-FR-SM-26).
func (r *PostgresCeremonyRepo) RecordSignerCertificate(ctx context.Context, tx *sqlx.Tx, ceremonyID, subject, serial string) error {
	_, err := tx.ExecContext(ctx, `
		UPDATE signature_ceremonies
		   SET signer_cert_subject = $2, signer_cert_serial = $3
		 WHERE id = $1`,
		ceremonyID, subject, serial,
	)
	if err != nil {
		return fmt.Errorf("record signer certificate for ceremony %s: %w", ceremonyID, err)
	}
	return nil
}

func (r *PostgresCeremonyRepo) MarkCeremonyConsumed(ctx context.Context, tx *sqlx.Tx, id string) error {
	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `
		UPDATE signature_ceremonies
		   SET consumed_at = $2
		 WHERE id = $1 AND consumed_at IS NULL`,
		id, now,
	)
	if err != nil {
		return fmt.Errorf("mark ceremony %s consumed: %w", id, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark ceremony %s consumed rows: %w", id, err)
	}
	if affected == 0 {
		return fmt.Errorf("ceremony %s signing request is already consumed", id)
	}
	return nil
}

func (r *PostgresCeremonyRepo) MarkCeremonyVerified(ctx context.Context, tx *sqlx.Tx, v db.VerifiedPresentation) error {
	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `
		UPDATE signature_ceremonies
		   SET status = $2, signer_did = $3, vp_token = $4, pid_claims = $5, kb_sd_hash = $6, verified_at = $7,
		       poa_organization = $9, poa_roles = $10, poa_vp_token = $11
		 WHERE id = $1 AND status = $8`,
		v.CeremonyID, db.CeremonyVerified, v.SignerDID, v.VpToken, v.PidClaims, v.KbSdHash, now, db.CeremonyPending,
		v.PoAOrganization, v.PoARoles, v.PoAVpToken,
	)
	if err != nil {
		return fmt.Errorf("mark ceremony %s verified: %w", v.CeremonyID, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("mark ceremony %s verified rows: %w", v.CeremonyID, err)
	}
	if affected == 0 {
		return fmt.Errorf("ceremony %s is not in %q state", v.CeremonyID, db.CeremonyPending)
	}
	return nil
}

func (r *PostgresCeremonyRepo) FindVerifiedCeremonyByField(ctx context.Context, tx *sqlx.Tx, contractDID, fieldName string) (*db.SignatureCeremony, error) {
	var c db.SignatureCeremony
	err := tx.GetContext(ctx, &c, `
		SELECT id, contract_did, field_name, requested_by, status, wallet_uri, nonce,
		       signer_did, vp_token, pid_claims, kb_sd_hash, created_at, verified_at, expires_at,
		       poa_organization, poa_roles, poa_vp_token
		  FROM signature_ceremonies
		 WHERE contract_did = $1 AND field_name = $2 AND status = $3
		 ORDER BY verified_at DESC NULLS LAST
		 LIMIT 1`,
		contractDID, fieldName, db.CeremonyVerified,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find verified ceremony for %s field %q: %w", contractDID, fieldName, err)
	}
	return &c, nil
}

func (r *PostgresCeremonyRepo) FindVerifiedCeremony(ctx context.Context, tx *sqlx.Tx, contractDID, signerDID string) (*db.SignatureCeremony, error) {
	var c db.SignatureCeremony
	err := tx.GetContext(ctx, &c, `
		SELECT id, contract_did, field_name, requested_by, status, wallet_uri, nonce,
		       signer_did, vp_token, pid_claims, kb_sd_hash, created_at, verified_at, expires_at,
		       poa_organization, poa_roles, poa_vp_token
		  FROM signature_ceremonies
		 WHERE contract_did = $1 AND signer_did = $2 AND status = $3
		 ORDER BY verified_at DESC NULLS LAST
		 LIMIT 1`,
		contractDID, signerDID, db.CeremonyVerified,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("find verified ceremony for %s/%s: %w", contractDID, signerDID, err)
	}
	return &c, nil
}

// RecordSummaryVC retains the signing summary issued for a ceremony. The
// embedded copy inside the PDF is what travels; this one is what the compliance
// viewer reads per ceremony, and it outlives a PDF this instance no longer holds.
func (r *PostgresCeremonyRepo) RecordSummaryVC(ctx context.Context, tx *sqlx.Tx, ceremonyID string, summary []byte) error {
	_, err := tx.ExecContext(ctx, `UPDATE signature_ceremonies SET summary_vc = $2 WHERE id = $1`, ceremonyID, string(summary))
	if err != nil {
		return fmt.Errorf("record signing summary for ceremony %s: %w", ceremonyID, err)
	}
	return nil
}
