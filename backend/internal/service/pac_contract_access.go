package service

import (
	"context"
	"encoding/json"
	"fmt"

	pac "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/base/contractreadpolicy"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/processauditandcompliance/auditexecutor"
)

func (s *processAuditAndCompliancesrvc) contractAuditContext(ctx context.Context) (context.Context, error) {
	scope := contractreadpolicy.Scope{Organization: middleware.GetParticipantID(ctx)}
	if s.ATrailReader.Artifacts != nil {
		scope.LocalPeer = s.ATrailReader.Artifacts.InstanceScope().ID
	}
	if scope.LocalPeer == "" {
		return ctx, pac.MakeForbidden(contractreadpolicy.ErrDenied)
	}
	return contractreadpolicy.WithScope(ctx, scope), nil
}

func (s *processAuditAndCompliancesrvc) authorizeContract(ctx context.Context, did string) error {
	scoped, err := s.contractAuditContext(ctx)
	if err != nil {
		return err
	}
	scope, _ := contractreadpolicy.FromContext(scoped)
	return authorizeContractRead(ctx, s.DB, scope.LocalPeer, did)
}

func filterContractAuditEvidence(evidence []*auditEvidenceResource, allowed map[string]bool) []*auditEvidenceResource {
	if allowed == nil {
		return evidence
	}
	filtered := make([]*auditEvidenceResource, 0, len(evidence))
	for _, resource := range evidence {
		if !allowed[resource.Did] {
			continue
		}
		entries := make([]*pac.PACResourceAuditTrailEntry, 0, len(resource.AuditTrail))
		for _, entry := range resource.AuditTrail {
			if entry.Did != nil && !allowed[*entry.Did] {
				continue
			}
			entries = append(entries, entry)
		}
		copy := *resource
		copy.AuditTrail = entries
		filtered = append(filtered, &copy)
	}
	return filtered
}

// Frozen reports are indivisible: authorize every resource, including erased placeholders.
func (s *processAuditAndCompliancesrvc) authorizeAuditReport(ctx context.Context, run persistedAuditRun) error {
	ctx, err := s.contractAuditContext(ctx)
	if err != nil {
		return err
	}
	allowed, err := readableContractDIDs(ctx, s.DB)
	if err != nil || allowed == nil {
		return err
	}
	var request struct {
		Scope    string                     `json:"scope"`
		Resource *auditexecutor.Resource    `json:"resource"`
		Evidence map[string]json.RawMessage `json:"evidence"`
	}
	if err := json.Unmarshal(run.RequestBytes, &request); err != nil {
		return pac.MakeInternalError(fmt.Errorf("decode PAC snapshot request: %w", err))
	}
	var response auditexecutor.Response
	if err := json.Unmarshal(run.ResponseBytes, &response); err != nil {
		return pac.MakeInternalError(fmt.Errorf("decode PAC snapshot response: %w", err))
	}
	if request.Scope != "archive" && request.Scope != "contracts" {
		return pac.MakeForbidden(contractreadpolicy.ErrDenied)
	}
	if request.Resource != nil && !allowed[request.Resource.DID] {
		return pac.MakeForbidden(contractreadpolicy.ErrDenied)
	}
	for scope, raw := range request.Evidence {
		if scope != "archive" && scope != "contracts" {
			return pac.MakeForbidden(contractreadpolicy.ErrDenied)
		}
		var resources []persistedAuditEvidenceResource
		if err := json.Unmarshal(raw, &resources); err != nil {
			return pac.MakeInternalError(fmt.Errorf("decode PAC snapshot evidence: %w", err))
		}
		for _, resource := range resources {
			if !allowed[resource.DID] {
				return pac.MakeForbidden(contractreadpolicy.ErrDenied)
			}
			for _, event := range resource.AuditTrail {
				if event.DID != "" && !allowed[event.DID] {
					return pac.MakeForbidden(contractreadpolicy.ErrDenied)
				}
			}
		}
	}
	if response.Resource != nil && !allowed[response.Resource.DID] {
		return pac.MakeForbidden(contractreadpolicy.ErrDenied)
	}
	return nil
}
