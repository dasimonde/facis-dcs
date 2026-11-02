package service

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"

	c2paservice "digital-contracting-service/gen/c2_pa_service"
	"digital-contracting-service/internal/base/artifactstore"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"

	"github.com/jmoiron/sqlx"
)

// c2paManifestMediaType is the C2PA manifest store media type (DCS-OR-C2PA-008).
const c2paManifestMediaType = "application/c2pa"

type c2paSrvc struct {
	DB        *sqlx.DB
	Artifacts *artifactstore.Store
	CRepo     cwedb.ContractRepo
	PDFCore   *pdfcore.Client
}

// NewC2PAService wires the public C2PA manifest endpoint (DCS-OR-C2PA-008). It
// serves the manifest store of the contract PDF this instance stores.
func NewC2PAService(
	db *sqlx.DB,
	artifacts *artifactstore.Store,
	cRepo cwedb.ContractRepo,
	pdfCore *pdfcore.Client,
) c2paservice.Service {
	if pdfCore == nil {
		panic("PDFCore client is required")
	}
	return &c2paSrvc{
		DB:        db,
		Artifacts: artifacts,
		CRepo:     cRepo,
		PDFCore:   pdfCore,
	}
}

// GetManifest returns the raw C2PA JUMBF manifest store of the stored PDF of a
// signed contract (Content-Type application/c2pa), or — when ?history=true — a
// parsed JSON enumeration of the manifest chain.
//
// The endpoint is public, so it never waits for a regeneration in flight: it
// serves the stored PDF as it stands and answers not found when there is none.
func (s *c2paSrvc) GetManifest(ctx context.Context, p *c2paservice.GetManifestPayload) (*c2paservice.GetManifestResult, io.ReadCloser, error) {
	pdfBytes, err := s.storedPDF(ctx, p.ContractDid)
	if err != nil {
		if artifactstore.IsShredded(err) || isNotFoundErr(err) {
			return nil, nil, c2paservice.MakeNotFound(err)
		}
		return nil, nil, c2paservice.MakeInternalError(fmt.Errorf("read the stored PDF of %s for its C2PA manifest: %w", p.ContractDid, err))
	}

	if p.History != nil && *p.History {
		chain, err := s.PDFCore.ExtractManifestChain(ctx, pdfBytes)
		if err != nil {
			return nil, nil, c2paservice.MakeInternalError(fmt.Errorf("C2PA manifest chain for %s: %w", p.ContractDid, err))
		}
		body, err := json.Marshal(chain)
		if err != nil {
			return nil, nil, c2paservice.MakeInternalError(fmt.Errorf("encode C2PA manifest chain for %s: %w", p.ContractDid, err))
		}
		ct := "application/json"
		return &c2paservice.GetManifestResult{ContentType: &ct}, io.NopCloser(bytes.NewReader(body)), nil
	}

	manifestBytes, err := s.PDFCore.ExtractManifest(ctx, pdfBytes)
	if err != nil {
		return nil, nil, c2paservice.MakeInternalError(fmt.Errorf("extract C2PA manifest for %s: %w", p.ContractDid, err))
	}
	if len(manifestBytes) == 0 {
		return nil, nil, c2paservice.MakeNotFound(fmt.Errorf("no C2PA manifest embedded in PDF for contract %s", p.ContractDid))
	}

	ct := c2paManifestMediaType
	return &c2paservice.GetManifestResult{ContentType: &ct}, io.NopCloser(bytes.NewReader(manifestBytes)), nil
}

// storedPDF returns the contract PDF this instance stores for did.
func (s *c2paSrvc) storedPDF(ctx context.Context, did string) ([]byte, error) {
	tx, err := s.DB.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	state, err := s.CRepo.ReadPDFState(ctx, tx, did)
	_ = tx.Rollback()
	if err != nil {
		return nil, err
	}
	if state.IPFSCID == "" {
		return nil, fmt.Errorf("contract %s has no stored PDF: not found", did)
	}
	// The manifest of a contract that is not signed names its drafts and
	// negotiation states and their reasons. It is served to anyone only once the
	// contract is signed; before that its parties read the chain through the
	// authenticated GET /signature/provenance/{did} (UC-IF-32).
	if !state.Signed {
		return nil, fmt.Errorf("contract %s is not signed: not found", did)
	}
	return s.Artifacts.Get(ctx, artifactstore.ContractScope(did), state.IPFSCID)
}
