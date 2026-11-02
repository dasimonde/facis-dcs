package query

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base/datatype/userrole"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	signingmanagementevents "digital-contracting-service/internal/signingmanagement/event"
)

type GetAuditLogQry struct {
	DID       string
	AuditedBy string
	HolderDID string
	UserRoles userrole.UserRoles
	// Justification is the reason the auditor gave for this access (DCS-FR-PACM-04).
	Justification string
}

type Auditor struct {
	DB           *sqlx.DB
	ATrailReader base.AuditTrailReader
}

func (h *Auditor) Handle(ctx context.Context, query GetAuditLogQry) ([]datatype.AuditLogEntry, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	result, err := h.ATrailReader.ReadAuditLogEntriesByComponentAndDID(ctx, tx, componenttype.SignatureManagement, query.DID)
	if err != nil {
		return nil, err
	}

	evt := signingmanagementevents.AuditEvt{
		DID:           query.DID,
		ComponentType: componenttype.SignatureManagement,
		AuditedBy:     query.AuditedBy,
		OccurredAt:    time.Now().UTC(),
		HolderDID:     query.HolderDID,
		UserRoles:     query.UserRoles,
		Justification: query.Justification,
	}
	err = event.Create(ctx, tx, evt, componenttype.SignatureManagement)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return result, nil
}
