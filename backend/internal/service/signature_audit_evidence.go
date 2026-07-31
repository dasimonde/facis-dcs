package service

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/base/datatype/componenttype"
)

// signatureEvidenceRecord is a safe projection of the durable signature
// record. It deliberately excludes signature_bytes and the JAdES token: PAC
// needs verifiable provenance metadata, not another copy of secret-bearing or
// potentially large cryptographic material.
type signatureEvidenceRecord struct {
	ID             int64          `db:"id"`
	ContractDID    string         `db:"contract_did"`
	SignerDID      string         `db:"signer_did"`
	CredentialType string         `db:"credential_type"`
	Status         string         `db:"status"`
	SignedAt       sql.NullTime   `db:"signed_at"`
	RevokedAt      sql.NullTime   `db:"revoked_at"`
	CreatedAt      time.Time      `db:"created_at"`
	IPFSCID        sql.NullString `db:"ipfs_cid"`
	CeremonyID     sql.NullString `db:"ceremony_id"`
	PDFHash        sql.NullString `db:"pdf_hash"`
	ContentHash    sql.NullString `db:"content_hash"`
	FieldName      sql.NullString `db:"field_name"`
}

func (s *processAuditAndCompliancesrvc) collectSignatureEvidence(ctx context.Context, did string) ([]*auditEvidenceResource, error) {
	if s.DB == nil {
		return nil, fmt.Errorf("signature evidence persistence is not configured")
	}
	records := make([]signatureEvidenceRecord, 0)
	if err := s.DB.SelectContext(ctx, &records, `
		SELECT id, contract_did, signer_did, credential_type, status,
		       signed_at, revoked_at, created_at, ipfs_cid,
		       ceremony_id::text AS ceremony_id, pdf_hash, content_hash, field_name
		  FROM contract_signatures
		 WHERE ($1 = '' OR contract_did = $1)
		 ORDER BY contract_did, created_at, id`, did); err != nil {
		return nil, fmt.Errorf("read durable signature evidence: %w", err)
	}

	byDID := make(map[string]*auditEvidenceResource)
	order := make([]string, 0)
	for _, record := range records {
		resource := byDID[record.ContractDID]
		if resource == nil {
			resource = &auditEvidenceResource{
				Did: record.ContractDID, Component: componenttype.SignatureManagement.String(),
				CreatedAt:  record.CreatedAt.UTC().Format(time.RFC3339),
				AuditTrail: []*processauditandcompliance.PACResourceAuditTrailEntry{},
			}
			byDID[record.ContractDID] = resource
			order = append(order, record.ContractDID)
		}
		resource.AuditTrail = append(resource.AuditTrail, signatureEvidenceEntry(record))
	}
	result := make([]*auditEvidenceResource, 0, len(order))
	for _, contractDID := range order {
		result = append(result, byDID[contractDID])
	}
	return result, nil
}

func signatureEvidenceEntry(record signatureEvidenceRecord) *processauditandcompliance.PACResourceAuditTrailEntry {
	data := map[string]any{
		"signatureId": record.ID,
		"signerDid":   record.SignerDID, "credentialType": record.CredentialType,
		"status": record.Status,
	}
	putNullString(data, "ipfsCid", record.IPFSCID)
	putNullString(data, "ceremonyId", record.CeremonyID)
	putNullString(data, "pdfHash", record.PDFHash)
	putNullString(data, "contentHash", record.ContentHash)
	putNullString(data, "fieldName", record.FieldName)
	if record.SignedAt.Valid {
		data["signedAt"] = record.SignedAt.Time.UTC().Format(time.RFC3339)
	}
	if record.RevokedAt.Valid {
		data["revokedAt"] = record.RevokedAt.Time.UTC().Format(time.RFC3339)
	}
	did := record.ContractDID
	kind := "TIMELINE"
	return &processauditandcompliance.PACResourceAuditTrailEntry{
		ID: record.ID, Component: componenttype.SignatureManagement.String(),
		EventType: "SIGNATURE_EVIDENCE", EventData: data, Did: &did,
		CreatedAt: record.CreatedAt.UTC().Format(time.RFC3339), Kind: &kind,
	}
}

func mergeAuditEvidenceResources(base, additions []*auditEvidenceResource) []*auditEvidenceResource {
	index := make(map[string]*auditEvidenceResource, len(base)+len(additions))
	for _, resource := range base {
		if resource != nil {
			index[resource.Component+"\x00"+resource.Did] = resource
		}
	}
	for _, resource := range additions {
		if resource == nil {
			continue
		}
		key := resource.Component + "\x00" + resource.Did
		if existing := index[key]; existing != nil {
			existing.AuditTrail = append(existing.AuditTrail, resource.AuditTrail...)
			continue
		}
		base = append(base, resource)
		index[key] = resource
	}
	return base
}

func putNullString(target map[string]any, key string, value sql.NullString) {
	if value.Valid && value.String != "" {
		target[key] = value.String
	}
}
