package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

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
		return errors.New("contract template was updated elsewhere, please reload")
	}

	if processData.State != contracttemplatestate.Registered.String() {
		return errors.New("contract template must be in registered state to publish")
	}

	if h.FCClient == nil {
		return fcclient.ErrFederatedCatalogueNotConfigured
	}

	// Exclude remote calls from the transaction to avoid a long-running transaction.
	if err := h.publishTemplateResourceToFC(ctx, cmd, processData, fullTemplate); err != nil {
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
		return errors.New("contract template must be in registered state to publish")
	}

	err = h.CTRepo.UpdateState(ctx, tx, cmd.DID, contracttemplatestate.Published.String())
	if err != nil {
		return fmt.Errorf("could not update state: %w", err)
	}

	evt := templateevents.PublishEvent{
		DID:            cmd.DID,
		DocumentNumber: processData.DocumentNumber,
		Version:        processData.Version,
		PublishedBy:    cmd.PublishedBy,
		HolderDID:      cmd.HolderDID,
		OccurredAt:     time.Now().UTC(),
		UserRoles:      cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}

func (h *Publisher) publishTemplateResourceToFC(ctx context.Context, cmd PublishCmd, processData *db.ContractTemplateProcessData, fullTemplate *db.ContractTemplate) error {
	if cmd.HolderDID == "" {
		return fmt.Errorf("holder did is empty")
	}

	name := ""
	description := ""
	if fullTemplate.Name != nil {
		name = *fullTemplate.Name
	}
	if fullTemplate.Description != nil {
		description = *fullTemplate.Description
	}

	templateDataString, err := fcasset.TemplateDataString(fullTemplate.TemplateData)
	if err != nil {
		return fmt.Errorf("serialize template data for Federated Catalogue: %w", err)
	}

	payload, err := fcasset.BuildPayload(fcasset.BuildInput{
		Issuer:    cmd.HolderDID,
		ValidFrom: fullTemplate.UpdatedAt,
		Subject: fcasset.CatalogueSubjectFromRepository(
			cmd.DID,
			processData.Version,
			processData.State,
			name,
			description,
			fullTemplate.TemplateType,
		),
		TemplateDataString: templateDataString,
	})

	if err != nil {
		return fmt.Errorf("build template asset payload failed: %w", err)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal template asset payload failed: %w", err)
	}

	resp, err := h.FCClient.PostRaw(ctx, fcclient.AssetsEndpointPath, nil, fcclient.JSONLDContentType, body)
	if err != nil {
		return fmt.Errorf("publish template resource failed: %w", err)
	}

	if resp.StatusCode == http.StatusCreated {
		return nil
	}

	// FC duplicate self-descriptions return conflict_error
	if resp.StatusCode == http.StatusConflict {
		code := h.FCClient.ExtractErrorCode(resp.Body)
		message := h.FCClient.ExtractErrorMessage(resp.Body)
		// The template SD already exists in the FC
		if code == "conflict_error" {
			return nil
		}
		if message != "" {
			return fmt.Errorf("publish template resource failed: %s", message)
		}
		return fmt.Errorf("publish template resource failed with status %d", resp.StatusCode)
	}
	if message := h.FCClient.ExtractErrorMessage(resp.Body); message != "" {
		return fmt.Errorf("publish template resource failed: %s", message)
	}
	return fmt.Errorf("publish template resource failed with status %d", resp.StatusCode)
}
