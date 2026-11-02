package query

import (
	"context"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"github.com/jmoiron/sqlx"

	pdfgen "digital-contracting-service/gen/pdf_generation"
	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	tpldb "digital-contracting-service/internal/templaterepository/db"
)

type VerifyTemplatePdfQry struct {
	DID string
}

type VerifyTemplatePdfHandler struct {
	DB        *sqlx.DB
	TRepo     tpldb.ContractTemplateRepo
	Artifacts *artifactstore.Store
	PDFCore   *pdfcore.Client
	VCIssuer  provenance.VCIssuer
	IssuerDID string
	// Credentials verifies the lifecycle credential embedded in the PDF against
	// the key its issuer publishes for assertions.
	Credentials *provenance.CredentialVerifier
	// CredentialStatus resolves that credential's revocation entry against the
	// signed status list it names.
	CredentialStatus *provenance.CredentialStatusVerifier
	// SignerRoots anchor the C2PA manifest signers. A template is signed by this
	// instance only.
	SignerRoots *x509.CertPool
}

func (h *VerifyTemplatePdfHandler) signerAnchors() SignerAnchors {
	return SignerAnchors{Roots: h.SignerRoots, Issuers: []string{h.IssuerDID}}
}

func (h *VerifyTemplatePdfHandler) Handle(ctx context.Context, qry VerifyTemplatePdfQry) (*pdfgen.PDFVerifyResult, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	tpl, err := h.TRepo.ReadDataByID(ctx, tx, qry.DID)
	if err != nil {
		return nil, fmt.Errorf("template %s: %w", qry.DID, err)
	}

	pdfState, err := h.TRepo.ReadPDFState(ctx, tx, qry.DID)
	if err != nil {
		return nil, fmt.Errorf("read template PDF verification state for %s: %w", qry.DID, err)
	}

	currentC2PAState, err := provenance.MapCWEStateToC2PA(tpl.State)
	if err != nil {
		return nil, fmt.Errorf("map template state %q to C2PA state: %w", tpl.State, err)
	}

	// Verify is read-only, as for a contract: the regenerator is the only
	// writer of the template PDF, under its per-template lock. While a
	// regeneration is in flight the stored PDF can lag the template's state;
	// lifecycle_status reports the template's state, and the checks report
	// what the stored PDF holds.
	latestCID := pdfState.IPFSCID
	if latestCID == "" {
		return nil, fmt.Errorf("no cached PDF for template %s; call export first", qry.DID)
	}

	pdf, err := h.Artifacts.Get(ctx, artifactstore.TemplateScope(qry.DID), latestCID)
	if artifactstore.IsTampered(err) {
		return tamperedVerifyResult(currentC2PAState), nil
	}
	if err != nil || len(pdf) == 0 {
		return nil, fmt.Errorf("fetch template PDF %s from IPFS for verify: %w", qry.DID, err)
	}

	return runVerify(ctx, pdf, h.PDFCore, h.Credentials, h.CredentialStatus, nil, h.signerAnchors(), currentC2PAState)
}
