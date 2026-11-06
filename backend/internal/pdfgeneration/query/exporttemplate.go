package query

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	tpldb "digital-contracting-service/internal/templaterepository/db"
)

type ExportTemplatePdfQry struct {
	DID string
}

type ExportTemplatePdfHandler struct {
	DB        *sqlx.DB
	TRepo     tpldb.ContractTemplateRepo
	Artifacts *artifactstore.Store
	PDFCore   *pdfcore.Client
	VCIssuer  provenance.VCIssuer
	IssuerDID string
}

// Handle serves the template PDF the event-driven regenerator stored. It waits
// while a regeneration of the latest change is in flight and never renders or
// appends on demand: the regenerator is the only writer, under its
// per-template lock, and records the event's reason in the lifecycle
// assertion.
func (h *ExportTemplatePdfHandler) Handle(ctx context.Context, qry ExportTemplatePdfQry) (io.ReadCloser, error) {
	deadline := time.Now().Add(pdfExportWaitTimeout)
	for {
		tpl, pdfState, err := h.read(ctx, qry.DID)
		if err != nil {
			return nil, err
		}
		var jsonldBytes []byte
		if tpl.TemplateData != nil {
			jsonldBytes = []byte(*tpl.TemplateData)
		}
		currentC2PAState, err := provenance.MapCWEStateToC2PA(tpl.State)
		if err != nil {
			return nil, fmt.Errorf("map template state %q to C2PA state: %w", tpl.State, err)
		}
		if pdfState.IPFSCID != "" && pdfState.C2PAState == currentC2PAState && pdfState.PayloadHash == payloadHash(jsonldBytes) {
			pdf, err := h.Artifacts.Get(ctx, artifactstore.TemplateScope(qry.DID), pdfState.IPFSCID)
			if err != nil || len(pdf) == 0 {
				return nil, fmt.Errorf("fetch PDF from IPFS %s for template %s: %w", pdfState.IPFSCID, qry.DID, err)
			}
			return io.NopCloser(bytes.NewReader(pdf)), nil
		}
		if time.Now().After(deadline) {
			log.Printf("pdfgeneration: export of template %s timed out waiting for regeneration: cid=%q c2pa(pdf=%q want=%q)",
				qry.DID, pdfState.IPFSCID, pdfState.C2PAState, currentC2PAState)
			return nil, fmt.Errorf("template %s PDF is being regenerated after the latest change; retry shortly", qry.DID)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(pdfExportPollInterval):
		}
	}
}

func (h *ExportTemplatePdfHandler) read(ctx context.Context, did string) (*tpldb.ContractTemplate, *tpldb.ContractTemplatePDFState, error) {
	tx, err := h.DB.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	tpl, err := h.TRepo.ReadDataByID(ctx, tx, did)
	if err != nil {
		return nil, nil, fmt.Errorf("template %s: %w", did, err)
	}
	pdfState, err := h.TRepo.ReadPDFState(ctx, tx, did)
	if err != nil {
		return nil, nil, fmt.Errorf("read cached template PDF state for %s: %w", did, err)
	}
	return tpl, pdfState, nil
}
