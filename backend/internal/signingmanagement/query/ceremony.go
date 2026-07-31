package query

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/signingmanagement/db"
)

// CeremonyStatusQry carries the inputs for polling a ceremony's status.
type CeremonyStatusQry struct {
	CeremonyID string
}

// CeremonyStatusHandler reads a signing ceremony's lifecycle status.
type CeremonyStatusHandler struct {
	DB           *sqlx.DB
	CeremonyRepo db.CeremonyRepo
}

func (h *CeremonyStatusHandler) Handle(ctx context.Context, qry CeremonyStatusQry) (*db.SignatureCeremony, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer rollbackQuery(tx)

	ceremony, err := h.CeremonyRepo.GetCeremonyByID(ctx, tx, qry.CeremonyID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit ceremony status: %w", err)
	}
	return ceremony, nil
}

func rollbackQuery(tx *sqlx.Tx) {
	_ = tx.Rollback()
}
