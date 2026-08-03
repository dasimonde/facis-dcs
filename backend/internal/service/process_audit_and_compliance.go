package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"digital-contracting-service/internal/base/datatype"
	baseevent "digital-contracting-service/internal/base/event"

	qry2 "digital-contracting-service/internal/processauditandcompliance/query"

	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/auth"
	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/conf"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/base/datatype/userrole"
	cwedb "digital-contracting-service/internal/contractworkflowengine/db"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/processauditandcompliance/auditexecutor"
	pacevent "digital-contracting-service/internal/processauditandcompliance/event"
	"digital-contracting-service/internal/processauditandcompliance/workflowgate"
	templatedb "digital-contracting-service/internal/templaterepository/db"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"goa.design/clue/log"
)

type processAuditAndCompliancesrvc struct {
	DB                   *sqlx.DB
	ATrailReader         base.AuditTrailReader
	CTRepo               templatedb.ContractTemplateRepo
	CRepo                cwedb.ContractRepo
	ATRepo               cwedb.ApprovalTaskRepo
	AuditExecutor        auditexecutor.Client
	WorkflowGate         *workflowgate.Coordinator
	auditRunReader       func(context.Context, string, string) ([]byte, error)
	reportEventPersister func(context.Context, auditReport, string, string, string, string) error
	auth.JWTAuthenticator
}

type auditEvidenceResource struct {
	Did        string                                                  `json:"did"`
	Component  string                                                  `json:"component"`
	CreatedAt  string                                                  `json:"created_at"`
	AuditTrail []*processauditandcompliance.PACResourceAuditTrailEntry `json:"audit_trail"`
}

// auditTrailEntryWire is deliberately independent of Goa's generated service
// types. Generated result structs have no JSON contract; this DTO fixes the
// executor wire keys and keeps TIMELINE and CHECK evidence stable.
type auditTrailEntryWire struct {
	ID            int64   `json:"id"`
	Component     string  `json:"component"`
	EventType     string  `json:"event_type"`
	EventData     any     `json:"event_data"`
	DID           *string `json:"did"`
	CreatedAt     string  `json:"created_at"`
	ResLogPredCID *string `json:"res_log_pred_cid"`
	Kind          *string `json:"kind"`
	RuleID        *string `json:"rule_id"`
	Result        *string `json:"result"`
	Reason        *string `json:"reason"`
}

type auditEvidenceResourceWire struct {
	DID        string                 `json:"did"`
	Component  string                 `json:"component"`
	CreatedAt  string                 `json:"created_at"`
	AuditTrail []*auditTrailEntryWire `json:"audit_trail"`
}

func auditEvidenceToWire(resources []*auditEvidenceResource) []*auditEvidenceResourceWire {
	result := make([]*auditEvidenceResourceWire, 0, len(resources))
	for _, resource := range resources {
		wire := &auditEvidenceResourceWire{DID: resource.Did, Component: resource.Component, CreatedAt: resource.CreatedAt}
		wire.AuditTrail = make([]*auditTrailEntryWire, 0, len(resource.AuditTrail))
		for _, entry := range resource.AuditTrail {
			wire.AuditTrail = append(wire.AuditTrail, &auditTrailEntryWire{
				ID: entry.ID, Component: entry.Component, EventType: entry.EventType,
				EventData: entry.EventData, DID: entry.Did, CreatedAt: entry.CreatedAt,
				ResLogPredCID: entry.ResLogPredCid, Kind: entry.Kind, RuleID: entry.RuleID,
				Result: entry.Result, Reason: entry.Reason,
			})
		}
		result = append(result, wire)
	}
	return result
}

type auditScopeConfig struct {
	scopeName                      string
	component                      componenttype.ComponentType
	requiresTemplateRepo           bool
	requiresContractRepo           bool
	includeTemplatePolicyTrail     bool
	includeTemplateProvenanceTrail bool
	includeContractContentTrail    bool
	includeArchiveTrail            bool
}

func NewProcessAuditAndCompliance(db *sqlx.DB, jwtAuth auth.JWTAuthenticator, auditTrailReader base.AuditTrailReader, ctRepo templatedb.ContractTemplateRepo, cRepo cwedb.ContractRepo, atRepo cwedb.ApprovalTaskRepo, executor auditexecutor.Client, gate *workflowgate.Coordinator) processauditandcompliance.Service {
	return &processAuditAndCompliancesrvc{DB: db, JWTAuthenticator: jwtAuth, ATrailReader: auditTrailReader, CTRepo: ctRepo, CRepo: cRepo, ATRepo: atRepo, AuditExecutor: executor, WorkflowGate: gate}
}

func (s *processAuditAndCompliancesrvc) Audit(ctx context.Context, req *processauditandcompliance.PACAuditRequest) (*processauditandcompliance.PACExternalAuditResponse, error) {
	scopeConfig, err := resolveAuditScope(req.Scope)
	if err != nil {
		return nil, processauditandcompliance.MakeBadRequest(err)
	}
	roles := middleware.GetUserRoles(ctx)
	if userrole.UserRoles(roles).HasRoles(userrole.ArchiveManager) && !userrole.UserRoles(roles).HasRoles(userrole.Auditor) && scopeConfig.scopeName != "archive" {
		return nil, processauditandcompliance.MakeForbidden(fmt.Errorf("Archive Manager may only audit archive scope"))
	}
	if s.AuditExecutor == nil {
		return nil, processauditandcompliance.MakeExecutorError(errors.New("audit executor is not configured"))
	}

	evidence, err := s.gatherAuditEvidence(ctx, req, scopeConfig)
	if err != nil {
		return nil, err
	}
	id := uuid.NewString()
	roleNames := make([]string, 0, len(roles))
	for _, role := range roles {
		roleNames = append(roleNames, role.String())
	}
	executorRequest := auditexecutor.Request{
		ContractVersion: auditexecutor.ContractVersion,
		AuditID:         id,
		CorrelationID:   id,
		Scope:           scopeConfig.scopeName,
		Requester: auditexecutor.Requester{
			Subject: middleware.GetParticipantID(ctx),
			Roles:   roleNames,
		},
		Justification: req.Justification,
		Evidence:      map[string]any{scopeConfig.scopeName: auditEvidenceToWire(evidence)},
	}
	if req.Did != nil && strings.TrimSpace(*req.Did) != "" {
		executorRequest.Resource = &auditexecutor.Resource{DID: strings.TrimSpace(*req.Did)}
	}
	executorResponse, rawResponse, err := s.AuditExecutor.Run(ctx, executorRequest)
	if err != nil {
		return nil, processauditandcompliance.MakeExecutorError(err)
	}
	if err := s.persistAuditRun(ctx, executorRequest, executorResponse, rawResponse); err != nil {
		return nil, processauditandcompliance.MakeInternalError(err)
	}
	return toPACExternalAuditResponse(executorResponse), nil
}

func (s *processAuditAndCompliancesrvc) gatherAuditEvidence(ctx context.Context, req *processauditandcompliance.PACAuditRequest, scopeConfig auditScopeConfig) (res []*auditEvidenceResource, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	if err := s.validateAuditScopeDependencies(scopeConfig); err != nil {
		return nil, processauditandcompliance.MakeInternalError(err)
	}
	scope := scopeConfig.component
	qry := qry2.GetAuditLogQry{
		Scope:         scope,
		AuditedBy:     middleware.GetParticipantID(ctx),
		HolderDID:     middleware.GetHolderDID(ctx),
		UserRoles:     middleware.GetUserRoles(ctx),
		Justification: req.Justification,
	}
	if req.Did != nil {
		qry.DID = strings.TrimSpace(*req.Did)
	}
	handler := qry2.Auditor{
		DB:           s.DB,
		ATrailReader: s.ATrailReader,
	}
	resLogHistories, err := handler.Handle(ctx, qry)
	if err != nil {
		return nil, processauditandcompliance.MakeInternalError(err)
	}

	contractContentEntriesByDID := make(map[string][]datatype.AuditLogEntry)
	if scopeConfig.includeContractContentTrail {
		contractContentTrailQry := qry2.GetContractContentTrailQry{
			RetrievedBy: middleware.GetParticipantID(ctx),
			HolderDID:   middleware.GetHolderDID(ctx),
			UserRoles:   middleware.GetUserRoles(ctx),
		}
		contractContentTrailHandler := qry2.ContractContentTrailAuditor{
			DB:    s.DB,
			CRepo: s.CRepo,
		}
		result, err := contractContentTrailHandler.Handle(ctx, contractContentTrailQry)
		if err != nil {
			return nil, processauditandcompliance.MakeInternalError(err)
		}
		contractContentEntriesByDID = result
	}

	templatePolicyEntriesByDID := make(map[string][]datatype.AuditLogEntry)
	if scopeConfig.includeTemplatePolicyTrail {
		policyTrailQry := qry2.GetContractPolicyTrailQry{
			RetrievedBy: middleware.GetParticipantID(ctx),
			HolderDID:   middleware.GetHolderDID(ctx),
			UserRoles:   middleware.GetUserRoles(ctx),
		}
		policyTrailHandler := qry2.ContractPolicyTrailAuditor{
			DB:     s.DB,
			CTRepo: s.CTRepo,
		}
		result, err := policyTrailHandler.Handle(ctx, policyTrailQry)
		if err != nil {
			return nil, processauditandcompliance.MakeInternalError(err)
		}
		templatePolicyEntriesByDID = result
	}

	archiveEntriesByDID := map[string][]*processauditandcompliance.PACResourceAuditTrailEntry{}
	if scopeConfig.includeArchiveTrail {
		result, err := s.auditArchiveTrailEntries(ctx)
		if err != nil {
			return nil, processauditandcompliance.MakeInternalError(err)
		}
		archiveEntriesByDID = result
	}

	result := make([]*auditEvidenceResource, 0)
	seenDIDs := map[string]bool{}
	for _, resLog := range resLogHistories {

		var did string
		history := make([]*processauditandcompliance.PACResourceAuditTrailEntry, 0)
		for _, entry := range resLog {

			if entry.DID != nil {
				did = *entry.DID
			}
			if !base.IsAuditVisibleEventType(entry.EventType) {
				continue
			}

			history = append(history, &processauditandcompliance.PACResourceAuditTrailEntry{
				ID:            entry.ID,
				Component:     entry.Component,
				EventType:     entry.EventType,
				EventData:     entry.EventData,
				Did:           entry.DID,
				CreatedAt:     entry.CreatedAt.Format(time.RFC3339),
				ResLogPredCid: entry.ResLogPredCID,
			})
		}
		if scopeConfig.includeTemplatePolicyTrail && did != "" {
			for _, entry := range templatePolicyEntriesByDID[did] {
				history = append(history, &processauditandcompliance.PACResourceAuditTrailEntry{
					ID:            entry.ID,
					Component:     entry.Component,
					EventType:     entry.EventType,
					EventData:     entry.EventData,
					Did:           entry.DID,
					CreatedAt:     entry.CreatedAt.Format(time.RFC3339),
					ResLogPredCid: entry.ResLogPredCID,
				})
			}
			seenDIDs[did] = true
		}
		if scopeConfig.includeTemplateProvenanceTrail && did != "" {

			provenanceQuery := qry2.GetTemplateApprovalProvenanceTrailQry{
				RetrievedBy: middleware.GetParticipantID(ctx),
				HolderDID:   middleware.GetHolderDID(ctx),
				UserRoles:   middleware.GetUserRoles(ctx),
				DID:         did,
				LogEntries:  resLog,
			}
			provenanceHandler := qry2.TemplateApprovalProvenanceTrailAuditor{}
			provenanceResult, err := provenanceHandler.Handle(ctx, provenanceQuery)
			if err != nil {
				return nil, processauditandcompliance.MakeInternalError(err)
			}

			for _, entry := range provenanceResult {
				history = append(history, &processauditandcompliance.PACResourceAuditTrailEntry{
					ID:            entry.ID,
					Component:     entry.Component,
					EventType:     entry.EventType,
					EventData:     entry.EventData,
					Did:           entry.DID,
					CreatedAt:     entry.CreatedAt.Format(time.RFC3339),
					ResLogPredCid: entry.ResLogPredCID,
				})
			}
		}
		if scopeConfig.includeContractContentTrail && did != "" {
			for _, entry := range contractContentEntriesByDID[did] {
				history = append(history, &processauditandcompliance.PACResourceAuditTrailEntry{
					ID:            entry.ID,
					Component:     entry.Component,
					EventType:     entry.EventType,
					EventData:     entry.EventData,
					Did:           entry.DID,
					CreatedAt:     entry.CreatedAt.Format(time.RFC3339),
					ResLogPredCid: entry.ResLogPredCID,
				})
			}
			seenDIDs[did] = true
		}
		if scopeConfig.includeArchiveTrail && did != "" {
			history = append(history, archiveEntriesByDID[did]...)
			seenDIDs[did] = true
		}
		if len(history) == 0 {
			continue
		}

		result = append(result, &auditEvidenceResource{
			Component:  scope.String(),
			Did:        did,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
			AuditTrail: history,
		})
	}
	for did, entries := range templatePolicyEntriesByDID {
		if seenDIDs[did] || len(entries) == 0 {
			continue
		}

		auditTrail := []*processauditandcompliance.PACResourceAuditTrailEntry{}
		for _, entry := range entries {
			auditTrail = append(auditTrail, &processauditandcompliance.PACResourceAuditTrailEntry{
				ID:            entry.ID,
				Component:     entry.Component,
				EventType:     entry.EventType,
				EventData:     entry.EventData,
				Did:           entry.DID,
				CreatedAt:     entry.CreatedAt.Format(time.RFC3339),
				ResLogPredCid: entry.ResLogPredCID,
			})
		}

		result = append(result, &auditEvidenceResource{
			Component:  componenttype.ContractTemplateRepo.String(),
			Did:        did,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
			AuditTrail: auditTrail,
		})
	}
	for did, entries := range contractContentEntriesByDID {
		// A content-audited contract with zero findings is a COMPLIANT result,
		// not an unaudited one — it must appear in the audit with an empty
		// trail (the SHACL engine only reports non-conformance, ADR-9).
		if seenDIDs[did] {
			continue
		}

		auditTrail := []*processauditandcompliance.PACResourceAuditTrailEntry{}
		for _, entry := range entries {
			auditTrail = append(auditTrail, &processauditandcompliance.PACResourceAuditTrailEntry{
				ID:            entry.ID,
				Component:     entry.Component,
				EventType:     entry.EventType,
				EventData:     entry.EventData,
				Did:           entry.DID,
				CreatedAt:     entry.CreatedAt.Format(time.RFC3339),
				ResLogPredCid: entry.ResLogPredCID,
			})
		}

		result = append(result, &auditEvidenceResource{
			Component:  componenttype.ContractWorkflowEngine.String(),
			Did:        did,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
			AuditTrail: auditTrail,
		})
	}
	for did, entries := range archiveEntriesByDID {
		if seenDIDs[did] || len(entries) == 0 {
			continue
		}
		result = append(result, &auditEvidenceResource{
			Component:  componenttype.ContractStorageArchive.String(),
			Did:        did,
			CreatedAt:  time.Now().UTC().Format(time.RFC3339),
			AuditTrail: entries,
		})
	}

	if req.Did != nil && strings.TrimSpace(*req.Did) != "" {
		filtered := result[:0]
		for _, response := range result {
			if response.Did == strings.TrimSpace(*req.Did) {
				filtered = append(filtered, response)
			}
		}
		result = filtered
	}
	if scopeConfig.scopeName == "signatures" {
		did := ""
		if req.Did != nil {
			did = strings.TrimSpace(*req.Did)
		}
		signatureEvidence, err := s.collectSignatureEvidence(ctx, did)
		if err != nil {
			return nil, processauditandcompliance.MakeInternalError(err)
		}
		result = mergeAuditEvidenceResources(result, signatureEvidence)
	}
	for _, response := range result {
		for _, entry := range response.AuditTrail {
			if entry.Kind == nil {
				entry.Kind = stringPointer("TIMELINE")
			}
		}
	}
	return result, nil
}

func resolveAuditScope(rawScope string) (auditScopeConfig, error) {
	normalizedScope := strings.TrimSpace(rawScope)
	switch strings.ToLower(normalizedScope) {
	case "templates":
		return templateAuditScopeConfig(), nil
	case "contracts":
		return contractAuditScopeConfig(), nil
	case "archive":
		return archiveAuditScopeConfig(), nil
	case "signatures":
		return auditScopeConfig{
			scopeName: "signatures",
			component: componenttype.SignatureManagement,
		}, nil
	}

	scope, err := componenttype.NewComponentType(normalizedScope)
	if err != nil {
		return auditScopeConfig{}, fmt.Errorf("invalid audit scope %q; allowed values are templates, contracts, archive, or a valid component type", rawScope)
	}

	switch scope {
	case componenttype.ContractTemplateRepo:
		return templateAuditScopeConfig(), nil
	case componenttype.ContractWorkflowEngine:
		return contractAuditScopeConfig(), nil
	case componenttype.ContractStorageArchive:
		return archiveAuditScopeConfig(), nil
	case componenttype.SignatureManagement:
		return auditScopeConfig{scopeName: "signatures", component: scope}, nil
	default:
		return auditScopeConfig{scopeName: scope.String(), component: scope}, nil
	}
}

func templateAuditScopeConfig() auditScopeConfig {
	return auditScopeConfig{
		scopeName:                      "templates",
		component:                      componenttype.ContractTemplateRepo,
		requiresTemplateRepo:           true,
		includeTemplatePolicyTrail:     true,
		includeTemplateProvenanceTrail: true,
	}
}

func contractAuditScopeConfig() auditScopeConfig {
	return auditScopeConfig{
		scopeName:                   "contracts",
		component:                   componenttype.ContractWorkflowEngine,
		requiresContractRepo:        true,
		includeContractContentTrail: true,
	}
}

func archiveAuditScopeConfig() auditScopeConfig {
	return auditScopeConfig{
		scopeName:            "archive",
		component:            componenttype.ContractStorageArchive,
		requiresContractRepo: true,
		includeArchiveTrail:  true,
	}
}

func (s *processAuditAndCompliancesrvc) validateAuditScopeDependencies(scopeConfig auditScopeConfig) error {
	if scopeConfig.requiresTemplateRepo && s.CTRepo == nil {
		return fmt.Errorf("audit scope %s is not configured", scopeConfig.scopeName)
	}
	if scopeConfig.requiresContractRepo && s.CRepo == nil {
		return fmt.Errorf("audit scope %s is not configured", scopeConfig.scopeName)
	}
	return nil
}

func (s *processAuditAndCompliancesrvc) AuditReport(ctx context.Context, p *processauditandcompliance.AuditReportPayload) (res []byte, err error) {
	log.Printf(ctx, "processAuditAndCompliance.audit_report")
	scope := "contracts"
	if p != nil && p.Scope != nil && strings.TrimSpace(*p.Scope) != "" {
		scope = strings.TrimSpace(*p.Scope)
	}
	format := "json"
	if p != nil && p.Format != nil && strings.TrimSpace(*p.Format) != "" {
		format = strings.ToLower(strings.TrimSpace(*p.Format))
	}
	did := ""
	if p != nil && p.Did != nil {
		did = strings.TrimSpace(*p.Did)
	}
	if format != "json" && format != "csv" && format != "pdf" {
		return nil, fmt.Errorf("unsupported audit report format %q", format)
	}
	roles := middleware.GetUserRoles(ctx)
	if userrole.UserRoles(roles).HasRoles(userrole.ArchiveManager) && !userrole.UserRoles(roles).HasRoles(userrole.Auditor) && strings.ToLower(scope) != "archive" {
		return nil, processauditandcompliance.MakeForbidden(fmt.Errorf("Archive Manager may only export archive scope"))
	}
	normalized, err := resolveAuditScope(scope)
	if err != nil {
		return nil, processauditandcompliance.MakeBadRequest(err)
	}
	raw, err := s.readLatestAuditRun(ctx, normalized.scopeName, did)
	if errors.Is(err, errPACAuditRunNotFound) {
		return nil, processauditandcompliance.MakeNotFound(err)
	}
	if err != nil {
		return nil, processauditandcompliance.MakeInternalError(err)
	}
	content, report, err := renderPersistedExecutorReport(raw, format, middleware.GetParticipantID(ctx), time.Now().UTC())
	if err != nil {
		return nil, processauditandcompliance.MakeInternalError(err)
	}
	contentHash := hashBytes(content)
	contentCID := ""
	if s.ATrailReader.IPFSClient != nil {
		stored, err := s.ATrailReader.IPFSClient.CreateFile(ctx, content)
		if err != nil {
			return nil, processauditandcompliance.MakeInternalError(fmt.Errorf("archive audit report bytes: %w", err))
		}
		if stored != nil {
			contentCID = stored.Identifier.Value
		}
	}
	justification := ""
	if p != nil {
		justification = p.Justification
	}
	if err := s.recordReportGenerated(ctx, report, format, contentHash, contentCID, justification); err != nil {
		return nil, processauditandcompliance.MakeInternalError(err)
	}
	return content, nil
}

func (s *processAuditAndCompliancesrvc) readLatestAuditRun(ctx context.Context, scope, did string) ([]byte, error) {
	if s.auditRunReader != nil {
		return s.auditRunReader(ctx, scope, did)
	}
	return s.latestAuditRun(ctx, scope, did)
}

func (s *processAuditAndCompliancesrvc) recordReportGenerated(ctx context.Context, report auditReport, format, contentHash, contentCID, justification string) error {
	if s.reportEventPersister != nil {
		return s.reportEventPersister(ctx, report, format, contentHash, contentCID, justification)
	}
	return s.persistReportGeneratedEvent(ctx, report, format, contentHash, contentCID, justification)
}

func (s *processAuditAndCompliancesrvc) persistReportGeneratedEvent(ctx context.Context, report auditReport, format string, contentHash, contentCID, justification string) error {
	if s.DB == nil {
		return nil
	}
	tx, err := s.DB.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("could not start transaction: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			_ = rollbackErr
		}
	}()
	evt := pacevent.ReportGeneratedEvent{
		ReportID:      report.ReportID,
		Scope:         report.Scope,
		Format:        format,
		DID:           report.DID,
		GeneratedBy:   report.GeneratedBy,
		GeneratedAt:   time.Now().UTC(),
		ContentHash:   contentHash,
		ContentCID:    contentCID,
		Justification: justification,
		Summary: map[string]int{
			"totalEvents": report.Summary.TotalEvents,
			"totalChecks": report.Summary.TotalChecks,
			"passed":      report.Summary.Passed,
			"failed":      report.Summary.Failed,
			"warnings":    report.Summary.Warnings,
			"needsReview": report.Summary.NeedsReview,
		},
		HolderDID: middleware.GetHolderDID(ctx),
		UserRoles: middleware.GetUserRoles(ctx),
	}
	reportScope, scopeErr := resolveAuditScope(report.Scope)
	if scopeErr != nil {
		return fmt.Errorf("resolve report audit scope: %w", scopeErr)
	}
	if err := baseevent.Create(ctx, tx, evt, reportScope.component); err != nil {
		return fmt.Errorf("could not create report event: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit report event: %w", err)
	}
	return nil
}

func (s *processAuditAndCompliancesrvc) Monitor(ctx context.Context, p *processauditandcompliance.MonitorPayload) (res *processauditandcompliance.PACMonitorResponse, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	handler := qry2.ComplianceMonitor{
		DB:     s.DB,
		ATRepo: s.ATRepo,
		CRepo:  s.CRepo,
	}
	result, err := handler.Handle(ctx, qry2.MonitorQry{
		MonitoredBy: middleware.GetParticipantID(ctx),
		HolderDID:   middleware.GetHolderDID(ctx),
		UserRoles:   middleware.GetUserRoles(ctx),
	})
	if err != nil {
		return nil, processauditandcompliance.MakeInternalError(err)
	}

	risks := make([]*processauditandcompliance.PACComplianceRisk, 0, len(result.Risks))
	for _, risk := range result.Risks {
		risks = append(risks, &processauditandcompliance.PACComplianceRisk{
			Did:        risk.DID,
			RiskType:   risk.RiskType,
			Detail:     risk.Detail,
			DetectedAt: risk.DetectedAt.Format(time.RFC3339),
		})
	}
	return &processauditandcompliance.PACMonitorResponse{
		CheckedAt: result.CheckedAt.Format(time.RFC3339),
		Risks:     risks,
	}, nil
}

func (s *processAuditAndCompliancesrvc) IncidentReport(ctx context.Context, p *processauditandcompliance.IncidentReportPayload) (res any, err error) {
	log.Printf(ctx, "processAuditAndCompliance.incident_report")

	if p == nil || len(p.Findings) == 0 {
		return map[string]any{"status": "accepted"}, nil
	}

	did := ""
	if p.ContractDid != nil {
		did = strings.TrimSpace(*p.ContractDid)
	}
	if did == "" && p.TemplateDid != nil {
		did = strings.TrimSpace(*p.TemplateDid)
	}
	if did == "" {
		return nil, processauditandcompliance.MakeBadRequest(fmt.Errorf("incident report findings must be linked to a contract_did or template_did"))
	}

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	findings := make([]qry2.IncidentFinding, 0, len(p.Findings))
	for _, finding := range p.Findings {
		if finding == nil {
			continue
		}
		findings = append(findings, qry2.IncidentFinding{RiskType: finding.RiskType, Detail: finding.Detail})
	}

	handler := qry2.IncidentReporter{DB: s.DB}
	if err := handler.Handle(ctx, qry2.IncidentReportQry{
		DID:        did,
		Findings:   findings,
		ReportedBy: middleware.GetParticipantID(ctx),
		HolderDID:  middleware.GetHolderDID(ctx),
		UserRoles:  middleware.GetUserRoles(ctx),
	}); err != nil {
		return nil, processauditandcompliance.MakeInternalError(err)
	}

	return map[string]any{"status": "recorded", "did": did, "findings": len(findings)}, nil
}

// CheckpointHead serves the newest audit-trail checkpoint head. Everything in
// the response is a hash, a count or a trusted timestamp, so a caller may
// publish it onward to an external notary — which is the point: a head held by
// someone we do not control pins every entry anchored before it (ADR-16).
func (s *processAuditAndCompliancesrvc) CheckpointHead(ctx context.Context, p *processauditandcompliance.CheckpointHeadPayload) (res *processauditandcompliance.PACCheckpointHead, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	handler := qry2.CheckpointAuditor{DB: s.DB, ARepo: s.ATrailReader.ARepo}
	head, err := handler.Head(ctx)
	if err != nil {
		return nil, processauditandcompliance.MakeInternalError(err)
	}
	if head == nil {
		return nil, processauditandcompliance.MakeNotFound(errors.New("no audit checkpoint has been anchored yet"))
	}
	return toCheckpointHead(*head), nil
}

func (s *processAuditAndCompliancesrvc) CheckpointBySequence(ctx context.Context, p *processauditandcompliance.CheckpointBySequencePayload) (*processauditandcompliance.PACCheckpointHead, error) {
	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()
	handler := qry2.CheckpointAuditor{DB: s.DB, ARepo: s.ATrailReader.ARepo}
	head, err := handler.BySequence(ctx, p.Seq)
	if err != nil {
		return nil, processauditandcompliance.MakeInternalError(err)
	}
	if head == nil {
		return nil, processauditandcompliance.MakeNotFound(fmt.Errorf("checkpoint %d not found", p.Seq))
	}
	return toCheckpointHead(*head), nil
}

// CheckpointProof serves the inclusion proof for one anchored entry. The entry
// itself is deliberately not part of the response: the verifier already holds
// it, and everything here is a hash, so the proof carries no system data.
func (s *processAuditAndCompliancesrvc) CheckpointProof(ctx context.Context, p *processauditandcompliance.CheckpointProofPayload) (res *processauditandcompliance.PACCheckpointProof, err error) {

	ctx, cancel := context.WithTimeout(ctx, conf.TransactionTimeout())
	defer cancel()

	handler := qry2.CheckpointAuditor{DB: s.DB, ARepo: s.ATrailReader.ARepo}
	proof, err := handler.Proof(ctx, p.EntryCid)
	if err != nil {
		return nil, processauditandcompliance.MakeInternalError(err)
	}
	if proof == nil {
		return nil, processauditandcompliance.MakeNotFound(fmt.Errorf("no checkpoint commits to entry %s", p.EntryCid))
	}

	return &processauditandcompliance.PACCheckpointProof{
		EntryCid:  proof.EntryCID,
		LeafHash:  proof.LeafHash,
		LeafIndex: proof.LeafIndex,
		Siblings:  proof.Siblings,
		Head:      toCheckpointHead(proof.Head),
	}, nil
}

func toCheckpointHead(head qry2.CheckpointHead) *processauditandcompliance.PACCheckpointHead {
	result := &processauditandcompliance.PACCheckpointHead{
		Seq:          head.Seq,
		Root:         head.Root,
		PrevRoot:     head.PrevRoot,
		LeafCount:    head.LeafCount,
		CreatedAt:    head.CreatedAt.UTC().Format(time.RFC3339),
		TsaTimestamp: head.TsaTimestamp,
	}
	if head.TimestampedAt != nil {
		timestampedAt := head.TimestampedAt.UTC().Format(time.RFC3339)
		result.TimestampedAt = &timestampedAt
	}
	return result
}
