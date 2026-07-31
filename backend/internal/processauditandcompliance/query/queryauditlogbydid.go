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
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
	templateevents "digital-contracting-service/internal/templaterepository/event"
)

type GetAuditLogByDIDQry struct {
	Scope     componenttype.ComponentType
	DID       string
	AuditedBy string
	HolderDID string
	UserRoles userrole.UserRoles
}

type AuditLogByDIDAuditor struct {
	DB           *sqlx.DB
	ATrailReader base.AuditTrailReader
}

func buildAuditEvent(query GetAuditLogByDIDQry) (event.Event, error) {
	switch query.Scope {
	case componenttype.ContractWorkflowEngine:
		return contractevents.AuditEvent{
			DID:           query.DID,
			HolderDID:     query.HolderDID,
			AuditedBy:     query.AuditedBy,
			OccurredAt:    time.Now().UTC(),
			ComponentType: query.Scope,
			UserRoles:     query.UserRoles,
		}, nil
	case componenttype.ContractTemplateRepo:
		return templateevents.AuditEvt{
			DID:           query.DID,
			ComponentType: query.Scope,
			AuditedBy:     query.AuditedBy,
			OccurredAt:    time.Now().UTC(),
			HolderDID:     query.HolderDID,
			UserRoles:     query.UserRoles,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported audit scope: %s", query.Scope)
	}
}

func (h *AuditLogByDIDAuditor) Handle(ctx context.Context, query GetAuditLogByDIDQry) ([]datatype.AuditLogEntry, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	result, err := h.ATrailReader.ReadAuditLogEntriesByComponentAndDID(ctx, tx, query.Scope, query.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read audit log entries: %w", err)
	}
	evt, err := buildAuditEvent(query)
	if err != nil {
		return nil, fmt.Errorf("could not build audit event: %w", err)
	}

	err = event.Create(ctx, tx, evt, query.Scope)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return result, nil
}
