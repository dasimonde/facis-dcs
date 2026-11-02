package service

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	contractstoragearchive "digital-contracting-service/gen/contract_storage_archive"
	signaturemanagement "digital-contracting-service/gen/signature_management"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	baseevent "digital-contracting-service/internal/base/event"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/middleware"
)

// evidenceContentType is the Content-Type an evidence document is served
// with. The media type is what the uploader declared, so only a PDF keeps it;
// anything else is served as an opaque download and never rendered by a
// browser from the API origin.
func evidenceContentType(mediaType string) string {
	if strings.EqualFold(strings.TrimSpace(mediaType), "application/pdf") {
		return "application/pdf"
	}
	return "application/octet-stream"
}

// EvidenceContent serves one evidence document of an archived contract as a
// file (DCS-IR-CI-02). The read is recorded in the archive audit log.
func (s *contractStorageArchivesrvc) EvidenceContent(ctx context.Context, p *contractstoragearchive.EvidenceContentPayload) (res *contractstoragearchive.EvidenceContentResult, body io.ReadCloser, err error) {
	defer func() { s.auditFailure(ctx, "evidenceContent", p.Did, err) }()

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	if s.Artifacts == nil {
		return nil, nil, contractstoragearchive.MakeInternalError(errors.New("artifact store is not configured"))
	}
	localPeer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, nil, contractstoragearchive.MakeInternalError(err)
	}
	if err := authorizeContractRead(ctx, s.DB, localPeer, p.Did); err != nil {
		return nil, nil, err
	}
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, contractstoragearchive.MakeInternalError(err)
	}
	defer func() { _ = tx.Rollback() }()

	var row archiveEvidenceRow
	err = tx.GetContext(ctx, &row, `
		SELECT id, did, name, media_type, description, content_hash, content_cid, size_bytes, stored_by, stored_at
		FROM contract_archive_evidence WHERE did = $1 AND id::text = $2`, p.Did, p.EvidenceID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, contractstoragearchive.MakeNotFound(fmt.Errorf("no evidence %s for contract %s", p.EvidenceID, p.Did))
	}
	if err != nil {
		return nil, nil, contractstoragearchive.MakeInternalError(err)
	}
	content, err := s.Artifacts.Get(ctx, artifactstore.ContractScope(row.DID), row.ContentCID)
	if artifactstore.IsShredded(err) {
		return nil, nil, contractstoragearchive.MakeNotFound(fmt.Errorf("the keys of contract %s are shredded", p.Did))
	}
	if err != nil {
		return nil, nil, contractstoragearchive.MakeInternalError(fmt.Errorf("read evidence %s: %w", row.ID, err))
	}
	if err := baseevent.Create(ctx, tx, contractevents.RetrieveArchivedEvent{
		RetrievedBy:  middleware.GetParticipantID(ctx),
		HolderDID:    middleware.GetHolderDID(ctx),
		UserRoles:    middleware.GetUserRoles(ctx),
		ContractDIDs: []string{p.Did},
		OccurredAt:   time.Now().UTC(),
	}, componenttype.ContractStorageArchive); err != nil {
		return nil, nil, contractstoragearchive.MakeInternalError(err)
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, contractstoragearchive.MakeInternalError(err)
	}
	return &contractstoragearchive.EvidenceContentResult{ContentType: evidenceContentType(row.MediaType)}, io.NopCloser(bytes.NewReader(content)), nil
}

// PreparedDocument serves the to-be-signed PDF prepareSignature pinned on a
// ceremony as application/pdf (DCS-IR-CI-02), to the session that started the
// ceremony only.
func (s *signatureManagementsrvc) PreparedDocument(ctx context.Context, p *signaturemanagement.PreparedDocumentPayload) (io.ReadCloser, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := s.DB.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	ceremony, err := s.CeremonyRepo.GetCeremonyByID(ctx, tx, p.CeremonyID)
	_ = tx.Rollback()
	if err != nil {
		return nil, signaturemanagement.MakeInternalError(err)
	}
	if ceremony == nil || len(ceremony.PreparedPDF) == 0 {
		return nil, signaturemanagement.MakeNotFound(fmt.Errorf("ceremony %s has no prepared document", p.CeremonyID))
	}
	if ceremony.RequestedBy != middleware.GetParticipantID(ctx) || ceremony.RequestedHolderDID != middleware.GetHolderDID(ctx) {
		return nil, signaturemanagement.MakeForbidden(fmt.Errorf("the signing ceremony %s was started by another session", p.CeremonyID))
	}
	if err := authorizeContractReadDocument(ctx, s.DB, s.DIDDocument, ceremony.ContractDID); err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(ceremony.PreparedPDF)), nil
}
