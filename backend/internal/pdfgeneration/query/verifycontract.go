package query

import (
	"context"
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
	VCIssuer  provenance.VCIssuer
	IssuerDID string
	// Credentials verifies the lifecycle credential embedded in the PDF against
	// the key its issuer publishes for assertions.
	Credentials *provenance.CredentialVerifier
	// CredentialStatus resolves that credential's revocation entry against the
	// signed status list it names.
	CredentialStatus *provenance.CredentialStatusVerifier
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

	latestCID := pdfState.IPFSCID

	if pdfState.IPFSCID != "" && provenance.IsFrozenC2PAState(pdfState.C2PAState) {
		// Already PAdES-signed: never mutate this PDF's embedded attachments
		// again, even if its lifecycle state has since moved on (see
		// exportcontract.go's identical guard for the full explanation).
		// Verify against the frozen signed bytes as-is and only refresh the
		// cached C2PA-state bookkeeping so this branch doesn't keep
		// re-triggering.
		if pdfState.C2PAState != currentC2PAState {
			log.Printf("pdfgeneration: VerifyContractPdf %s already signed (c2paState=%q, contract now %q); verifying frozen PDF, no post-signature mutation",
				qry.DID, pdfState.C2PAState, currentC2PAState)
			updater := func(ctx context.Context, tx *sqlx.Tx, did string, state PDFStateData) error {
				return h.CRepo.UpdatePDFState(ctx, tx, did, cwedb.ContractPDFState{
					IPFSCID:         state.IPFSCID,
					RendererVersion: state.RendererVersion,
					C2PAState:       state.C2PAState,
					PayloadHash:     state.PayloadHash,
				})
			}
			if err := updater(ctx, tx, qry.DID, PDFStateData{
				IPFSCID:         pdfState.IPFSCID,
				RendererVersion: pdfState.RendererVersion,
				C2PAState:       currentC2PAState,
				PayloadHash:     pdfState.PayloadHash,
			}); err != nil {
				return nil, fmt.Errorf("refresh frozen PDF bookkeeping for %s: %w", qry.DID, err)
			}
			if err := tx.Commit(); err != nil {
				return nil, fmt.Errorf("commit frozen PDF bookkeeping for %s: %w", qry.DID, err)
			}
		}

		pdf, err := h.Artifacts.Get(ctx, artifactstore.ContractScope(qry.DID), pdfState.IPFSCID)
		if artifactstore.IsTampered(err) {
			return tamperedVerifyResult(currentC2PAState), nil
		}
		if err != nil || len(pdf) == 0 {
			return nil, fmt.Errorf("fetch frozen signed contract PDF %s from IPFS for verify: %w", qry.DID, err)
		}
		return runVerify(ctx, pdf, h.PDFCore, h.Credentials, h.CredentialStatus, currentC2PAState)
	}

	if pdfState.IPFSCID != "" && pdfState.C2PAState != currentC2PAState {
		log.Printf("pdfgeneration: VerifyContractPdf %s state advanced %q→%q; appending before verify",
			qry.DID, pdfState.C2PAState, currentC2PAState)

		var jsonldBytes []byte
		if contract.ContractData != nil {
			jsonldBytes = []byte(*contract.ContractData)
		}

		pdf, err := h.Artifacts.Get(ctx, artifactstore.ContractScope(qry.DID), pdfState.IPFSCID)
		if artifactstore.IsTampered(err) {
			return tamperedVerifyResult(currentC2PAState), nil
		}
		if err != nil || len(pdf) == 0 {
			return nil, fmt.Errorf("fetch cached contract PDF %s from IPFS for verify append: %w", qry.DID, err)
		}

		updater := func(ctx context.Context, tx *sqlx.Tx, did string, state PDFStateData) error {
			return h.CRepo.UpdatePDFState(ctx, tx, did, cwedb.ContractPDFState{
				IPFSCID:         state.IPFSCID,
				RendererVersion: state.RendererVersion,
				C2PAState:       state.C2PAState,
				PayloadHash:     state.PayloadHash,
			})
		}

		updatedPDF, err := appendAndCache(ctx, tx, qry.DID, contract.State, jsonldBytes, pdf,
			h.Artifacts, artifactstore.ContractScope(qry.DID), h.PDFCore, h.VCIssuer, h.IssuerDID, updater)
		if err != nil {
			return nil, fmt.Errorf("append contract C2PA assertion before verify for %s: %w", qry.DID, err)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit pre-verify append tx for contract %s: %w", qry.DID, err)
		}

		return runVerify(ctx, updatedPDF, h.PDFCore, h.Credentials, h.CredentialStatus, currentC2PAState)
	}

	if latestCID == "" {
		return nil, fmt.Errorf("no cached PDF for contract %s; call export first", qry.DID)
	}

	pdf, err := h.Artifacts.Get(ctx, artifactstore.ContractScope(qry.DID), latestCID)
	if artifactstore.IsTampered(err) {
		return tamperedVerifyResult(currentC2PAState), nil
	}
	if err != nil || len(pdf) == 0 {
		return nil, fmt.Errorf("fetch contract PDF %s from IPFS for verify: %w", qry.DID, err)
	}

	return runVerify(ctx, pdf, h.PDFCore, h.Credentials, h.CredentialStatus, currentC2PAState)
}
