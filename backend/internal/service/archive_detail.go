package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	archive "digital-contracting-service/gen/contract_storage_archive"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	baseevent "digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/contractworkflowengine/command"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	"digital-contracting-service/internal/middleware"
)

// Detail returns an archived snapshot. The read is recorded in the archive
// audit log, a failed read too (DCS-FR-CSA-18).
func (s *contractStorageArchivesrvc) Detail(ctx context.Context, p *archive.DetailPayload) (res any, err error) {
	defer func() { s.auditFailure(ctx, "detail", p.Did, err) }()
	if err := authorizeContractReadDocument(ctx, s.DB, s.DIDDocument, p.Did); err != nil {
		return nil, err
	}
	var entry struct {
		CID  string `db:"snapshot_cid"`
		Hash string `db:"content_hash"`
	}
	err = s.DB.GetContext(ctx, &entry, `SELECT snapshot_cid, content_hash FROM contract_archive_entries WHERE did=$1 AND contract_version=$2 AND deleted_at IS NULL AND archive_status <> 'DELETED'`, p.Did, p.ContractVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, archive.MakeNotFound(err)
	}
	if err != nil {
		return nil, archive.MakeInternalError(err)
	}
	if entry.CID == "" || s.ATrailReader.Artifacts == nil {
		return nil, archive.MakeNotFound(errors.New("archive snapshot unavailable"))
	}
	raw, err := s.ATrailReader.Artifacts.Get(ctx, artifactstore.ContractScope(p.Did), entry.CID)
	if err != nil {
		return nil, archive.MakeNotFound(errors.New("archive snapshot unavailable"))
	}
	hash, err := command.HashArchiveSnapshot(datatype.JSON(raw))
	if err != nil || hash != entry.Hash {
		return nil, archive.MakeInternalError(errors.New("archive snapshot integrity mismatch"))
	}
	var identity struct {
		DID     string `json:"did"`
		Version int    `json:"contract_version"`
	}
	if err := json.Unmarshal(raw, &identity); err != nil || identity.DID != p.Did || identity.Version != p.ContractVersion {
		return nil, archive.MakeInternalError(fmt.Errorf("archive snapshot identity mismatch"))
	}
	var snapshot any
	if err := json.Unmarshal(raw, &snapshot); err != nil {
		return nil, archive.MakeInternalError(err)
	}
	if err := s.auditRetrieval(ctx, p.Did); err != nil {
		return nil, archive.MakeInternalError(err)
	}
	return snapshot, nil
}

// auditRetrieval records that the caller received the archived contract.
func (s *contractStorageArchivesrvc) auditRetrieval(ctx context.Context, did string) error {
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	evt := contractevents.RetrieveArchivedEvent{
		RetrievedBy:  middleware.GetParticipantID(ctx),
		HolderDID:    middleware.GetHolderDID(ctx),
		UserRoles:    middleware.GetUserRoles(ctx),
		ContractDIDs: []string{did},
		OccurredAt:   time.Now().UTC(),
	}
	if err := baseevent.Create(ctx, tx, evt, componenttype.ContractStorageArchive); err != nil {
		return err
	}
	return tx.Commit()
}
