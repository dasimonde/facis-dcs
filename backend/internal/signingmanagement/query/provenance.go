package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/pdfgeneration/pdfcore"
	"digital-contracting-service/internal/signingmanagement/db"

	"github.com/jmoiron/sqlx"
)

// ProvenanceChainHandler reads the C2PA provenance chain embedded in a
// contract's PDF: one entry per manifest in the JUMBF store, oldest first.
type ProvenanceChainHandler struct {
	DB      *sqlx.DB
	CRepo   db.ContractRepo
	PDFCore *pdfcore.Client
}

// Handle fetches the contract's current PDF and asks pdf-core for its parsed
// C2PA manifest chain. A contract with no PDF yet (never signed/exported)
// yields an empty chain, not an error.
func (h *ProvenanceChainHandler) Handle(ctx context.Context, did string) ([]pdfcore.ChainEntry, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	pdfBytes, err := h.CRepo.FetchContractPDFBytes(ctx, tx, did)
	if err != nil || len(pdfBytes) == 0 {
		return []pdfcore.ChainEntry{}, nil
	}

	chain, err := h.PDFCore.ExtractManifestChain(ctx, pdfBytes)
	if err != nil {
		return nil, fmt.Errorf("C2PA manifest chain for %s: %w", did, err)
	}
	return chain, nil
}
