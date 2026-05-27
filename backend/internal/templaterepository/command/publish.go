package command

import (
	"context"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	templatequery "digital-contracting-service/internal/templatecatalogueintegration/query/template"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
	"digital-contracting-service/internal/templaterepository/selfdescription"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jmoiron/sqlx"
)

type PublishCmd struct {
	DID           string
	UpdatedAt     time.Time
	PublishedBy   string
	ParticipantID string
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
		defer tx.Rollback()

		processData, err = h.CTRepo.ReadProcessData(ctx, tx, cmd.DID)
		if err != nil {
			return fmt.Errorf("could not read process data: %w", err)
		}

		fullTemplate, err = h.CTRepo.ReadDataByID(ctx, tx, cmd.DID)
		if err != nil {
			return fmt.Errorf("could not read template data: %w", err)
		}
	}

	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		return errors.New("contract template was updated elsewhere, please reload")
	}

	if processData.State != contracttemplatestate.Approved.String() {
		return errors.New("contract template must be in approved state to publish")
	}

	if h.FCClient == nil {
		return errors.New("federated catalogue is not configured")
	}

	if err := h.ensureTemplateNotInFC(ctx, cmd.DID); err != nil {
		return err
	}

	// Exclude remote calls from the transaction to avoid a long-running transaction.
	if err := h.publishTemplateResourceToFC(ctx, cmd, processData, fullTemplate); err != nil {
		return err
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer tx.Rollback()

	processData, err = h.CTRepo.ReadProcessData(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		return errors.New("contract template was updated elsewhere, please reload")
	}

	evt := templateevents.PublishEvent{
		DID:            cmd.DID,
		DocumentNumber: processData.DocumentNumber,
		Version:        processData.Version,
		PublishedBy:    cmd.PublishedBy,
		OccurredAt:     time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}

func (h *Publisher) ensureTemplateNotInFC(ctx context.Context, did string) error {
	queryHandler := templatequery.GetByIDHandler{
		Ctx:      ctx,
		FCClient: h.FCClient,
	}
	existing, err := queryHandler.Handle(templatequery.GetByIDQry{DID: did})
	if err != nil {
		return fmt.Errorf("could not check template in Federated Catalogue: %w", err)
	}
	if existing != nil {
		return errors.New("template already exists in Federated Catalogue")
	}
	return nil
}

func (h *Publisher) publishTemplateResourceToFC(ctx context.Context, cmd PublishCmd, processData *db.ContractTemplateProcessData, fullTemplate *db.ContractTemplate) error {
	if cmd.ParticipantID == "" {
		return fmt.Errorf("participant id is empty")
	}
	documentNumber := ""
	if processData.DocumentNumber != nil && *processData.DocumentNumber != "" {
		documentNumber = *processData.DocumentNumber
	}

	templateType := fullTemplate.TemplateType
	name := ""
	description := ""
	if fullTemplate.Name != nil {
		name = *fullTemplate.Name
	}
	if fullTemplate.Description != nil {
		description = *fullTemplate.Description
	}

	sd := selfdescription.BuildTemplateResourceSelfDescription(selfdescription.TemplateResourceInput{
		ParticipantID:  cmd.ParticipantID,
		DID:            cmd.DID,
		DocumentNumber: documentNumber,
		Version:        processData.Version,
		TemplateType:   templateType,
		Name:           name,
		Description:    description,
		CreatedAt:      fullTemplate.CreatedAt,
		UpdatedAt:      fullTemplate.UpdatedAt,
		TemplateData:   fullTemplate.TemplateData,
	})

	body, err := json.Marshal(sd)
	if err != nil {
		return fmt.Errorf("marshal template resource self-description failed: %w", err)
	}

	resp, err := h.FCClient.Post(ctx, fcclient.SelfDescriptionsEndpointPath, nil, body)
	if err != nil {
		return fmt.Errorf("publish template resource failed: %w", err)
	}

	if resp.StatusCode != http.StatusCreated {
		if message := h.FCClient.ExtractErrorMessage(resp.Body); message != "" {
			return fmt.Errorf("publish template resource failed: %s", message)
		}
		return fmt.Errorf("publish template resource failed with status %d", resp.StatusCode)
	}
	return nil
}
