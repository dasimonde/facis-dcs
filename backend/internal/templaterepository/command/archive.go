package command

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
)

type ArchiveCmd struct {
	DID           string
	UpdatedAt     time.Time
	ArchivedBy    string
	Justification string
	HolderDID     string
	UserRoles     userrole.UserRoles
}

type Archiver struct {
	DB       *sqlx.DB
	CTRepo   db.ContractTemplateRepo
	RTRepo   db.ReviewTaskRepo
	ATRepo   db.ApprovalTaskRepo
	FCClient *fcclient.FederatedCatalogueClient
}

func (h *Archiver) Handle(ctx context.Context, cmd ArchiveCmd) error {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		err := tx.Rollback()
		if err != nil {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	processData, err := h.CTRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	// Optimistic concurrency (see command package doc / ADR-0007).
	if cmd.UpdatedAt.Unix() < processData.ContentUpdatedAt.Unix() {
		return fmt.Errorf("contract template %w, please reload", base.ErrUpdatedElsewhere)
	}

	// DCS-IR-TR-07: an archival states its justification.
	if strings.TrimSpace(cmd.Justification) == "" {
		return fmt.Errorf("%w: archiving a template requires a justification", ErrInvalidRequest)
	}

	// A registered or published template is deprecated (DCS-FR-TR-17). A
	// template that never entered the pool (DRAFT, REJECTED) or a deprecated
	// one is deleted (DCS-FR-TR-18). A template in review or approved is
	// neither.
	var newState contracttemplatestate.ContractTemplateState
	switch processData.State {
	case contracttemplatestate.Registered.String(), contracttemplatestate.Published.String():
		newState = contracttemplatestate.Deprecated
	case contracttemplatestate.Draft.String(), contracttemplatestate.Rejected.String(), contracttemplatestate.Deprecated.String():
		newState = contracttemplatestate.Deleted
	default:
		return fmt.Errorf("%w: a %s template is neither deprecated nor deleted", ErrInvalidTemplateState, processData.State)
	}

	// DCS-IR-TR-07: a template that open contracts were made from is not deleted.
	if newState == contracttemplatestate.Deleted {
		open, err := h.CTRepo.CountOpenContracts(ctx, tx, cmd.DID)
		if err != nil {
			return err
		}
		if open > 0 {
			return fmt.Errorf("%w: %d contracts made from the template are not terminated", ErrTemplateInUse, open)
		}
	}

	// DCS-NFR-BR-09: a deprecated template is withdrawn from the catalogue first.
	if processData.State == contracttemplatestate.Published.String() && processData.CatalogueAssetHash != nil {
		if h.FCClient == nil {
			return fcclient.ErrFederatedCatalogueNotConfigured
		}
		if err := h.FCClient.RevokeAsset(ctx, *processData.CatalogueAssetHash); err != nil {
			return fmt.Errorf("could not withdraw the template from the Federated Catalogue: %w", err)
		}
	}

	err = h.CTRepo.UpdateState(ctx, tx, cmd.DID, processData.State, newState.String())
	if err != nil {
		return fmt.Errorf("could not update state: %w", err)
	}

	evt := templateevents.ArchiveEvent{
		DID:           cmd.DID,
		Version:       processData.Version,
		ArchivedBy:    cmd.ArchivedBy,
		PreviousState: processData.State,
		NewState:      newState.String(),
		Justification: cmd.Justification,
		OccurredAt:    time.Now().UTC(),
		HolderDID:     cmd.HolderDID,
		UserRoles:     cmd.UserRoles,
	}
	var archived event.Event = evt
	if newState == contracttemplatestate.Deleted {
		archived = templateevents.DeleteEvent(evt)
	}
	err = event.Create(ctx, tx, archived, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	// DCS-FR-TR-17: a deprecated version keeps its review and approval
	// records. A deleted template keeps no open task.
	if newState == contracttemplatestate.Deleted {
		if err := h.RTRepo.Delete(ctx, tx, cmd.DID); err != nil {
			return fmt.Errorf("could not delete review tasks: %w", err)
		}
		if err := h.ATRepo.Delete(ctx, tx, cmd.DID); err != nil {
			return fmt.Errorf("could not delete approval tasks: %w", err)
		}
	}

	return tx.Commit()
}

// ErrInvalidRequest refuses a request with missing or invalid input.
var ErrInvalidRequest = errors.New("invalid request")

// ErrTemplateInUse refuses the deletion of a template that open contracts were made from.
var ErrTemplateInUse = errors.New("the template is in use")
