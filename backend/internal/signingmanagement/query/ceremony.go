package query

import (
	"context"
	"fmt"
	"time"

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
	// A pending ceremony after its deadline does not accept a presentation,
	// and a published signing request after its expiry does not accept a
	// signed document. Its status is expired, which tells the signer to retry
	// (FR-SM-13).
	now := time.Now().UTC()
	if ceremony != nil && ceremony.Status == db.CeremonyPending && !now.Before(ceremony.ExpiresAt) {
		ceremony.Status = db.CeremonyExpired
	}
	if ceremony != nil && ceremony.Status == db.CeremonyVerified && ceremony.ConsumedAt == nil &&
		ceremony.RequestExpiresAt != nil && now.After(*ceremony.RequestExpiresAt) {
		ceremony.Status = db.CeremonyExpired
	}
	return ceremony, nil
}

func rollbackQuery(tx *sqlx.Tx) {
	_ = tx.Rollback()
}
