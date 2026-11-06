package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	cwe "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/contractworkflowengine/command"
	"digital-contracting-service/internal/middleware"
)

func (s *contractWorkflowEnginesrvc) Store(ctx context.Context, req *cwe.ContractStoreRequest) (*cwe.ContractStoreResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()
	if err := authorizeContractWrite(ctx, s.DB, s.DIDDocument, req.Did); err != nil {
		return nil, err
	}
	updated, err := time.Parse(time.RFC3339, req.UpdatedAt)
	if err != nil {
		return nil, cwe.MakeBadRequest(err)
	}
	peer, err := s.DIDDocument.GetID()
	if err != nil {
		return nil, cwe.MakeInternalError(err)
	}
	media := "application/octet-stream"
	if req.MediaType != nil && *req.MediaType != "" {
		media = *req.MediaType
	}
	handler := command.EvidenceRecorder{DB: s.DB, CRepo: s.CRepo, DIDDocument: s.DIDDocument, Artifacts: s.ATrailReader.Artifacts}
	id, err := handler.Handle(ctx, command.RecordEvidenceCmd{DID: req.Did, UpdatedAt: updated, CauserDID: peer, RecordedBy: middleware.GetParticipantID(ctx), HolderDID: middleware.GetHolderDID(ctx), UserRoles: middleware.GetUserRoles(ctx), Evidence: command.Evidence{Content: req.Content, Filename: req.Filename, Description: req.Description, MediaType: media}})
	if err != nil {
		if errors.Is(err, command.ErrInvalidEvidence) || errors.Is(err, command.ErrEvidenceTerminated) || errors.Is(err, command.ErrEvidenceCreatorDraftOnly) {
			return nil, cwe.MakeBadRequest(err)
		}
		if errors.Is(err, base.ErrUpdatedElsewhere) {
			return nil, cwe.MakeConflict(err)
		}
		return nil, evidenceError(err)
	}
	return &cwe.ContractStoreResponse{Did: req.Did, EvidenceID: id}, nil
}

func evidenceError(err error) error {
	if errors.Is(err, sql.ErrNoRows) || artifactstore.IsShredded(err) {
		return cwe.MakeNotFound(err)
	}
	return cwe.MakeInternalError(err)
}

func (s *contractWorkflowEnginesrvc) readEvidence(ctx context.Context, did, id, cid string) (*cwe.ContractEvidence, error) {
	if s.ATrailReader.Artifacts == nil {
		return nil, cwe.MakeInternalError(errors.New("evidence artifact store unavailable"))
	}
	raw, err := s.ATrailReader.Artifacts.Get(ctx, artifactstore.ContractScope(did), cid)
	if err != nil {
		return nil, evidenceError(err)
	}
	var e command.Evidence
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, evidenceError(err)
	}
	if err := e.Verify(did, id); err != nil {
		return nil, evidenceError(err)
	}
	return &cwe.ContractEvidence{EvidenceID: e.EvidenceID, Did: e.DID, ContractVersion: e.ContractVersion, RecordedBy: e.RecordedBy, RecordedAt: e.RecordedAt, Filename: e.Filename, Description: e.Description, MediaType: e.MediaType, Sha256: e.SHA256, Content: e.Content}, nil
}

func (s *contractWorkflowEnginesrvc) ListEvidence(ctx context.Context, req *cwe.ListEvidencePayload) ([]*cwe.ContractEvidence, error) {
	if err := authorizeContractReadDocument(ctx, s.DB, s.DIDDocument, req.Did); err != nil {
		return nil, err
	}
	var rows []struct {
		ID  string `db:"evidence_id"`
		CID string `db:"artifact_cid"`
	}
	if err := s.DB.SelectContext(ctx, &rows, `SELECT evidence_id, artifact_cid FROM contract_evidence WHERE contract_did=$1 ORDER BY evidence_id`, req.Did); err != nil {
		return nil, evidenceError(err)
	}
	result := make([]*cwe.ContractEvidence, 0, len(rows))
	for _, row := range rows {
		e, err := s.readEvidence(ctx, req.Did, row.ID, row.CID)
		if err != nil {
			return nil, err
		}
		e.Content = nil
		result = append(result, e)
	}
	return result, nil
}

func (s *contractWorkflowEnginesrvc) RetrieveEvidence(ctx context.Context, req *cwe.RetrieveEvidencePayload) (*cwe.ContractEvidence, error) {
	if err := authorizeContractReadDocument(ctx, s.DB, s.DIDDocument, req.Did); err != nil {
		return nil, err
	}
	var cid string
	if err := s.DB.GetContext(ctx, &cid, `SELECT artifact_cid FROM contract_evidence WHERE contract_did=$1 AND evidence_id=$2`, req.Did, req.EvidenceID); err != nil {
		return nil, evidenceError(err)
	}
	return s.readEvidence(ctx, req.Did, req.EvidenceID, cid)
}
