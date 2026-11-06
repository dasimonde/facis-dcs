package service

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"digital-contracting-service/internal/base/validation"
)

func reportSnapshotFixture(t *testing.T) persistedAuditRun {
	t.Helper()
	request := []byte(`{"contract_version":"1","audit_id":"audit-1","correlation_id":"correlation-1","scope":"contracts","resource":{"did":"did:example:contract"},"evidence":{"contracts":[{"did":"did:example:contract","component":"CONTRACT_WORKFLOW_ENGINE","audit_trail":[{"id":3,"kind":"TIMELINE","created_at":"2026-10-15T11:00:00+02:00","event_type":"THIRD_EVENT","event_data":{"stored_by":"actor-c"}},{"id":2,"kind":"TIMELINE","created_at":"2026-10-15T09:00:00Z","event_type":"SECOND_EVENT","event_data":{"stored_by":"actor-b"}},{"id":9,"kind":"CHECK","created_at":"invalid-check-time","event_type":"NOT_A_LIFECYCLE_EVENT"},{"id":1,"kind":"TIMELINE","created_at":"2026-10-15T08:00:00Z","event_type":"FIRST_EVENT","event_data":{"stored_by":"actor-a","note":"snapshot"}}]}]}}`)
	response := []byte(`{"contract_version":"1","audit_id":"audit-1","correlation_id":"correlation-1","scope":"contracts","resource":{"did":"did:example:contract"},"executor":{"id":"executor","version":"v1"},"executed_at":"2026-10-15T10:00:00Z","findings":[{"rule_id":"rule-1","result":"FAILED","reason":"reason","severity":"error","evidence_refs":["entry:1"]}],"receipt":{"proof":"unchanged"}}`)
	return persistedAuditRun{RequestBytes: request, ResponseBytes: response, RequestHash: hashBytes(request), ResponseHash: hashBytes(response)}
}

func TestPersistedAuditReportIncludesFrozenTimeline(t *testing.T) {
	run := reportSnapshotFixture(t)
	original := append([]byte(nil), run.ResponseBytes...)
	for _, format := range []string{"json", "csv", "pdf"} {
		t.Run(format, func(t *testing.T) {
			content, report, err := renderPersistedExecutorReport(run, format, "exporter", time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatal(err)
			}
			if report.Summary.TotalEvents != 3 || report.Summary.TotalChecks != 1 || len(report.Resources) != 1 || report.Resources[0].EventCount != 3 || report.Resources[0].FindingCount != 1 {
				t.Fatalf("incorrect counts: %+v", report)
			}
			for i, event := range report.Events {
				if event.ID != int64(i+1) {
					t.Fatalf("events not sorted by instant then id: %+v", report.Events)
				}
			}
			if report.Events[0].Actor != "actor-a" || report.Events[0].Details["note"] != "snapshot" {
				t.Fatalf("event details lost: %+v", report.Events[0])
			}
			if report.Findings[0].Result != "FAILED" || len(report.Findings[0].EvidenceRefs) != 1 || report.Executor.ID != "executor" || string(report.Receipt) != `{"proof":"unchanged"}` {
				t.Fatalf("executor provenance lost: %+v", report)
			}
			if report.RequestHash != run.RequestHash || report.ResponseHash != run.ResponseHash || hashBytes(content) == run.ResponseHash {
				t.Fatal("report hash confused with original response hash")
			}
			if bytes.Contains(content, []byte("NOT_A_LIFECYCLE_EVENT")) {
				t.Fatal("CHECK exported as lifecycle event")
			}
			for _, event := range report.Events {
				if !bytes.Contains(content, []byte(event.EventType)) || !bytes.Contains(content, []byte(event.Actor)) {
					t.Fatalf("%s lacks lifecycle evidence", format)
				}
			}
			switch format {
			case "json":
				var decoded auditReport
				if err := json.Unmarshal(content, &decoded); err != nil || decoded.ReportID != "audit-1" || len(decoded.Events) != 3 {
					t.Fatalf("invalid full JSON report: %s, %v", content, err)
				}
			case "csv":
				rows, err := csv.NewReader(bytes.NewReader(content)).ReadAll()
				if err != nil {
					t.Fatal(err)
				}
				if len(rows) != 5 || rows[1][0] != "event" || rows[1][5] != "actor-a" || rows[4][0] != "finding" {
					t.Fatalf("unexpected CSV: %v", rows)
				}
			case "pdf":
				if !bytes.HasPrefix(content, []byte("%PDF-")) {
					t.Fatal("invalid PDF header")
				}
			}
		})
	}
	if !bytes.Equal(run.ResponseBytes, original) {
		t.Fatal("renderer modified original executor bytes")
	}
}

func TestPersistedAuditReportRejectsInvalidSnapshot(t *testing.T) {
	for _, name := range []string{"request hash", "response hash", "association", "timestamp", "missing evidence", "request JSON", "response JSON"} {
		t.Run(name, func(t *testing.T) {
			run := reportSnapshotFixture(t)
			switch name {
			case "request hash":
				run.RequestHash = "invalid"
			case "response hash":
				run.ResponseHash = "invalid"
			case "association":
				run.RequestBytes = bytes.ReplaceAll(run.RequestBytes, []byte("audit-1"), []byte("audit-other"))
			case "timestamp":
				run.RequestBytes = bytes.ReplaceAll(run.RequestBytes, []byte("2026-10-15T08:00:00Z"), []byte("invalid"))
			case "missing evidence":
				run.RequestBytes = bytes.ReplaceAll(run.RequestBytes, []byte(`"evidence":{"contracts":`), []byte(`"evidence":{"other":`))
			case "request JSON":
				run.RequestBytes = []byte("{")
			case "response JSON":
				run.ResponseBytes = []byte("{")
			}
			if name != "request hash" {
				run.RequestHash = hashBytes(run.RequestBytes)
			}
			if name != "response hash" {
				run.ResponseHash = hashBytes(run.ResponseBytes)
			}
			if _, _, err := renderPersistedExecutorReport(run, "json", "actor", time.Now()); err == nil {
				t.Fatal("invalid snapshot accepted")
			}
		})
	}
}

func TestAuditReportPDFPaginatesEveryEvent(t *testing.T) {
	report := auditReport{ReportID: "many-events", Scope: "contracts"}
	for i := 0; i < 180; i++ {
		report.Events = append(report.Events, auditReportEvent{Timestamp: "2026-10-15T08:00:00Z", Actor: "actor", EventType: fmt.Sprintf("LIFECYCLE_%03d", i), DID: "did:example:contract"})
	}
	pdf := string(renderAuditReportPDF(report))
	pages := strings.Count(pdf, "/Type /Page ")
	if pages < 4 || !strings.Contains(pdf, fmt.Sprintf("/Count %d", pages)) {
		t.Fatalf("missing PDF pages: %d", pages)
	}
	if strings.Count(pdf, "FACIS Audit Report - Page") != pages {
		t.Fatal("missing repeated page header")
	}
	for _, event := range report.Events {
		if strings.Count(pdf, event.EventType) != 1 {
			t.Fatalf("event omitted or duplicated: %s", event.EventType)
		}
	}
}

func TestWrapPDFLinePreservesRunes(t *testing.T) {
	input := strings.Repeat("ä", 100)
	lines := wrapPDFLine(input)
	if len(lines) != 2 || strings.Join(lines, "") != input {
		t.Fatalf("invalid wrapping: %q", lines)
	}
	for _, line := range lines {
		if !utf8.ValidString(line) {
			t.Fatal("split UTF-8 rune")
		}
	}
}

func TestGlobalAuditReportDoesNotInventFindingResource(t *testing.T) {
	run := reportSnapshotFixture(t)
	resource := []byte(`"resource":{"did":"did:example:contract"},`)
	run.RequestBytes = bytes.ReplaceAll(run.RequestBytes, resource, nil)
	run.ResponseBytes = bytes.ReplaceAll(run.ResponseBytes, resource, nil)
	run.RequestHash, run.ResponseHash = hashBytes(run.RequestBytes), hashBytes(run.ResponseBytes)
	_, report, err := renderPersistedExecutorReport(run, "json", "actor", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if report.Findings[0].DID != "" || report.Resources[0].FindingCount != 0 || report.Summary.TotalChecks != 1 {
		t.Fatalf("global finding assigned to evidence reference as resource: %+v", report)
	}
}

func TestAuditReportDoesNotCountADeferredFindingAsPassed(t *testing.T) {
	report := auditReport{
		ReportID: "pac-report-deferred",
		Scope:    "contracts",
		Findings: []auditReportFinding{
			{RuleID: "FACIS-SATISFIED", Severity: validation.SeveritySatisfied, Message: "ODRL policy satisfied"},
			{RuleID: "FACIS-DEFERRED", Severity: validation.SeverityDeferred, Message: "constraint on spatial is enforced at use-time"},
		},
	}
	report.Summary = summarizeAuditReport(report)

	if report.Summary.Passed != 1 {
		t.Fatalf("passed = %d, want 1", report.Summary.Passed)
	}
	if report.Summary.NotEvaluated != 1 {
		t.Fatalf("notEvaluated = %d, want 1", report.Summary.NotEvaluated)
	}
	if report.Summary.NeedsReview != 0 || report.Summary.Failed != 0 || report.Summary.Warnings != 0 {
		t.Fatalf("a deferred finding is neither a failure, a warning nor a review task: %+v", report.Summary)
	}

	csvBytes, err := renderAuditReportCSV(report)
	if err != nil {
		t.Fatalf("render csv: %v", err)
	}
	csvText := string(csvBytes)
	if !strings.Contains(csvText, "finding,,,,,,not_evaluated,FACIS-DEFERRED") {
		t.Fatalf("csv does not report the deferred finding as not_evaluated: %s", csvText)
	}

	pdfText := string(renderAuditReportPDF(report))
	if !strings.Contains(pdfText, "1 passed") || !strings.Contains(pdfText, "1 not evaluated") {
		t.Fatalf("pdf summary does not separate passed from not evaluated: %s", pdfText)
	}
}

func TestNormalizedSeverityBuckets(t *testing.T) {
	for severity, want := range map[string]string{
		validation.SeveritySatisfied: "passed",
		validation.SeverityDeferred:  "not_evaluated",
		validation.SeverityError:     "failed",
		validation.SeverityWarning:   "warning",
		validation.SeverityInfo:      "review",
		"":                           "review",
	} {
		if got := normalizedSeverity(severity); got != want {
			t.Fatalf("normalizedSeverity(%q) = %q, want %q", severity, got, want)
		}
	}
}
