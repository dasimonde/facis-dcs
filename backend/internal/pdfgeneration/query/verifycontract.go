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
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/pdfgeneration/provenance"
)

type VerifyContractPdfQry struct {
	DID string
}

type VerifyContractPdfHandler struct {
	DB        *sqlx.DB
	CRepo     cwedb.ContractRepo
	Artifacts *artifactstore.Store
	PDFCore   *pdfcore.Client
	// Credentials verifies the lifecycle credential embedded in the PDF against
	// the key its issuer publishes for assertions.
	Credentials *provenance.CredentialVerifier
	// CredentialStatus resolves that credential's revocation entry against the
	// signed status list it names.
	CredentialStatus *provenance.CredentialStatusVerifier
	// Signatures validates the PAdES signatures of the PDF; nil when no EU DSS
	// is configured, which the verifier reports as indeterminate.
	Signatures PDFSignatureValidator
	// SignerRoots anchor the C2PA manifest signers.
	SignerRoots *x509.CertPool
}

// signerAnchors anchors the manifest signers of contract to the instances
// party to it.
func (h *VerifyContractPdfHandler) signerAnchors(contract *cwedb.Contract) SignerAnchors {
	anchors := SignerAnchors{Roots: h.SignerRoots}
	if contract.Responsible != nil {
		anchors.Issuers = contract.Responsible.GetParties()
	}
	return anchors
}

func (h *VerifyContractPdfHandler) Handle(ctx context.Context, qry VerifyContractPdfQry) (*pdfgen.PDFVerifyResult, error) {
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	contract, err := h.CRepo.ReadDataByDID(ctx, tx, qry.DID)
	if err != nil {
		return nil, fmt.Errorf("contract %s: %w", qry.DID, err)
	}

	pdfState, err := h.CRepo.ReadPDFState(ctx, tx, qry.DID)
	if err != nil {
		return nil, fmt.Errorf("read contract PDF verification state for %s: %w", qry.DID, err)
	}

	currentC2PAState, err := provenance.MapCWEStateToC2PA(contract.State)
	if err != nil {
		return nil, fmt.Errorf("map contract state %q to C2PA state: %w", contract.State, err)
	}

	// Verify is read-only. It verifies the stored PDF as it stands and never
	// appends to it or changes its bookkeeping: the regenerator is the only
	// writer, under its per-contract lock. While a regeneration is in flight
	// the stored PDF can lag the contract's state; lifecycle_status reports the
	// contract's state, and the checks report what the stored PDF holds.
	if pdfState.IPFSCID == "" {
		return nil, fmt.Errorf("no cached PDF for contract %s; call export first", qry.DID)
	}
	pdf, err := h.Artifacts.Get(ctx, artifactstore.ContractScope(qry.DID), pdfState.IPFSCID)
	if artifactstore.IsTampered(err) {
		return tamperedVerifyResult(currentC2PAState), nil
	}
	if err != nil || len(pdf) == 0 {
		return nil, fmt.Errorf("fetch contract PDF %s from IPFS for verify: %w", qry.DID, err)
	}
	return runVerify(ctx, pdf, h.PDFCore, h.Credentials, h.CredentialStatus, h.Signatures, h.signerAnchors(contract), currentC2PAState)
}
