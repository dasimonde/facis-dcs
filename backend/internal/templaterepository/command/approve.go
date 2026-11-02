package command

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype/userrole"

	"github.com/jmoiron/sqlx"

	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/base/templatestructure"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/templaterepository/datatype/approvaltaskstate"
	"digital-contracting-service/internal/templaterepository/datatype/contracttemplatestate"
	"digital-contracting-service/internal/templaterepository/db"
	templateevents "digital-contracting-service/internal/templaterepository/event"
)

type ApproveCmd struct {
	DID           string
	UpdatedAt     time.Time
	ApprovedBy    string
	DecisionNotes []string
	HolderDID     string
	UserRoles     userrole.UserRoles
}

type Approver struct {
	DB     *sqlx.DB
	CTRepo db.ContractTemplateRepo
	ATRepo db.ApprovalTaskRepo
}

func (h *Approver) Handle(ctx context.Context, cmd ApproveCmd) error {

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

	// Optimistic concurrency: reject if the caller's view of the template is
	// older than what's stored (see command package doc / ADR-0007). Templates
	// aren't peer-synced, so unlike contractworkflowengine there's no
	// local-vs-remote distinction in the error message here.
	if cmd.UpdatedAt.Unix() < processData.ContentUpdatedAt.Unix() {
		return fmt.Errorf("contract template %w, please reload", base.ErrUpdatedElsewhere)
	}

	if processData.State != contracttemplatestate.Reviewed.String() {
		return ErrInvalidTemplateState
	}

	valid, err := h.ATRepo.IsValidApprover(ctx, tx, cmd.DID, cmd.ApprovedBy)
	if err != nil {
		return err
	}

	if !valid {
		return fmt.Errorf("%w: invalid user", ErrInvalidRequest)
	}

	// DCS-FR-TR-26: the required templates can have changed since submission.
	if err := templatestructure.CheckTemplate(ctx, tx, cmd.DID); err != nil {
		return err
	}

	// DCS-IR-TR-05, DCS-IR-TR-06: the template policy audit refuses the
	// approval when it reports an error. A passed audit sets the regulatory
	// compliance status (DCS-FR-TR-10).
	template, err := h.CTRepo.ReadDataByID(ctx, tx, cmd.DID)
	if err != nil {
		return fmt.Errorf("could not read template data: %w", err)
	}
	findings, err := ApprovalAudit(template)
	if err != nil {
		return fmt.Errorf("could not audit template policies: %w", err)
	}
	if ComplianceStatus(findings) != Compliant {
		return &NonCompliantError{Findings: findings}
	}
	if err := h.CTRepo.UpdateComplianceStatus(ctx, tx, cmd.DID, Compliant); err != nil {
		return fmt.Errorf("could not update compliance status: %w", err)
	}

	err = h.ATRepo.UpdateState(ctx, tx, cmd.DID, cmd.ApprovedBy, approvaltaskstate.Approved.String())
	if err != nil {
		return fmt.Errorf("could not update approval task state: %w", err)
	}

	err = h.CTRepo.UpdateState(ctx, tx, cmd.DID, processData.State, contracttemplatestate.Approved.String())
	if err != nil {
		return fmt.Errorf("could not update current template state: %w", err)
	}

	evt := templateevents.ApproveEvent{
		DID:           cmd.DID,
		Version:       processData.Version,
		ApprovedBy:    cmd.ApprovedBy,
		DecisionNotes: cmd.DecisionNotes,
		OccurredAt:    time.Now().UTC(),
		HolderDID:     cmd.HolderDID,
		UserRoles:     cmd.UserRoles,
	}
	err = event.Create(ctx, tx, evt, componenttype.ContractTemplateRepo)
	if err != nil {
		return fmt.Errorf("could not create event: %w", err)
	}

	return tx.Commit()
}

// Compliance statuses of a template (DCS-FR-TR-10).
const (
	Compliant    = "COMPLIANT"
	NonCompliant = "NON_COMPLIANT"
)

// ErrTemplateNotCompliant refuses a template that has not passed the template policy audit.
var ErrTemplateNotCompliant = errors.New("the template has not passed the template policy audit")

// NonCompliantError lists the findings of the template policy audit that refuse an approval.
type NonCompliantError struct {
	Findings []validation.PolicyFinding
}

func (e *NonCompliantError) Error() string {
	var errs []string
	for _, finding := range e.Findings {
		if finding.Severity == validation.SeverityError {
			errs = append(errs, finding.RuleID+": "+finding.Message)
		}
	}
	return ErrTemplateNotCompliant.Error() + ": " + strings.Join(errs, "; ")
}

func (e *NonCompliantError) Unwrap() error { return ErrTemplateNotCompliant }

// ApprovalAudit runs the template policy audit for the state the approval
// sets (DCS-FR-TR-07).
func ApprovalAudit(template *db.ContractTemplate) ([]validation.PolicyFinding, error) {
	return validation.AuditTemplatePolicies(template.TemplateData, validation.TemplatePolicyAuditMetadata{
		DID:          template.DID,
		TemplateType: template.TemplateType,
		State:        contracttemplatestate.Approved.String(),
	})
}

// ComplianceStatus is NON_COMPLIANT when the template policy audit reports an
// error, else COMPLIANT.
func ComplianceStatus(findings []validation.PolicyFinding) string {
	for _, finding := range findings {
		if finding.Severity == validation.SeverityError {
			return NonCompliant
		}
	}
	return Compliant
}
