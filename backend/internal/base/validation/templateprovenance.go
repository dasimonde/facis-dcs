package validation

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"digital-contracting-service/internal/base/datatype"
)

const templateProvenancePolicySetID = "facis.dcs.template.approval-provenance"
const templateProvenancePolicyVersion = "v1"

func AuditTemplateApprovalProvenance(resourceDID string, entries []datatype.AuditLogEntry) []PolicyFinding {
	ordered := chronologicalAuditEntries(entries)
	state := templateProvenanceState{}
	findings := []PolicyFinding{}

	if strings.TrimSpace(resourceDID) == "" {
		resourceDID = auditEntryDID(ordered)
	}

	for index, entry := range ordered {
		eventData := decodeAuditEventData(entry.EventData)
		eventDID := stringField(eventData, "did")
		if eventDID != "" && resourceDID != "" && eventDID != resourceDID {
			findings = append(findings, templateProvenanceFinding(
				"FACIS-TPL-PROV-002",
				"Template audit event DID must match the resource DID",
				"error",
				fmt.Sprintf("event %s references DID %q but belongs to resource %q", entry.EventType, eventDID, resourceDID),
				"event_data.did",
			))
		}

		if index > 0 && entry.CreatedAt.Before(ordered[index-1].CreatedAt) {
			findings = append(findings, templateProvenanceFinding(
				"FACIS-TPL-PROV-006",
				"Template audit events must be chronological",
				"error",
				fmt.Sprintf("event %s at %s is older than the previous event at %s", entry.EventType, entry.CreatedAt.Format(time.RFC3339), ordered[index-1].CreatedAt.Format(time.RFC3339)),
				"created_at",
			))
		}

		findings = append(findings, auditTemplateProvenanceEvent(entry, eventData, &state)...)
	}

	if !state.created {
		findings = append(findings, templateProvenanceFinding(
			"FACIS-TPL-PROV-001",
			"Template provenance must start with a create event",
			"error",
			"CREATE_CONTRACT_TEMPLATE is missing from the resource audit trail",
			"event_type",
		))
	}

	if len(findings) == 0 {
		findings = append(findings, templateProvenanceFinding(
			"FACIS-TPL-PROV-000",
			"Template approval provenance is consistent",
			SeveritySatisfied,
			"create, review, approval, and registration provenance checks passed",
			"event_type",
		))
	}

	for i := range findings {
		findings[i].PolicySetID = templateProvenancePolicySetID
		findings[i].PolicyVersion = templateProvenancePolicyVersion
	}
	return findings
}

type templateProvenanceState struct {
	created      bool
	submitted    bool
	reviewed     bool
	approved     bool
	registered   bool
	creator      string
	reviewers    map[string]bool
	approver     string
	verifiedBy   map[string]bool
	reviewedBy   map[string]bool
	approvedBy   string
	registeredBy string
}

func auditTemplateProvenanceEvent(entry datatype.AuditLogEntry, eventData map[string]any, state *templateProvenanceState) []PolicyFinding {
	switch strings.ToUpper(entry.EventType) {
	case "CREATE_CONTRACT_TEMPLATE":
		state.created = true
		state.creator = stringField(eventData, "created_by")
	case "SUBMIT_CONTRACT_TEMPLATE":
		return auditTemplateSubmitProvenance(eventData, state)
	case "VERIFY_CONTRACT_TEMPLATE":
		verifiedBy := stringField(eventData, "verified_by")
		if state.verifiedBy == nil {
			state.verifiedBy = map[string]bool{}
		}
		if verifiedBy != "" {
			state.verifiedBy[verifiedBy] = true
		}
	case "APPROVE_CONTRACT_TEMPLATE":
		approvedBy := stringField(eventData, "approved_by")
		state.approved = true
		state.approvedBy = approvedBy
		if !state.reviewed {
			return []PolicyFinding{templateProvenanceFinding(
				"FACIS-TPL-PROV-004",
				"Template approval requires completed review",
				"error",
				"APPROVE_CONTRACT_TEMPLATE occurred before the template reached REVIEWED state",
				"event_type",
			)}
		}
		if state.approver != "" && approvedBy != "" && state.approver != approvedBy {
			return []PolicyFinding{templateProvenanceFinding(
				"FACIS-TPL-PROV-007",
				"Template must be approved by the assigned approver",
				"error",
				fmt.Sprintf("template was approved by %q but assigned approver is %q", approvedBy, state.approver),
				"event_data.approved_by",
			)}
		}
	case "REGISTER_CONTRACT_TEMPLATE":
		state.registered = true
		state.registeredBy = stringField(eventData, "registered_by")
		if !state.approved {
			return []PolicyFinding{templateProvenanceFinding(
				"FACIS-TPL-PROV-005",
				"Template registration requires prior approval",
				"error",
				"REGISTER_CONTRACT_TEMPLATE occurred before APPROVE_CONTRACT_TEMPLATE",
				"event_type",
			)}
		}
	}
	return nil
}

func auditTemplateSubmitProvenance(eventData map[string]any, state *templateProvenanceState) []PolicyFinding {
	previousState := strings.ToUpper(stringField(eventData, "previous_state"))
	newState := strings.ToUpper(stringField(eventData, "new_state"))
	submittedBy := stringField(eventData, "submitted_by")
	findings := []PolicyFinding{}

	if !state.created {
		findings = append(findings, templateProvenanceFinding(
			"FACIS-TPL-PROV-001",
			"Template provenance must start with a create event",
			"error",
			"SUBMIT_CONTRACT_TEMPLATE occurred before CREATE_CONTRACT_TEMPLATE",
			"event_type",
		))
	}

	if newState == "SUBMITTED" {
		state.submitted = true
		extractResponsible(eventData, state)
		if state.creator != "" && submittedBy != "" && state.creator != submittedBy && previousState != "SUBMITTED" {
			findings = append(findings, templateProvenanceFinding(
				"FACIS-TPL-PROV-003",
				"Initial template submission must be performed by the creator",
				"error",
				fmt.Sprintf("template was submitted by %q but created by %q", submittedBy, state.creator),
				"event_data.submitted_by",
			))
		}
	}
	if newState == "REVIEWED" {
		state.reviewed = true
		if state.reviewedBy == nil {
			state.reviewedBy = map[string]bool{}
		}
		if submittedBy != "" {
			state.reviewedBy[submittedBy] = true
		}
		if !state.submitted {
			findings = append(findings, templateProvenanceFinding(
				"FACIS-TPL-PROV-003",
				"Template review completion requires prior submission",
				"error",
				"template reached REVIEWED state without a prior SUBMITTED state",
				"event_data.new_state",
			))
		}
		if len(state.reviewers) > 0 && submittedBy != "" && !state.reviewers[submittedBy] {
			findings = append(findings, templateProvenanceFinding(
				"FACIS-TPL-PROV-008",
				"Template review transition must be performed by an assigned reviewer",
				"error",
				fmt.Sprintf("template was reviewed by %q who is not listed as responsible reviewer", submittedBy),
				"event_data.submitted_by",
			))
		}
	}
	if newState == "REJECTED" && !state.submitted {
		findings = append(findings, templateProvenanceFinding(
			"FACIS-TPL-PROV-003",
			"Template rejection requires prior submission",
			"error",
			"template reached REJECTED state without a prior SUBMITTED state",
			"event_data.new_state",
		))
	}
	return findings
}

func extractResponsible(eventData map[string]any, state *templateProvenanceState) {
	responsible, ok := eventData["responsible_persons"].(map[string]any)
	if !ok {
		return
	}
	if creator := stringField(responsible, "Creator"); creator != "" && state.creator == "" {
		state.creator = creator
	}
	if creator := stringField(responsible, "creator"); creator != "" && state.creator == "" {
		state.creator = creator
	}
	approver := stringField(responsible, "Approver")
	if approver == "" {
		approver = stringField(responsible, "approver")
	}
	if approver != "" {
		state.approver = approver
	}
	reviewers := arrayField(responsible, "Reviewers")
	if len(reviewers) == 0 {
		reviewers = arrayField(responsible, "reviewers")
	}
	if len(reviewers) > 0 && state.reviewers == nil {
		state.reviewers = map[string]bool{}
	}
	for _, reviewer := range reviewers {
		if reviewer != "" {
			state.reviewers[reviewer] = true
		}
	}
}

func chronologicalAuditEntries(entries []datatype.AuditLogEntry) []datatype.AuditLogEntry {
	ordered := make([]datatype.AuditLogEntry, len(entries))
	copy(ordered, entries)
	for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
		ordered[i], ordered[j] = ordered[j], ordered[i]
	}
	return ordered
}

func auditEntryDID(entries []datatype.AuditLogEntry) string {
	for _, entry := range entries {
		if entry.DID != nil && strings.TrimSpace(*entry.DID) != "" {
			return *entry.DID
		}
	}
	return ""
}

func decodeAuditEventData(raw json.RawMessage) map[string]any {
	var data map[string]any
	if len(raw) == 0 {
		return map[string]any{}
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return map[string]any{}
	}
	return data
}

func stringField(data map[string]any, key string) string {
	value, ok := data[key]
	if !ok {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func arrayField(data map[string]any, key string) []string {
	value, ok := data[key]
	if !ok {
		return nil
	}
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(values))
	for _, item := range values {
		if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
			result = append(result, strings.TrimSpace(text))
		}
	}
	return result
}

func templateProvenanceFinding(ruleID, title, severity, message, path string) PolicyFinding {
	return PolicyFinding{
		RuleID:       ruleID,
		Title:        title,
		Severity:     severity,
		Message:      message,
		Path:         path,
		OntologyTerm: "dcs:TemplateApprovalProvenance",
		Requirement:  "DCS-FR-PACM-03",
	}
}
