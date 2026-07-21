package command

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/pdfgeneration/provenance"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	templatequery "digital-contracting-service/internal/templatecatalogueintegration/query/template"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatetype"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"

	"github.com/jmoiron/sqlx"
)

type RegisterCmd struct {
	DID          string
	Version      int
	RegisteredBy string
	HolderDID    string
	UserRoles    userrole.UserRoles
}

type Registrar struct {
	DB       *sqlx.DB
	CTRepo   db.ContractTemplateRepo
	RTRepo   db.ReviewTaskRepo
	ATRepo   db.ApprovalTaskRepo
	FCClient *fcclient.FederatedCatalogueClient
	// VCSigner + IssuerDID issue the per-version template provenance VC
	// (DCS-FR-TR-09) at registration — registration is the moment a version
	// becomes the published one, so its provenance claims are sealed here.
	VCSigner  provenance.VCSigner
	IssuerDID string
}

func (h *Registrar) Handle(ctx context.Context, cmd RegisterCmd) (*string, error) {

	if cmd.DID == "" {
		return nil, errors.New("did is empty")
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	existing, err := h.CTRepo.ReadDataByID(ctx, tx, cmd.DID)
	if err != nil && !errors.Is(err, db.ErrContractTemplateNotFound) {
		return nil, fmt.Errorf("could not check if contract template already exists: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	if existing != nil {

		tx, err := h.DB.BeginTxx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("could not start transaction: %w", err)
		}
		defer func(tx *sqlx.Tx) {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				log.Printf("could not rollback transaction: %v", err)
			}
		}(tx)

		err = h.CTRepo.UpdateState(ctx, tx, cmd.DID, contracttemplatestate.Registered.String())
		if err != nil {
			return nil, fmt.Errorf("could not update registered state: %w", err)
		}

		// DCS-FR-TR-09: seal this version's provenance as a signed W3C VC,
		// linked to the previous version's credential. Issuance failure fails
		// the registration — a published version without verifiable
		// provenance would be exactly the gap the requirement closes.
		if err := h.issueProvenanceCredential(ctx, tx, cmd, existing); err != nil {
			return nil, fmt.Errorf("could not issue template provenance credential: %w", err)
		}

		newState := contracttemplatestate.Registered.String()
		evt := templateevents.RegisterEvent{
			DID:           cmd.DID,
			RegisteredBy:  cmd.RegisteredBy,
			UpdatedAt:     time.Now().UTC(),
			Name:          existing.Name,
			Description:   existing.Description,
			TemplateData:  existing.TemplateData,
			SourceDID:     existing.DID,
			SourceVersion: existing.Version,
			OccurredAt:    time.Now().UTC(),
			HolderDID:     cmd.HolderDID,
			UserRoles:     cmd.UserRoles,
			PreviousState: &existing.State,
			NewState:      &newState,
		}
		err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
		if err != nil {
			return nil, fmt.Errorf("could not create event: %w", err)
		}

		err = tx.Commit()
		if err != nil {
			return nil, fmt.Errorf("could not commit transaction: %w", err)
		}

		return &cmd.DID, nil

	} else {

		if cmd.Version < 1 {
			return nil, errors.New("version must be greater than 0")
		}

		if h.FCClient == nil {
			return nil, fcclient.ErrFederatedCatalogueNotConfigured
		}

		newDID, err := base.GenerateID()
		if err != nil {
			return nil, fmt.Errorf("could not get new DID for contract template: %w", err)
		}

		queryHandler := templatequery.GetByIDHandler{
			FCClient: h.FCClient,
		}
		fcTemplate, err := queryHandler.Handle(ctx, templatequery.GetByIDQry{
			DID:     cmd.DID,
			Version: cmd.Version,
		})
		if err != nil {
			return nil, fmt.Errorf("could not retrieve template from Federated Catalogue: %w", err)
		}
		if fcTemplate == nil {
			return nil, fcclient.ErrTemplateNotFoundInFederatedCatalogue
		}

		var templateData *datatype.JSON
		if fcTemplate.TemplateData == nil {
			return nil, errors.New("template data is missing from Federated Catalogue")
		}
		templateData, err = templateDataFromAny(fcTemplate.TemplateData)
		if err != nil {
			return nil, err
		}

		if fcTemplate.TemplateType == nil || strings.TrimSpace(*fcTemplate.TemplateType) == "" {
			return nil, errors.New("template type is missing from Federated Catalogue")
		}
		templateType, err := contracttemplatetype.NewContractTemplateType(*fcTemplate.TemplateType)
		if err != nil {
			return nil, fmt.Errorf("invalid template type from Federated Catalogue: %w", err)
		}

		tx, err := h.DB.BeginTxx(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("could not start transaction: %w", err)
		}
		defer func(tx *sqlx.Tx) {
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				log.Printf("could not rollback transaction: %v", err)
			}
		}(tx)

		_, err = h.CTRepo.Create(ctx, tx, db.ContractTemplate{
			DID:            *newDID,
			DocumentNumber: fcTemplate.DocumentNumber,
			State:          contracttemplatestate.Draft.String(),
			TemplateType:   templateType.String(),
			Name:           fcTemplate.Name,
			Description:    fcTemplate.Description,
			CreatedBy:      cmd.RegisteredBy,
			TemplateData:   templateData,
		})
		if err != nil {
			return nil, fmt.Errorf("could not create registered contract template: %w", err)
		}

		evt := templateevents.RegisterEvent{
			DID:           *newDID,
			RegisteredBy:  cmd.RegisteredBy,
			UpdatedAt:     time.Now().UTC(),
			Name:          fcTemplate.Name,
			Description:   fcTemplate.Description,
			TemplateData:  templateData,
			SourceDID:     cmd.DID,
			SourceVersion: cmd.Version,
			OccurredAt:    time.Now().UTC(),
			HolderDID:     cmd.HolderDID,
			UserRoles:     cmd.UserRoles,
		}
		err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
		if err != nil {
			return nil, fmt.Errorf("could not create event: %w", err)
		}

		err = tx.Commit()
		if err != nil {
			return nil, fmt.Errorf("could not commit transaction: %w", err)
		}

		return newDID, nil
	}
}

// issueProvenanceCredential gathers the version's actor trail (creator from
// the template row, reviewers/approvers from their decided tasks, registrar
// from the command), issues the signed provenance VC, and stores it linked
// to the previous version's credential (DCS-FR-TR-09).
func (h *Registrar) issueProvenanceCredential(ctx context.Context, tx *sqlx.Tx, cmd RegisterCmd, existing *db.ContractTemplate) error {
	if h.VCSigner == nil || h.IssuerDID == "" {
		return errors.New("template provenance VC issuance is not configured (VCSigner/IssuerDID)")
	}

	// Registration is idempotent per state, so a repeated register of the
	// same version must not mint a second credential: one credential seals
	// one version. Issued evidence is never overwritten.
	issued, err := h.CTRepo.ReadProvenanceCredentials(ctx, tx, cmd.DID)
	if err != nil {
		return err
	}
	for _, cred := range issued {
		if cred.Version == existing.Version {
			return nil
		}
	}

	var reviewers []string
	if h.RTRepo != nil {
		tasks, err := h.RTRepo.ReadAllByDID(ctx, tx, cmd.DID)
		if err != nil {
			return fmt.Errorf("read review tasks: %w", err)
		}
		for _, t := range tasks {
			if t.State == "VERIFIED" || t.State == "APPROVED" {
				reviewers = append(reviewers, t.Reviewer)
			}
		}
	}
	var approvers []string
	if h.ATRepo != nil {
		tasks, err := h.ATRepo.ReadAllByDID(ctx, tx, cmd.DID)
		if err != nil {
			return fmt.Errorf("read approval tasks: %w", err)
		}
		for _, t := range tasks {
			if t.State == "APPROVED" {
				approvers = append(approvers, t.Approver)
			}
		}
	}

	previousVCID, err := h.CTRepo.ReadLatestProvenanceVCID(ctx, tx, cmd.DID)
	if err != nil {
		return err
	}

	var templateBytes []byte
	if existing.TemplateData != nil {
		templateBytes = []byte(*existing.TemplateData)
	}

	claim := TemplateProvenanceClaim{
		TemplateDID:        cmd.DID,
		Version:            existing.Version,
		TemplateHash:       TemplateContentHash(templateBytes),
		CreatedBy:          existing.CreatedBy,
		ReviewedBy:         reviewers,
		ApprovedBy:         approvers,
		RegisteredBy:       cmd.RegisteredBy,
		RegistrarHolderDID: cmd.HolderDID,
		EffectiveAt:        time.Now().UTC(),
	}
	if previousVCID != nil {
		claim.PreviousCredentialID = *previousVCID
	}

	signedVC, vcID, err := IssueTemplateProvenanceVC(ctx, h.VCSigner, h.IssuerDID, claim)
	if err != nil {
		return err
	}

	return h.CTRepo.InsertProvenanceCredential(ctx, tx, db.TemplateProvenanceCredential{
		DID:          cmd.DID,
		Version:      existing.Version,
		VCID:         vcID,
		PreviousVCID: previousVCID,
		Credential:   datatype.JSON(signedVC),
	})
}

func templateDataFromAny(raw any) (*datatype.JSON, error) {
	if raw == nil {
		return nil, errors.New("template data is missing from Federated Catalogue")
	}

	var templateDataMap map[string]interface{}
	switch value := raw.(type) {
	case map[string]interface{}:
		templateDataMap = value
	case string:
		if strings.TrimSpace(value) == "" {
			return nil, errors.New("template data is missing from Federated Catalogue")
		}
		if err := json.Unmarshal([]byte(value), &templateDataMap); err != nil {
			return nil, fmt.Errorf("parse template data from Federated Catalogue: %w", err)
		}
	default:
		return nil, errors.New("invalid template data format from Federated Catalogue")
	}

	templateData, err := datatype.NewJSON(templateDataMap)
	if err != nil {
		return nil, fmt.Errorf("marshal template data failed: %w", err)
	}

	return &templateData, nil
}
