// Package command implements the write-side CQRS use cases for the template
// repository: one file per Goa endpoint. Templates use copy-on-version
// (see copy.go, register.go): every version is its own row/DID linked by a
// base_template lineage key, unlike contractworkflowengine's contracts,
// which mutate a single row in place and keep history in a separate table
// (see ADR-0006 for the rationale).
package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"

	"digital-contracting-service/internal/base/datatype/userrole"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
)

type CreateCmd struct {
	DID          string
	CreatedBy    string
	TemplateType contracttemplatetype.ContractTemplateType
	Name         *string
	Description  *string
	TemplateData *datatype.JSON
	HolderDID    string
	UserRoles    userrole.UserRoles
}

type Creator struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
}

func (h *Creator) Handle(ctx context.Context, cmd CreateCmd) error {
	normalizedTemplateData, err := validation.NormalizeTemplateDataForPersistence(cmd.TemplateData, cmd.DID)
	if err != nil {
		return fmt.Errorf("template data validation failed: %w", err)
	}
	cmd.TemplateData = normalizedTemplateData

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	data := db.ContractTemplate{
		DID:          cmd.DID,
		CreatedBy:    cmd.CreatedBy,
		State:        contracttemplatestate.Draft.String(),
		TemplateType: cmd.TemplateType.String(),
		Name:         cmd.Name,
		Description:  cmd.Description,
		TemplateData: cmd.TemplateData,
	}
	createdAt, err := h.CTRepo.Create(ctx, tx, data)
	if err != nil {
		return fmt.Errorf("could not create contract template: %w", err)
	}

	evt := templateevents.CreateEvent{
		DID:          cmd.DID,
		CreatedBy:    cmd.CreatedBy,
		Name:         cmd.Name,
		Description:  cmd.Description,
		TemplateData: cmd.TemplateData,
		OccurredAt:   *createdAt,
		HolderDID:    cmd.HolderDID,
		UserRoles:    cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}
