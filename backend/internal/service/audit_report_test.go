package service

import (
	"strings"
	"testing"

	"digital-contracting-service/internal/base/validation"
)

// A constraint whose verdict belongs to the target system that executes the
// contract (ADR-33) is reported as deferred. Nothing downstream may present it
// as a check that passed — not the summary, not the CSV result column, and not
// the exported PDF, which prints the counts an auditor reads.
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
