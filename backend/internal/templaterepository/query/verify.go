package query

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"digital-contracting-service/internal/base/validation"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/fcasset"
	fcclient "digital-contracting-service/internal/templatecatalogueintegration/client"
	"digital-contracting-service/internal/templaterepository/command"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/datatype/reviewtaskstate"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"

	"github.com/jmoiron/sqlx"
)

type VerifyQry struct {
	DID           string
	VerifiedBy    string
	ParticipantID string
	Token         string
	HolderDID     string
	UserRoles     userrole.UserRoles
}

type VerifyResult struct {
	// Findings block the review; Warnings do not.
	Findings []string
	Warnings []string
}

type Verifier struct {
	DB       *sqlx.DB
	CTRepo   db.ContractTemplateRepo
	RTRepo   db.ReviewTaskRepo
	FCClient *fcclient.FederatedCatalogueClient
	// VerifyCredential checks the proof of a template provenance credential against its issuer.
	VerifyCredential func(json.RawMessage) error
}

func (h *Verifier) Handle(ctx context.Context, cmd VerifyQry) (*VerifyResult, error) {

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("could not start transaction: %w", err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	processData, err := h.CTRepo.ReadProcessDataByDID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read process data: %w", err)
	}

	fullTemplate, err := h.CTRepo.ReadDataByID(ctx, tx, cmd.DID)
	if err != nil {
		return nil, fmt.Errorf("could not read template data: %w", err)
	}

	findings, warnings, err := contentFindings(ctx, fullTemplate)
	if err != nil {
		return nil, err
	}
	provenance, err := h.provenanceFindings(ctx, tx, fullTemplate)
	if err != nil {
		return nil, err
	}
	findings = append(findings, provenance...)
	if h.FCClient != nil {
		catalogueFindings, err := h.verifyTemplateResourceSelfDescription(ctx, cmd, processData, fullTemplate)
		if err != nil {
			return nil, err
		}
		findings = append(findings, catalogueFindings...)
	}

	// DCS-FR-TR-06: only a Template Reviewer's verification completes a review
	// task, and only when it has no findings. Warnings do not block.
	if len(findings) == 0 && cmd.UserRoles.HasRoles(userrole.TemplateReviewer) {
		hasTask, err := h.RTRepo.TaskExistsInState(ctx, tx, cmd.DID, cmd.VerifiedBy, reviewtaskstate.Open.String())
		if err != nil {
			return nil, err
		}
		if hasTask {
			if err := h.RTRepo.UpdateState(ctx, tx, cmd.DID, cmd.VerifiedBy, reviewtaskstate.Verified.String()); err != nil {
				return nil, err
			}
		}
	}

	// DCS-FR-TR-21: every verification is logged with its result.
	evt := templateevents.VerifyEvent{
		DID:        cmd.DID,
		Version:    processData.Version,
		VerifiedBy: cmd.VerifiedBy,
		Findings:   findings,
		Warnings:   warnings,
		OccurredAt: time.Now().UTC(),
		HolderDID:  cmd.HolderDID,
		UserRoles:  cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return nil, fmt.Errorf("could not create event: %w", err)
	}

	err = tx.Commit()
	if err != nil {
		return nil, fmt.Errorf("could not commit transaction: %w", err)
	}

	return &VerifyResult{
		Findings: findings,
		Warnings: warnings,
	}, nil
}

// contentFindings checks the template content (DCS-IR-TR-03, DCS-FR-TR-20):
// the canonical envelope, the template policy audit the approval runs, and
// the Semantic Hub shapes the template declares. Error findings are
// findings; warnings and information do not block.
func contentFindings(ctx context.Context, template *db.ContractTemplate) (findings, warnings []string, err error) {
	if _, err := validation.NormalizeTemplateData(template.TemplateData); err != nil {
		return []string{"template data is not valid: " + err.Error()}, nil, nil
	}
	results, err := command.ApprovalAudit(template)
	if err != nil {
		return nil, nil, fmt.Errorf("could not audit template policies: %w", err)
	}
	shapes, err := validation.AuditTemplateShapes(ctx, template.TemplateData)
	if err != nil {
		findings = append(findings, "Semantic Hub shapes validation failed: "+err.Error())
	}
	for _, result := range append(results, shapes...) {
		text := result.RuleID + ": " + result.Message
		switch result.Severity {
		case validation.SeverityError:
			findings = append(findings, text)
		case validation.SeverityWarning, validation.SeverityInfo:
			warnings = append(warnings, text)
		}
	}
	return findings, warnings, nil
}

// provenanceFindings checks the template against its provenance credentials
// (DCS-FR-TR-09, DCS-FR-TR-20): each credential proof must verify, and the
// credential of the current version must bind the current content hash. A
// template from REGISTERED onwards must have that credential.
func (h *Verifier) provenanceFindings(ctx context.Context, tx *sqlx.Tx, template *db.ContractTemplate) ([]string, error) {
	credentials, err := h.CTRepo.ReadProvenanceCredentials(ctx, tx, template.DID)
	if err != nil {
		return nil, err
	}

	var findings []string
	var current *db.TemplateProvenanceCredential
	for i := range credentials {
		credential := &credentials[i]
		if h.VerifyCredential == nil {
			findings = append(findings, fmt.Sprintf("provenance credential %s cannot be verified: no credential verifier is configured", credential.VCID))
		} else if err := h.VerifyCredential(json.RawMessage(credential.Credential)); err != nil {
			findings = append(findings, fmt.Sprintf("provenance credential %s does not verify: %v", credential.VCID, err))
		}
		if credential.Version == template.Version {
			current = credential
		}
	}

	if current == nil {
		switch template.State {
		case contracttemplatestate.Registered.String(), contracttemplatestate.Published.String(), contracttemplatestate.Deprecated.String():
			findings = append(findings, fmt.Sprintf("version %d has no provenance credential", template.Version))
		}
		return findings, nil
	}

	var sealed struct {
		CredentialSubject struct {
			TemplateHash string `json:"template_hash"`
		} `json:"credentialSubject"`
	}
	if err := json.Unmarshal(current.Credential, &sealed); err != nil {
		return nil, fmt.Errorf("decode provenance credential %s: %w", current.VCID, err)
	}
	var content []byte
	if template.TemplateData != nil {
		content = []byte(*template.TemplateData)
	}
	if sealed.CredentialSubject.TemplateHash != command.TemplateContentHash(content) {
		findings = append(findings, fmt.Sprintf("template content changed since approval: its hash does not match provenance credential %s", current.VCID))
	}
	return findings, nil
}

func (h *Verifier) verifyTemplateResourceSelfDescription(ctx context.Context, cmd VerifyQry, processData *db.ContractTemplateProcessData, fullTemplate *db.ContractTemplate) ([]string, error) {
	if h.FCClient == nil {
		return nil, fcclient.ErrFederatedCatalogueNotConfigured
	}

	if processData == nil {
		return nil, fmt.Errorf("process data is nil")
	}
	if fullTemplate == nil {
		return nil, fmt.Errorf("full template is nil")
	}

	findings := []string{}
	if cmd.HolderDID == "" {
		findings = append(findings, "holder did is empty")
	}

	if fullTemplate.Name != nil && *fullTemplate.Name == "" {
		findings = append(findings, "name is empty")
	}
	if fullTemplate.Description != nil && *fullTemplate.Description == "" {
		findings = append(findings, "description is empty")
	}

	payload, err := buildCatalogueVerificationPayload(cmd.DID, cmd.HolderDID, processData, fullTemplate)
	if err != nil {
		return nil, err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal template asset payload failed: %w", err)
	}

	query := url.Values{}
	query.Set("verifySemantics", "true")
	query.Set("verifySchema", "true")
	query.Set("verifySignatures", "true")
	query.Set("verifyVPSignature", "false")
	query.Set("verifyVCSignature", "false")

	resp, err := h.FCClient.Post(ctx, fcclient.VerificationEndpointPath, query, body)
	if err != nil {
		return nil, fmt.Errorf("verify template resource self-description failed: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if message := h.FCClient.ExtractErrorMessage(resp.Body); message != "" {
			findings = append(findings, fmt.Sprintf("verify template resource self-description failed: %s", message))
		} else {
			findings = append(findings, fmt.Sprintf("verify template resource self-description failed with status %d", resp.StatusCode))
		}
	}

	return findings, nil
}

func buildCatalogueVerificationPayload(
	did string,
	issuer string,
	processData *db.ContractTemplateProcessData,
	fullTemplate *db.ContractTemplate,
) (map[string]any, error) {
	templateDataString, err := fcasset.TemplateDataString(fullTemplate.TemplateData)
	if err != nil {
		return nil, fmt.Errorf("serialize template data for Federated Catalogue: %w", err)
	}

	return fcasset.BuildPayload(fcasset.BuildInput{
		Issuer:             issuer,
		SubjectIRI:         base.ResourceIRI("template", did),
		ValidFrom:          fullTemplate.UpdatedAt,
		Subject:            command.CatalogueSubject(processData, fullTemplate),
		TemplateDataString: templateDataString,
	})
}
