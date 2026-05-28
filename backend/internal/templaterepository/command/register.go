package command

import (
	"context"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	templatequery "digital-contracting-service/internal/templatecatalogueintegration/query/template"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

type RegisterCmd struct {
	DID          string
	NewDID       string
	Version      int
	RegisteredBy string
}

type Registrar struct {
	DB       *sqlx.DB
	CTRepo   db.ContractTemplateRepo
	FCClient *fcclient.FederatedCatalogueClient
}

func (h *Registrar) Handle(ctx context.Context, cmd RegisterCmd) error {
	if cmd.DID == "" {
		return errors.New("did is empty")
	}
	if cmd.NewDID == "" {
		return errors.New("new did is empty")
	}
	if cmd.Version < 1 {
		return errors.New("version must be greater than 0")
	}
	if h.FCClient == nil {
		return errors.New("federated catalogue is not configured")
	}

	queryHandler := templatequery.GetByIDHandler{
		Ctx:      ctx,
		FCClient: h.FCClient,
	}
	fcTemplate, err := queryHandler.Handle(templatequery.GetByIDQry{
		DID:     cmd.DID,
		Version: cmd.Version,
	})
	if err != nil {
		return fmt.Errorf("could not retrieve template from Federated Catalogue: %w", err)
	}
	if fcTemplate == nil {
		return errors.New("template not found in Federated Catalogue")
	}

	templateData, err := templateDataFromAny(fcTemplate.TemplateData)
	if err != nil {
		return err
	}

	templateTypeValue := ""
	if fcTemplate.TemplateType != nil {
		templateTypeValue = *fcTemplate.TemplateType
	}
	templateType, err := contracttemplatetype.NewContractTemplateType(templateTypeValue)
	if err != nil {
		return fmt.Errorf("invalid template type from Federated Catalogue: %w", err)
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer tx.Rollback()

	existing, err := h.CTRepo.ReadDataByID(ctx, tx, cmd.NewDID)
	if err == nil {
		return fmt.Errorf("generated did already exists locally: %s", existing.DID)
	}
	if !errors.Is(err, db.ErrContractTemplateNotFound) {
		return fmt.Errorf("could not read template: %w", err)
	}

	_, err = h.CTRepo.Create(ctx, tx, db.ContractTemplate{
		DID:            cmd.NewDID,
		DocumentNumber: fcTemplate.DocumentNumber,
		State:          contracttemplatestate.Draft.String(),
		TemplateType:   templateType.String(),
		Name:           fcTemplate.Name,
		Description:    fcTemplate.Description,
		CreatedBy:      cmd.RegisteredBy,
		TemplateData:   templateData,
	})
	if err != nil {
		return fmt.Errorf("could not create registered contract template: %w", err)
	}

	evt := templateevents.RegisterEvent{
		DID:           cmd.NewDID,
		RegisteredBy:  cmd.RegisteredBy,
		UpdatedAt:     time.Now().UTC(),
		Name:          fcTemplate.Name,
		Description:   fcTemplate.Description,
		TemplateData:  templateData,
		SourceDID:     cmd.DID,
		SourceVersion: cmd.Version,
		OccurredAt:    time.Now().UTC(),
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}

func templateDataFromAny(raw any) (*datatype.JSON, error) {
	if raw == nil {
		return nil, errors.New("template data is missing from Federated Catalogue")
	}

	templateDataMap, ok := raw.(map[string]interface{})
	if !ok {
		return nil, errors.New("invalid template data format from Federated Catalogue")
	}

	templateData, err := datatype.NewJSON(templateDataMap)
	if err != nil {
		return nil, fmt.Errorf("marshal template data failed: %w", err)
	}

	return &templateData, nil
}
