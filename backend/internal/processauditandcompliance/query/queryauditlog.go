package qry

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	event2 "digital-contracting-service/internal/processauditandcompliance/event"
)

type GetAuditLogQry struct {
	Scope         componenttype.ComponentType
	RelatedScopes []componenttype.ComponentType
	AuditedBy     string
	HolderDID     string
	UserRoles     userrole.UserRoles
	DID           string
	Justification string
}

type Auditor struct {
	DB           *sqlx.DB
	ATrailReader base.AuditTrailReader
}

func (h *Auditor) Handle(ctx context.Context, query GetAuditLogQry) ([][]datatype.AuditLogEntry, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	var result [][]datatype.AuditLogEntry
	if query.DID != "" {
		entries, readErr := h.ATrailReader.ReadAuditLogEntriesByComponentAndDID(ctx, tx, query.Scope, query.DID)
		if readErr != nil {
			return nil, fmt.Errorf("could not read resource audit log entries: %w", readErr)
		}
		result = [][]datatype.AuditLogEntry{entries}
	} else {
		result, err = h.ATrailReader.ReadAuditLogEntriesByComponent(ctx, tx, query.Scope)
		if err != nil {
			return nil, fmt.Errorf("could not read audit log entries: %w", err)
		}
	}
	for _, relatedScope := range query.RelatedScopes {
		if query.DID != "" {
			related, readErr := h.ATrailReader.ReadAuditLogEntriesByComponentAndDID(ctx, tx, relatedScope, query.DID)
			if readErr != nil {
				return nil, fmt.Errorf("could not read related %s resource audit log entries: %w", relatedScope, readErr)
			}
			result = append(result, related)
		} else {
			related, readErr := h.ATrailReader.ReadAuditLogEntriesByComponent(ctx, tx, relatedScope)
			if readErr != nil {
				return nil, fmt.Errorf("could not read related %s audit log entries: %w", relatedScope, readErr)
			}
			result = append(result, related...)
		}
	}

	evt := event2.AuditEvent{
		Scope:         query.Scope,
		ComponentType: componenttype.ProcessAuditAndCompliance,
		AuditedBy:     query.AuditedBy,
		OccurredAt:    time.Now().UTC(),
		HolderDID:     query.HolderDID,
		UserRoles:     query.UserRoles,
		DID:           query.DID,
		Justification: query.Justification,
	}
	err = event.Create(ctx, tx, evt, componenttype.ProcessAuditAndCompliance)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return result, nil
}
