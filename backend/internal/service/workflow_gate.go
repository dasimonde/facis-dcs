package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/processauditandcompliance/workflowgate"
)

func (s *processAuditAndCompliancesrvc) WorkflowGateRun(ctx context.Context, p *processauditandcompliance.WorkflowGateRunPayload) (*processauditandcompliance.PACWorkflowGateRun, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()
	run, err := s.WorkflowGate.Read(ctx, p.RunID, "", "")
	if errors.Is(err, sql.ErrNoRows) {
		return nil, processauditandcompliance.MakeNotFound(fmt.Errorf("workflow gate run %s not found", p.RunID))
	}
	if err != nil {
		return nil, processauditandcompliance.MakeInternalError(err)
	}
	if err := s.authorizeContract(ctx, run.ContractDID); err != nil {
		return nil, err
	}
	return workflowGateRunResponse(run), nil
}

// WorkflowGateReview records the decision on a REVIEW run of a contract the
// caller may read (DCS-FR-PACM-03, ADR-31).
func (s *processAuditAndCompliancesrvc) WorkflowGateReview(ctx context.Context, p *processauditandcompliance.WorkflowGateReviewPayload) (*processauditandcompliance.PACWorkflowGateReviewDecision, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()
	run, err := s.WorkflowGate.Read(ctx, p.RunID, "", "")
	if errors.Is(err, sql.ErrNoRows) {
		return nil, processauditandcompliance.MakeNotFound(fmt.Errorf("workflow gate run %s not found", p.RunID))
	}
	if err != nil {
		return nil, processauditandcompliance.MakeInternalError(err)
	}
	if err := s.authorizeContract(ctx, run.ContractDID); err != nil {
		return nil, err
	}
	decision, err := s.WorkflowGate.DecideReview(ctx, p.RunID, p.Decision, p.Justification, middleware.GetParticipantID(ctx))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, processauditandcompliance.MakeNotFound(err)
	}
	if err != nil {
		return nil, processauditandcompliance.MakeBadRequest(err)
	}
	return &processauditandcompliance.PACWorkflowGateReviewDecision{
		RunID: decision.RunID, Decision: decision.Decision,
		Justification: decision.Justification, DecidedBy: decision.DecidedBy,
		DecidedAt: decision.DecidedAt.Format(time.RFC3339Nano),
	}, nil
}

// WorkflowGateRuns lists the gate runs of the contracts the caller may read,
// by default the REVIEW runs that wait for a decision (DCS-FR-PACM-03).
func (s *processAuditAndCompliancesrvc) WorkflowGateRuns(ctx context.Context, p *processauditandcompliance.WorkflowGateRunsPayload) ([]*processauditandcompliance.PACWorkflowGateRun, error) {
	scoped, err := s.contractAuditContext(ctx)
	if err != nil {
		return nil, err
	}
	allowed, err := readableContractDIDs(scoped, s.DB)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()
	status, pending := "REVIEW", true
	if p.Status != nil {
		status = *p.Status
	}
	if p.Pending != nil {
		pending = *p.Pending
	}
	runs, err := s.WorkflowGate.List(ctx, status, pending && status == "REVIEW")
	if err != nil {
		return nil, processauditandcompliance.MakeInternalError(err)
	}
	result := make([]*processauditandcompliance.PACWorkflowGateRun, 0, len(runs))
	for _, run := range runs {
		if allowed == nil || allowed[run.ContractDID] {
			result = append(result, workflowGateRunResponse(run))
		}
	}
	return result, nil
}

func workflowGateRunResponse(run workflowgate.Run) *processauditandcompliance.PACWorkflowGateRun {
	createdAt := run.CreatedAt.UTC().Format(time.RFC3339)
	result := &processauditandcompliance.PACWorkflowGateRun{
		RunID: run.RunID, CorrelationID: run.CorrelationID, SnapshotID: run.SnapshotID,
		ContractDid: run.ContractDID, ContractVersion: run.ContractVersion,
		Gate: run.Gate, Status: run.Status, ContentHash: run.ContentHash,
		EffectiveShapes: run.EffectiveShapes, ProfileVersion: run.ProfileVersion,
		Findings: run.Findings, CreatedAt: &createdAt,
	}
	if run.Status == "REVIEW" {
		review := &processauditandcompliance.PACWorkflowGateReview{Status: "PENDING"}
		if run.Review != nil {
			review.Status = "DECIDED"
			review.Decision = &run.Review.Decision
			review.Justification = &run.Review.Justification
			review.DecidedBy = &run.Review.DecidedBy
			decidedAt := run.Review.DecidedAt.Format(time.RFC3339Nano)
			review.DecidedAt = &decidedAt
		}
		result.Review = review
	}
	return result
}
