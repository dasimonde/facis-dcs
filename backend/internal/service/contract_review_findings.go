package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"sort"
	"time"

	"github.com/jmoiron/sqlx"

	cwe "digital-contracting-service/gen/contract_workflow_engine"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/contractworkflowengine/datatype/contractstate"
	cweeventtype "digital-contracting-service/internal/contractworkflowengine/datatype/eventtype"
	contractevents "digital-contracting-service/internal/contractworkflowengine/event"
)

// ReviewFindings returns the reviewer decisions on a contract with their
// comments and block annotations, so the approver inspects them before the
// decision (DCS-IR-CWE-08, DCS-FR-CWE-25).
func (s *contractWorkflowEnginesrvc) ReviewFindings(ctx context.Context, req *cwe.ReviewFindingsPayload) ([]*cwe.ContractReviewFinding, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	if err := authorizeContractReadDocument(ctx, s.DB, s.DIDDocument, req.Did); err != nil {
		return nil, err
	}

	tx, err := s.DB.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, cwe.MakeInternalError(err)
	}
	defer func(tx *sqlx.Tx) {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Printf("could not rollback transaction: %v", err)
		}
	}(tx)

	entries, err := s.ATrailReader.ReadAuditLogEntriesByComponentAndDID(ctx, tx, componenttype.ContractWorkflowEngine, req.Did)
	if err != nil {
		return nil, cwe.MakeInternalError(err)
	}
	return reviewFindings(entries), nil
}

// reviewFindings keeps the SUBMIT events a reviewer wrote on a SUBMITTED
// contract, newest first.
func reviewFindings(entries []datatype.AuditLogEntry) []*cwe.ContractReviewFinding {
	findings := []*cwe.ContractReviewFinding{}
	for _, entry := range entries {
		if entry.EventType != cweeventtype.Submit.String() {
			continue
		}
		var evt contractevents.SubmitEvent
		if json.Unmarshal(entry.EventData, &evt) != nil || evt.PreviousState != contractstate.Submitted.String() || evt.ActionFlag == nil {
			continue
		}
		decision := evt.ActionFlag.String()
		finding := &cwe.ContractReviewFinding{
			SubmittedBy:     evt.SubmittedBy,
			OccurredAt:      evt.OccurredAt.UTC().Format(time.RFC3339),
			ContractVersion: evt.ContractVersion,
			Decision:        &decision,
			Comments:        []string{},
			Annotations:     []*cwe.ContractBlockAnnotation{},
		}
		if evt.HolderDID != "" {
			finding.HolderDid = &evt.HolderDID
		}
		finding.Comments = append(finding.Comments, evt.Comments...)
		for _, annotation := range evt.Annotations {
			finding.Annotations = append(finding.Annotations, &cwe.ContractBlockAnnotation{BlockID: annotation.BlockID, Comment: annotation.Comment})
		}
		findings = append(findings, finding)
	}
	sort.SliceStable(findings, func(i, j int) bool { return findings[i].OccurredAt > findings[j].OccurredAt })
	return findings
}
