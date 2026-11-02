package command

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/fcasset"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"

	"github.com/jmoiron/sqlx"
)

type PublishCmd struct {
	DID         string
	UpdatedAt   time.Time
	PublishedBy string
	HolderDID   string
	// InstanceDID is this DCS deployment's own did:web (signing.issuerDID) —
	// the FC-facing publisher identity. A federated catalogue's Participant
	// concept is an organization/deployment, not the individual employee who
	// clicked publish, so this — not HolderDID — is what's asserted as the
	// asset's issuer to FC.
	InstanceDID string
	UserRoles   userrole.UserRoles
}

type Publisher struct {
	DB       *sqlx.DB
	CTRepo   db.ContractTemplateRepo
	FCClient *fcclient.FederatedCatalogueClient
}

func (h *Publisher) Handle(ctx context.Context, cmd PublishCmd) error {
	var processData *db.ContractTemplateProcessData
	var fullTemplate *db.ContractTemplate

	{
		tx, err := h.DB.BeginTxx(ctx, nil)
		if err != nil {
			return fmt.Errorf("could not start transaction: %w", err)
		}
		defer func(tx *sqlx.Tx) {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				log.Printf("could not rollback transaction: %v", err)
			}
		}(tx)

		processData, err = h.CTRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
		if err != nil {
			return fmt.Errorf("could not read process data: %w", err)
		}

		fullTemplate, err = h.CTRepo.ReadDataByID(ctx, tx, cmd.DID)
		if err != nil {
			return fmt.Errorf("could not read template data: %w", err)
		}
	}

	if cmd.UpdatedAt.Unix() < processData.ContentUpdatedAt.Unix() {
		return fmt.Errorf("contract template %w, please reload", base.ErrUpdatedElsewhere)
	}

	if processData.State != contracttemplatestate.Registered.String() {
		return fmt.Errorf("%w: contract template must be in registered state to publish", ErrInvalidTemplateState)
	}
	if fullTemplate.ComplianceStatus != Compliant {
		return fmt.Errorf("%w: compliance status %s", ErrTemplateNotCompliant, fullTemplate.ComplianceStatus)
	}

	if h.FCClient == nil {
		return fcclient.ErrFederatedCatalogueNotConfigured
	}

	// Exclude remote calls from the transaction to avoid a long-running transaction.
	assetHash, err := h.publishTemplateResourceToFC(ctx, cmd, processData, fullTemplate)
	if err != nil {
		return err
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	processData, err = h.CTRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	// State may already be published when a previous FC publish succeeded but local
	// state transition/event transaction failed.
	if processData.State == contracttemplatestate.Published.String() {
		return nil
	}
	if processData.State != contracttemplatestate.Registered.String() {
		return fmt.Errorf("%w: contract template must be in registered state to publish", ErrInvalidTemplateState)
	}

	err = h.CTRepo.UpdateState(ctx, tx, cmd.DID, processData.State, contracttemplatestate.Published.String())
	if err != nil {
		return fmt.Errorf("could not update state: %w", err)
	}
	if err := h.CTRepo.UpdateCatalogueAssetHash(ctx, tx, cmd.DID, assetHash); err != nil {
		return fmt.Errorf("could not record catalogue asset: %w", err)
	}

	evt := templateevents.PublishEvent{
		DID:         cmd.DID,
		Version:     processData.Version,
		PublishedBy: cmd.PublishedBy,
		HolderDID:   cmd.HolderDID,
		OccurredAt:  time.Now().UTC(),
		UserRoles:   cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}

func (h *Publisher) publishTemplateResourceToFC(ctx context.Context, cmd PublishCmd, processData *db.ContractTemplateProcessData, fullTemplate *db.ContractTemplate) (string, error) {
	if cmd.InstanceDID == "" {
		return "", fmt.Errorf("instance did is empty")
	}

	templateDataString, err := fcasset.TemplateDataString(fullTemplate.TemplateData)
	if err != nil {
		return "", fmt.Errorf("serialize template data for Federated Catalogue: %w", err)
	}

	payload, err := fcasset.BuildPayload(fcasset.BuildInput{
		Issuer:             cmd.InstanceDID,
		SubjectIRI:         base.ResourceIRI("template", cmd.DID),
		ValidFrom:          fullTemplate.UpdatedAt,
		Subject:            CatalogueSubject(processData, fullTemplate),
		TemplateDataString: templateDataString,
	})

	if err != nil {
		return "", fmt.Errorf("build template asset payload failed: %w", err)
	}

	assetHash, err := h.FCClient.PublishAsset(ctx, payload)
	if err != nil {
		return "", fmt.Errorf("publish template resource failed: %w", err)
	}
	return assetHash, nil
}

// CatalogueSubject is the catalogue metadata of a template, with the
// searchable metadata an importing instance takes over (DCS-FR-TR-01).
func CatalogueSubject(processData *db.ContractTemplateProcessData, template *db.ContractTemplate) fcasset.CatalogueSubject {
	subject := fcasset.CatalogueSubjectFromRepository(
		template.DID,
		processData.Version,
		processData.State,
		base.DerefString(template.Name),
		base.DerefString(template.Description),
		template.TemplateType,
		template.PartyType,
	)
	subject.Jurisdiction = base.DerefString(template.Jurisdiction)
	subject.Industry = base.DerefString(template.Industry)
	subject.TemplateLevel = template.TemplateLevel
	return subject
}
