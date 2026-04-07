package command

import (
	"context"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
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

type RegisterCmd struct {
	DID           string
	UpdatedAt     time.Time
	RegisteredBy  string
	ParticipantID string
	Token         string
}

type Registrar struct {
	Ctx      context.Context
	DB       *sqlx.DB
	CTRepo   db.ContractTemplateRepo
	RTRepo   db.ReviewTaskRepo
	ATRepo   db.ApprovalTaskRepo
	FCClient *fcclient.FederatedCatalogueClient
}

func (h *Registrar) Handle(cmd RegisterCmd) error {

	ctx, cancel := context.WithTimeout(h.Ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer tx.Rollback()

	processData, err := h.CTRepo.ReadProcessData(tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	if cmd.UpdatedAt.Unix() < processData.UpdatedAt.Unix() {
		return errors.New("contract template was updated elsewhere, please reload")
	}

	if processData.State != contracttemplatestate.Approved.String() {
		return errors.New("invalid contract template state")
	}

	fullTemplate, err := h.CTRepo.ReadDataByID(tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read template data: %w", err)
	}

	if h.FCClient != nil {
		if err := h.publishTemplateResourceToFC(cmd, processData, fullTemplate); err != nil {
			return fmt.Errorf("could not publish template to Federated Catalogue: %w", err)
		}
	}

	err = h.CTRepo.UpdateState(tx, cmd.DID, contracttemplatestate.Registered.String())
	if err != nil {
		return fmt.Errorf("could not update state: %w", err)
	}

	evt := templateevents.RegisterEvent{
		DID:            cmd.DID,
		DocumentNumber: processData.DocumentNumber,
		Version:        processData.Version,
		RegisteredBy:   cmd.RegisteredBy,
		OccurredAt:     time.Now(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	err = h.RTRepo.Delete(tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not delete review tasks: %w", err)
	}

	err = h.ATRepo.Delete(tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not delete approval tasks: %w", err)
	}

	return tx.Commit()
}

func (h *Registrar) publishTemplateResourceToFC(cmd RegisterCmd, processData *db.ContractTemplateProcessData, fullTemplate *db.ContractTemplate) error {
	if h.FCClient == nil {
		return fmt.Errorf("federated catalogue client is nil")
	}
	if cmd.Token == "" {
		return fmt.Errorf("federated catalogue token is empty")
	}
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
	version := 0
	if fullTemplate.Name != nil {
		name = *fullTemplate.Name
	}
	if fullTemplate.Description != nil {
		description = *fullTemplate.Description
	}
	if processData.Version != nil && *processData.Version >= 0 {
		version = *processData.Version
	}

	sd := selfdescription.BuildTemplateResourceSelfDescription(selfdescription.TemplateResourceInput{
		ParticipantID:  cmd.ParticipantID,
		DID:            cmd.DID,
		DocumentNumber: documentNumber,
		Version:        version,
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

	resp, err := h.FCClient.Post(h.Ctx, fcclient.SelfDescriptionsEndpointPath, cmd.Token, nil, body)
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
