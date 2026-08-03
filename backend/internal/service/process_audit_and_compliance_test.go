package service

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	processauditandcompliance "digital-contracting-service/gen/process_audit_and_compliance"
	"digital-contracting-service/internal/base/datatype/componenttype"
	"digital-contracting-service/internal/middleware"
	"digital-contracting-service/internal/processauditandcompliance/auditexecutor"
)

func TestAuditEvidenceWireUsesExactSnakeCaseKeysForTimelineAndCheck(t *testing.T) {
	did := "did:example:contract"
	timeline, check := "TIMELINE", "CHECK"
	rule, result, reason := "ARCHIVE-1", "PASSED", "chain intact"
	resources := auditEvidenceToWire([]*auditEvidenceResource{{
		Did: did, Component: "contract-storage-archive", CreatedAt: "2026-07-31T10:00:00Z",
		AuditTrail: []*processauditandcompliance.PACResourceAuditTrailEntry{
			{ID: 1, Component: "contract-workflow-engine", EventType: "UPDATED", EventData: map[string]any{"ok": true}, Did: &did, CreatedAt: "2026-07-31T09:00:00Z", Kind: &timeline},
			{ID: 2, Component: "contract-storage-archive", EventType: "ARCHIVE_INTEGRITY_AUDIT_CHECK", EventData: map[string]any{}, Did: &did, CreatedAt: "2026-07-31T10:00:00Z", Kind: &check, RuleID: &rule, Result: &result, Reason: &reason},
		},
	}})
	raw, err := json.Marshal(resources)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	trail := decoded[0]["audit_trail"].([]any)
	want := []string{"component", "created_at", "did", "event_data", "event_type", "id", "kind", "reason", "res_log_pred_cid", "result", "rule_id"}
	for index, rawEntry := range trail {
		entry := rawEntry.(map[string]any)
		got := make([]string, 0, len(entry))
		for key := range entry {
			got = append(got, key)
		}
		slices.Sort(got)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("entry %d keys = %v, want %v; JSON=%s", index, got, want, raw)
		}
	}
	if trail[0].(map[string]any)["kind"] != "TIMELINE" || trail[1].(map[string]any)["kind"] != "CHECK" {
		t.Fatalf("wire kinds changed: %s", raw)
	}
}

type countingAuditExecutor struct {
	calls int
}

func (e *countingAuditExecutor) Run(context.Context, auditexecutor.Request) (auditexecutor.Response, []byte, error) {
	e.calls++
	return auditexecutor.Response{}, nil, nil
}

func TestAuditReportUsesPersistedRunWithoutExecutorAndHashesExactBytes(t *testing.T) {
	raw := []byte("{\n" +
		`  "contract_version":"facis-pac-audit-executor/v1",` + "\n" +
		`  "audit_id":"00000000-0000-4000-8000-000000000001",` + "\n" +
		`  "correlation_id":"00000000-0000-4000-8000-000000000001",` + "\n" +
		`  "scope":"contracts","resource":{"did":"did:example:contract"},` + "\n" +
		`  "executor":{"id":"test","version":"1"},"executed_at":"2026-07-28T12:00:00Z",` + "\n" +
		`  "findings":[{"rule_id":"RULE-1","result":"FAILED","reason":"failed","severity":"error","evidence_refs":["contracts/0"]}]` + "\n" +
		"}\n")
	executor := &countingAuditExecutor{}
	var recordedHash, recordedFormat, recordedJustification string
	var recordedSummary auditReportSummary
	service := &processAuditAndCompliancesrvc{
		AuditExecutor: executor,
		auditRunReader: func(_ context.Context, scope, did string) ([]byte, error) {
			if scope != "contracts" || did != "did:example:contract" {
				t.Fatalf("unexpected persisted-run lookup: scope=%q did=%q", scope, did)
			}
			return raw, nil
		},
		reportEventPersister: func(_ context.Context, report auditReport, format, contentHash, _, justification string) error {
			recordedHash = contentHash
			recordedFormat = format
			recordedJustification = justification
			recordedSummary = report.Summary
			return nil
		},
	}
	scope, format, did := "contracts", "json", "did:example:contract"
	content, err := service.AuditReport(context.Background(), &processauditandcompliance.AuditReportPayload{
		Scope: &scope, Format: &format, Did: &did, Justification: "report unit test",
	})
	if err != nil {
		t.Fatalf("AuditReport returned error: %v", err)
	}
	if string(content) != string(raw) {
		t.Fatalf("JSON report bytes changed:\n got %q\nwant %q", content, raw)
	}
	if executor.calls != 0 {
		t.Fatalf("report invoked executor %d times", executor.calls)
	}
	if recordedHash != hashBytes(raw) || recordedFormat != "json" || recordedJustification != "report unit test" {
		t.Fatalf("unexpected report event: hash=%q format=%q justification=%q", recordedHash, recordedFormat, recordedJustification)
	}
	if recordedSummary.TotalChecks != 1 || recordedSummary.Failed != 1 {
		t.Fatalf("unexpected persisted report summary: %+v", recordedSummary)
	}
}

func TestAuditRejectsBeforeExecutorDispatch(t *testing.T) {
	tests := []struct {
		name    string
		context context.Context
		scope   string
	}{
		{name: "unsupported scope", context: context.Background(), scope: "unsupported"},
		{
			name: "archive manager outside archive scope",
			context: middleware.InjectAuthContext(
				context.Background(), []string{"Archive Manager"}, "did:example:holder", "archive-manager",
			),
			scope: "contracts",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			executor := &countingAuditExecutor{}
			service := &processAuditAndCompliancesrvc{AuditExecutor: executor}
			_, err := service.Audit(test.context, &processauditandcompliance.PACAuditRequest{
				Scope: test.scope, Justification: "pre-dispatch unit test",
			})
			if err == nil {
				t.Fatal("Audit returned nil error")
			}
			if executor.calls != 0 {
				t.Fatalf("rejected audit invoked executor %d times", executor.calls)
			}
		})
	}
}

func TestResolveAuditScopeMapsUIScopes(t *testing.T) {
	tests := []struct {
		name       string
		scope      string
		scopeName  string
		component  componenttype.ComponentType
		template   bool
		contract   bool
		archive    bool
		provenance bool
	}{
		{
			name:       "templates",
			scope:      "templates",
			scopeName:  "templates",
			component:  componenttype.ContractTemplateRepo,
			template:   true,
			provenance: true,
		},
		{
			name:      "contracts",
			scope:     "contracts",
			scopeName: "contracts",
			component: componenttype.ContractWorkflowEngine,
			contract:  true,
		},
		{
			name:      "archive",
			scope:     "archive",
			scopeName: "archive",
			component: componenttype.ContractStorageArchive,
			archive:   true,
		},
		{
			name:      "signatures",
			scope:     "signatures",
			scopeName: "signatures",
			component: componenttype.SignatureManagement,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAuditScope(tt.scope)
			if err != nil {
				t.Fatalf("resolveAuditScope(%q) returned error: %v", tt.scope, err)
			}
			if got.component != tt.component {
				t.Fatalf("component = %s, want %s", got.component, tt.component)
			}
			if got.scopeName != tt.scopeName {
				t.Fatalf("scopeName = %s, want %s", got.scopeName, tt.scopeName)
			}
			if got.includeTemplatePolicyTrail != tt.template {
				t.Fatalf("includeTemplatePolicyTrail = %t, want %t", got.includeTemplatePolicyTrail, tt.template)
			}
			if got.includeTemplateProvenanceTrail != tt.provenance {
				t.Fatalf("includeTemplateProvenanceTrail = %t, want %t", got.includeTemplateProvenanceTrail, tt.provenance)
			}
			if got.includeContractContentTrail != tt.contract {
				t.Fatalf("includeContractContentTrail = %t, want %t", got.includeContractContentTrail, tt.contract)
			}
			if got.includeArchiveTrail != tt.archive {
				t.Fatalf("includeArchiveTrail = %t, want %t", got.includeArchiveTrail, tt.archive)
			}
		})
	}
}

func TestRenderAuditReportCSVAndPDF(t *testing.T) {
	report := auditReport{
		ReportID:    "pac-report-test",
		Scope:       "contracts",
		GeneratedAt: "2026-06-30T12:00:00Z",
		GeneratedBy: "auditor",
		Summary: auditReportSummary{
			TotalChecks: 1,
			Failed:      1,
		},
		Findings: []auditReportFinding{
			{Timestamp: "2026-06-30T10:05:00Z", DID: "did:example:contract", Component: "CONTRACT_WORKFLOW_ENGINE", EventType: "CONTRACT_CONTENT_POLICY_AUDIT_FINDING", RuleID: "rule,with,comma", Severity: "error", Message: "quoted \"message\"", Requirement: "value must be >= 99.9"},
		},
		Events: []auditReportEvent{{Timestamp: "2026-06-30T10:00:00Z", Actor: "did:web:actor", EventType: "CREATE_CONTRACT", DID: "did:example:contract"}},
	}

	csvBytes, err := renderAuditReportCSV(report)
	if err != nil {
		t.Fatalf("render csv: %v", err)
	}
	csvText := string(csvBytes)
	if !strings.Contains(csvText, `"rule,with,comma"`) || !strings.Contains(csvText, `"quoted ""message"""`) {
		t.Fatalf("csv does not contain escaped values: %s", csvText)
	}
	pdfBytes := renderAuditReportPDF(report)
	if !strings.HasPrefix(string(pdfBytes), "%PDF-") {
		t.Fatalf("pdf header missing: %q", string(pdfBytes[:8]))
	}
	if len(pdfBytes) < 100 {
		t.Fatalf("pdf too small: %d", len(pdfBytes))
	}
	if !strings.Contains(string(pdfBytes), "CREATE_CONTRACT") || !strings.Contains(string(pdfBytes), "did:web:actor") {
		t.Fatal("pdf does not include lifecycle actor and timestamp evidence")
	}
}

func TestResolveAuditScopeAcceptsComponentTypes(t *testing.T) {
	tests := []struct {
		name      string
		scope     string
		component componenttype.ComponentType
	}{
		{name: "template component", scope: "CONTRACT_TEMPLATE_REPOSITORY", component: componenttype.ContractTemplateRepo},
		{name: "workflow component lower case", scope: "contract_workflow_engine", component: componenttype.ContractWorkflowEngine},
		{name: "signature component", scope: "SIGNATURE_MANAGEMENT", component: componenttype.SignatureManagement},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveAuditScope(tt.scope)
			if err != nil {
				t.Fatalf("resolveAuditScope(%q) returned error: %v", tt.scope, err)
			}
			if got.component != tt.component {
				t.Fatalf("component = %s, want %s", got.component, tt.component)
			}
		})
	}
}

func TestResolveAuditScopeRejectsUnknownScope(t *testing.T) {
	if _, err := resolveAuditScope("unknown"); err == nil {
		t.Fatal("resolveAuditScope returned nil error for unknown scope")
	}
}

func TestValidateAuditScopeDependencies(t *testing.T) {
	service := &processAuditAndCompliancesrvc{}

	if err := service.validateAuditScopeDependencies(templateAuditScopeConfig()); err == nil {
		t.Fatal("validateAuditScopeDependencies returned nil error for missing template scope dependency")
	}
	if err := service.validateAuditScopeDependencies(contractAuditScopeConfig()); err == nil {
		t.Fatal("validateAuditScopeDependencies returned nil error for missing contract scope dependency")
	}
	if err := service.validateAuditScopeDependencies(auditScopeConfig{component: componenttype.SignatureManagement}); err != nil {
		t.Fatalf("validateAuditScopeDependencies returned error for scope without dependency requirement: %v", err)
	}
}
