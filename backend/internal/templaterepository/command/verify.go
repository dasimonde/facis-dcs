package command

import (
	"context"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templaterepository/datatype/reviewtaskstate"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
	"digital-contracting-service/internal/templaterepository/selfdescription"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/jmoiron/sqlx"
)

type VerifyCmd struct {
	DID           string
	VerifiedBy    string
	ParticipantID string
}

type Verifier struct {
	DB       *sqlx.DB
	CTRepo   db.ContractTemplateRepo
	RTRepo   db.ReviewTaskRepo
	FCClient *fcclient.FederatedCatalogueClient
}

func (h *Verifier) Handle(ctx context.Context, cmd VerifyCmd) error {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer tx.Rollback()

	processData, err := h.CTRepo.ReadProcessData(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read process data: %w", err)
	}

	fullTemplate, err := h.CTRepo.ReadDataByID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read template data: %w", err)
	}

	if h.FCClient != nil {
		if err := h.verifyTemplateResourceSelfDescription(ctx, cmd, processData, fullTemplate); err != nil {
			return err
		}
	}

	hasTask, err := h.RTRepo.TaskExistsInState(ctx, tx, cmd.DID, cmd.VerifiedBy, reviewtaskstate.Open.String())
	if err != nil {
		return err
	}

	if hasTask {
		err := h.RTRepo.UpdateState(ctx, tx, cmd.DID, cmd.VerifiedBy, reviewtaskstate.Verified.String())
		if err != nil {
			return err
		}
	}

	evt := templateevents.VerifyEvent{
		DID:            cmd.DID,
		DocumentNumber: processData.DocumentNumber,
		Version:        processData.Version,
		VerifiedBy:     cmd.VerifiedBy,
		OccurredAt:     time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}

func (h *Verifier) verifyTemplateResourceSelfDescription(ctx context.Context, cmd VerifyCmd, processData *db.ContractTemplateProcessData, fullTemplate *db.ContractTemplate) error {
	if h.FCClient == nil {
		return fcclient.ErrFederatedCatalogueNotConfigured
	}
	if cmd.ParticipantID == "" {
		return fmt.Errorf("participant id is empty")
	}
	if processData == nil {
		return fmt.Errorf("process data is nil")
	}
	if fullTemplate == nil {
		return fmt.Errorf("full template is nil")
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

	query := url.Values{}
	query.Set("verifySemantics", "true")
	query.Set("verifySchema", "true")
	query.Set("verifySignatures", "true")
	query.Set("verifyVPSignature", "false")
	query.Set("verifyVCSignature", "false")

	resp, err := h.FCClient.Post(ctx, fcclient.VerificationEndpointPath, query, body)
	if err != nil {
		return fmt.Errorf("verify template resource self-description failed: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if message := h.FCClient.ExtractErrorMessage(resp.Body); message != "" {
			return fmt.Errorf("verify template resource self-description failed: %s", message)
		}
		return fmt.Errorf("verify template resource self-description failed with status %d", resp.StatusCode)
	}

	return nil
}
