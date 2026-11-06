package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	goa "goa.design/goa/v3/pkg"

	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	baseevent "digital-contracting-service/internal/base/event"
	"digital-contracting-service/internal/processauditandcompliance/auditexecutor"
	pacevent "digital-contracting-service/internal/processauditandcompliance/event"
)

var errPACAuditRunNotFound = errors.New("no matching persisted PAC audit run")

func (s *processAuditAndCompliancesrvc) persistAuditRun(
	ctx context.Context,
	request auditexecutor.Request,
	response auditexecutor.Response,
	rawResponse []byte,
) error {
	if s.DB == nil {
		return nil
	}
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("marshal audit request for persistence: %w", err)
	}
	requestHash := sha256String(requestJSON)
	responseHash := sha256String(rawResponse)
	executedAt, _ := time.Parse(time.RFC3339, response.ExecutedAt)
	var resourceDID any
	did := ""
	if request.Resource != nil {
		did = request.Resource.DID
		resourceDID = did
	}
	if s.ATrailReader.Artifacts == nil {
		return fmt.Errorf("PAC snapshot encryption is not configured")
	}
	store := s.ATrailReader.Artifacts
	scopes, err := pacEvidenceScopes(requestJSON, store.InstanceScope())
	if err != nil {
		return err
	}
	payload, err := json.Marshal(pacSnapshotPayload{RequestBytes: requestJSON, ResponseBytes: rawResponse})
	if err != nil {
		return fmt.Errorf("marshal PAC snapshot: %w", err)
	}
	envelope, err := sealPACSnapshot(ctx, store, request.AuditID, scopes, payload)
	if err != nil {
		return err
	}
	snapshot, err := store.Encrypt(ctx, store.InstanceScope(), envelope)
	if err != nil {
		return fmt.Errorf("encrypt PAC snapshot envelope: %w", err)
	}

	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin PAC audit run transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback()
	}()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO pac_audit_runs (
			audit_id, correlation_id, contract_version, scope, resource_did,
			requester, justification, request_hash, response_hash, executor_id,
			executor_version, executed_at, snapshot_bytes
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		request.AuditID, request.CorrelationID, request.ContractVersion, request.Scope,
		resourceDID, request.Requester.Subject, request.Justification, requestHash,
		responseHash, response.Executor.ID, response.Executor.Version, executedAt,
		snapshot,
	)
	if err != nil {
		return fmt.Errorf("insert PAC audit run: %w", err)
	}
	event := pacevent.AuditExecutedEvent{
		AuditID: request.AuditID, CorrelationID: request.CorrelationID,
		Scope: request.Scope, DID: did, ExecutorID: response.Executor.ID,
		ExecutorVersion: response.Executor.Version, ExecutedAt: executedAt,
		RequestHash: requestHash, ResponseHash: responseHash,
	}
	scopeConfig, err := resolveAuditScope(request.Scope)
	if err != nil {
		return err
	}
	if err := baseevent.Create(ctx, tx, event, scopeConfig.component); err != nil {
		return fmt.Errorf("persist PAC_AUDIT_EXECUTED event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit PAC audit run: %w", err)
	}
	return nil
}

func (s *processAuditAndCompliancesrvc) latestAuditRun(ctx context.Context, scope, did string) (persistedAuditRun, error) {
	var run persistedAuditRun
	if s.DB == nil || s.ATrailReader.Artifacts == nil {
		return run, fmt.Errorf("PAC audit run persistence or encryption is not configured")
	}
	var before, beforeID any
	var denied error
	for {
		var snapshot []byte
		var auditID string
		var createdAt time.Time
		err := s.DB.QueryRowxContext(ctx, `
		SELECT audit_id, request_hash, response_hash, snapshot_bytes, created_at
		FROM pac_audit_runs
		WHERE scope = $1
		  AND (($2 = '' AND resource_did IS NULL) OR resource_did = $2)
		  AND ($3::timestamptz IS NULL OR (created_at, audit_id) < ($3, $4::uuid))
		ORDER BY created_at DESC, audit_id DESC
		LIMIT 1`, scope, did, before, beforeID).Scan(&auditID, &run.RequestHash, &run.ResponseHash, &snapshot, &createdAt)
		if errors.Is(err, sql.ErrNoRows) {
			if denied != nil {
				return persistedAuditRun{}, denied
			}
			return run, errPACAuditRunNotFound
		}
		if err != nil {
			return run, fmt.Errorf("read latest PAC audit run: %w", err)
		}
		store := s.ATrailReader.Artifacts
		envelope, err := store.Decrypt(ctx, store.InstanceScope(), snapshot)
		if err != nil {
			return run, fmt.Errorf("decrypt PAC snapshot envelope: %w", err)
		}
		payloadBytes, scopes, err := unsealPACSnapshot(ctx, store, auditID, envelope)
		if err != nil {
			return run, err
		}
		var payload pacSnapshotPayload
		if err := json.Unmarshal(payloadBytes, &payload); err != nil {
			return run, fmt.Errorf("decode PAC snapshot payload: %w", err)
		}
		run.RequestBytes, run.ResponseBytes, run.Scopes = payload.RequestBytes, payload.ResponseBytes, scopes
		if sha256String(run.RequestBytes) != run.RequestHash || sha256String(run.ResponseBytes) != run.ResponseHash {
			return persistedAuditRun{}, fmt.Errorf("PAC snapshot hash mismatch")
		}
		var request auditexecutor.Request
		if err := json.Unmarshal(run.RequestBytes, &request); err != nil {
			return persistedAuditRun{}, err
		}
		if request.AuditID != auditID || request.Scope != scope || (request.Resource == nil && did != "") || (request.Resource != nil && request.Resource.DID != did) {
			return persistedAuditRun{}, fmt.Errorf("PAC snapshot does not match selected run")
		}
		expectedScopes, err := pacEvidenceScopes(run.RequestBytes, store.InstanceScope())
		if err != nil {
			return persistedAuditRun{}, err
		}
		if !equalPACScopes(scopes, expectedScopes) {
			return persistedAuditRun{}, fmt.Errorf("PAC snapshot scopes do not match evidence")
		}
		if err := s.authorizeAuditReport(ctx, run); err != nil {
			var serviceError *goa.ServiceError
			if !errors.As(err, &serviceError) || serviceError.Name != "forbidden" {
				return persistedAuditRun{}, err
			}
			denied = err
			before, beforeID = createdAt, auditID
			continue
		}
		return run, nil
	}
}

// toPACExternalAuditResponse renders the executor verdict plus the timeline the
// DCS procured and submitted as evidence. The evidence is grouped per audited
// resource for the executor request; the `timeline` attribute is a flat entry
// list, so the groups are concatenated and each entry keeps its own `did`.
func toPACExternalAuditResponse(response auditexecutor.Response, evidence []*auditEvidenceResource) *processauditandcompliance.PACExternalAuditResponse {
	findings := make([]*processauditandcompliance.PACAuditFinding, 0, len(response.Findings))
	for _, finding := range response.Findings {
		findings = append(findings, &processauditandcompliance.PACAuditFinding{
			RuleID: finding.RuleID, Result: finding.Result, Reason: finding.Reason,
			Severity: optionalString(finding.Severity), EvidenceRefs: finding.EvidenceRefs,
		})
	}
	var resource any
	if response.Resource != nil {
		resource = map[string]any{"did": response.Resource.DID}
	}
	var receipt any
	if len(response.Receipt) != 0 && string(response.Receipt) != "null" {
		_ = json.Unmarshal(response.Receipt, &receipt)
	}
	timeline := make([]*processauditandcompliance.PACResourceAuditTrailEntry, 0, len(evidence))
	for _, audited := range evidence {
		if audited == nil {
			continue
		}
		for _, entry := range audited.AuditTrail {
			if entry != nil {
				timeline = append(timeline, entry)
			}
		}
	}
	return &processauditandcompliance.PACExternalAuditResponse{
		ContractVersion: response.ContractVersion, AuditID: response.AuditID,
		CorrelationID: response.CorrelationID, Scope: response.Scope, Resource: resource,
		Executor:   &processauditandcompliance.PACAuditExecutor{ID: response.Executor.ID, Version: response.Executor.Version},
		ExecutedAt: response.ExecutedAt, Findings: findings, Receipt: receipt, Timeline: timeline,
	}
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func sha256String(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
