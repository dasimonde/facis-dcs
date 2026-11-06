package query

import (
	"context"
	"fmt"
	"time"

	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/signingmanagement/db"

	"github.com/jmoiron/sqlx"
)

// ClauseReportQry builds a PACM-07 clause-segmented compliance projection from
// live SHACL/content-policy validation — no new persisted assessment tables.
type ClauseReportQry struct {
	DID string
}

// ClauseReportFinding is one content-policy finding with conservative clause attribution.
type ClauseReportFinding struct {
	RuleID    string
	Severity  string
	Message   string
	Path      string
	FieldIri  string
	FocusNode string
	Source    string
	ClauseIDs []string
}

// ClauseReportSection is one clause component for the Compliance Viewer.
// Status is never "compliant" merely because findings are absent — coverage
// must establish applicable checks that passed (see validation.DeriveClauseCoverage).
type ClauseReportSection struct {
	ClauseID               string
	Title                  string
	Status                 string // not_assessed | undetermined | warnings_only | issues_identified | compliant
	Findings               []ClauseReportFinding
	AssessmentRun          string
	ApplicableChecks       string // display: Not established | None | shape names
	ApplicableChecksStatus string // not_established | none | established
	VerdictNote            string
}

// ClauseReportResult is the clause reporting projection.
type ClauseReportResult struct {
	DID                string
	AssessmentRun      string
	Clauses            []ClauseReportSection
	UnassignedFindings []ClauseReportFinding
	ScopeNote          string
}

// ClauseReportHandler loads the contract document, reuses AuditContractContent,
// maps findings to clause @ids conservatively, and derives coverage-aware status.
type ClauseReportHandler struct {
	DB    *sqlx.DB
	CRepo db.ContractRepo
}

func (h *ClauseReportHandler) Handle(ctx context.Context, qry ClauseReportQry) (*ClauseReportResult, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin clause report: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	contract, err := h.CRepo.ReadDataByDID(ctx, tx, qry.DID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit clause report read: %w", err)
	}

	assessmentRun := time.Now().UTC().Format(time.RFC3339)
	result := &ClauseReportResult{
		DID:           qry.DID,
		AssessmentRun: assessmentRun,
		ScopeNote: "Clause status uses live SHACL/ODRL/profile validation plus coverage " +
			"(typed shapes, field-linked rules, and DomainField constraints on clause-referenced " +
			"ContractFields). Empty findings alone are not compliance success.",
		Clauses: []ClauseReportSection{},
	}

	if contract.ContractData == nil || len(*contract.ContractData) == 0 {
		return result, nil
	}

	document := []byte(*contract.ContractData)
	clauses, err := validation.ExtractClauseComponents(document)
	if err != nil {
		return nil, fmt.Errorf("extract clauses: %w", err)
	}

	findings, err := validation.AuditContractContent(ctx, document, nil, validation.ContractContentAuditMetadata{
		ContractDID:     qry.DID,
		ContractVersion: fmt.Sprint(contract.ContractVersion),
	})
	if err != nil {
		return nil, fmt.Errorf("content validation for clause report: %w", err)
	}

	// Generic dcs:Clause blocks: assess uniquely referenced ContractFields via
	// dcs:shape → DomainField ValueConstraints (PACM-07), without rewriting @type.
	if enriched, enrichErr := validation.MergeClauseDomainFieldFindings(ctx, document, clauses, findings); enrichErr == nil {
		findings = enriched
	}

	index, indexErr := validation.LoadAuditShapeTargetIndex(ctx, document)
	indexOK := indexErr == nil

	docID := validation.DocumentID(document)
	projected, unassigned := validation.ProjectClauseSections(clauses, findings, docID, index, indexOK, true)

	sections := make([]ClauseReportSection, 0, len(projected))
	for _, p := range projected {
		sections = append(sections, ClauseReportSection{
			ClauseID:               p.ClauseID,
			Title:                  p.Title,
			Status:                 p.Status,
			Findings:               toClauseReportFindings(p.Findings),
			AssessmentRun:          assessmentRun,
			ApplicableChecks:       p.ApplicableChecks,
			ApplicableChecksStatus: p.ApplicableChecksStatus,
			VerdictNote:            p.VerdictNote,
		})
	}

	result.Clauses = sections
	result.UnassignedFindings = toClauseReportFindings(unassigned)
	return result, nil
}

func toClauseReportFindings(in []validation.PolicyFinding) []ClauseReportFinding {
	out := make([]ClauseReportFinding, 0, len(in))
	for _, f := range in {
		out = append(out, ClauseReportFinding{
			RuleID:    f.RuleID,
			Severity:  f.Severity,
			Message:   f.Message,
			Path:      f.Path,
			FieldIri:  f.FieldIri,
			FocusNode: f.FocusNode,
			Source:    f.Source,
		})
	}
	return out
}
