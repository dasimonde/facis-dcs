package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"digital-contracting-service/internal/processauditandcompliance/auditexecutor"
)

type auditReport struct {
	ReportID    string                `json:"reportId"`
	Scope       string                `json:"scope"`
	GeneratedAt string                `json:"generatedAt"`
	GeneratedBy string                `json:"generatedBy"`
	Format      string                `json:"format"`
	DID         string                `json:"did,omitempty"`
	ContentHash string                `json:"contentHash,omitempty"`
	Summary     auditReportSummary    `json:"summary"`
	Resources   []auditReportResource `json:"resources"`
	Events      []auditReportEvent    `json:"events"`
	Findings    []auditReportFinding  `json:"findings"`
}

type auditReportSummary struct {
	TotalEvents int `json:"totalEvents"`
	TotalChecks int `json:"totalChecks"`
	Passed      int `json:"passed"`
	Failed      int `json:"failed"`
	Warnings    int `json:"warnings"`
	NeedsReview int `json:"needsReview"`
}

type auditReportResource struct {
	DID          string `json:"did"`
	Component    string `json:"component"`
	EventCount   int    `json:"eventCount"`
	FindingCount int    `json:"findingCount"`
}

type auditReportEvent struct {
	Timestamp string         `json:"timestamp"`
	Actor     string         `json:"actor,omitempty"`
	Component string         `json:"component"`
	EventType string         `json:"eventType"`
	DID       string         `json:"did,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

type auditReportFinding struct {
	Timestamp      string `json:"timestamp"`
	Component      string `json:"component"`
	EventType      string `json:"eventType"`
	DID            string `json:"did,omitempty"`
	RuleID         string `json:"ruleId,omitempty"`
	Title          string `json:"title,omitempty"`
	Severity       string `json:"severity,omitempty"`
	Message        string `json:"message,omitempty"`
	Requirement    string `json:"requirement,omitempty"`
	ActualValue    any    `json:"actualValue,omitempty"`
	ExpectedValue  any    `json:"expectedValue,omitempty"`
	ExpectedValues []any  `json:"expectedValues,omitempty"`
	Operator       string `json:"operator,omitempty"`
	Path           string `json:"path,omitempty"`
	FieldIri       string `json:"fieldIri,omitempty"`
	OntologyTerm   string `json:"ontologyTerm,omitempty"`
	Actor          string `json:"actor,omitempty"`
}

func buildExecutorAuditReport(response auditexecutor.Response, generatedBy string, generatedAt time.Time) auditReport {
	did := ""
	if response.Resource != nil {
		did = response.Resource.DID
	}
	report := auditReport{
		ReportID: response.AuditID, Scope: response.Scope, DID: did,
		GeneratedAt: generatedAt.UTC().Format(time.RFC3339), GeneratedBy: generatedBy,
		Format: "json", Resources: []auditReportResource{}, Events: []auditReportEvent{},
		Findings: []auditReportFinding{},
	}
	if did != "" {
		report.Resources = append(report.Resources, auditReportResource{
			DID: did, Component: response.Scope, FindingCount: len(response.Findings),
		})
	}
	for _, finding := range response.Findings {
		severity := finding.Severity
		if severity == "" {
			severity = finding.Result
		}
		report.Findings = append(report.Findings, auditReportFinding{
			Timestamp: response.ExecutedAt, Component: response.Scope,
			EventType: "PAC_AUDIT_EXECUTOR_FINDING", DID: did,
			RuleID: finding.RuleID, Severity: severity, Message: finding.Reason,
		})
	}
	report.Summary = summarizeAuditReport(report)
	return report
}

// renderPersistedExecutorReport renders exclusively from the validated
// response stored in pac_audit_runs. For JSON the exact stored bytes are the
// report bytes; CSV and PDF are deterministic projections of the same run.
func renderPersistedExecutorReport(raw []byte, format, generatedBy string, generatedAt time.Time) ([]byte, auditReport, error) {
	var response auditexecutor.Response
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, auditReport{}, fmt.Errorf("decode persisted PAC audit run: %w", err)
	}
	report := buildExecutorAuditReport(response, generatedBy, generatedAt)
	report.Format = format
	switch format {
	case "json":
		return raw, report, nil
	case "csv":
		content, err := renderAuditReportCSV(report)
		return content, report, err
	case "pdf":
		return renderAuditReportPDF(report), report, nil
	default:
		return nil, auditReport{}, fmt.Errorf("unsupported audit report format %q", format)
	}
}

func summarizeAuditReport(report auditReport) auditReportSummary {
	summary := auditReportSummary{
		TotalEvents: len(report.Events),
		TotalChecks: len(report.Findings),
	}
	for _, finding := range report.Findings {
		switch normalizedSeverity(finding.Severity) {
		case "passed":
			summary.Passed++
		case "failed":
			summary.Failed++
		case "warning":
			summary.Warnings++
			summary.NeedsReview++
		default:
			summary.NeedsReview++
		}
	}
	return summary
}

func normalizedSeverity(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "info", "ok", "passed", "pass", "success", "successful", "compliant":
		return "passed"
	case "error", "critical", "blocking", "failed", "fail", "violation", "non_compliant":
		return "failed"
	case "warning", "warn":
		return "warning"
	default:
		return "review"
	}
}

func hashBytes(bytes []byte) string {
	sum := sha256.Sum256(bytes)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func renderAuditReportCSV(report auditReport) ([]byte, error) {
	var buffer bytes.Buffer
	writer := csv.NewWriter(&buffer)
	rows := [][]string{
		{"section", "timestamp", "did", "component", "eventType", "actor", "result", "ruleId", "message", "requirement", "actualValue", "expectedValue", "expectedValues", "path"},
	}
	for _, event := range report.Events {
		rows = append(rows, []string{"event", event.Timestamp, event.DID, event.Component, event.EventType, event.Actor, "", "", "", "", "", "", "", ""})
	}
	for _, finding := range report.Findings {
		rows = append(rows, []string{
			"finding",
			finding.Timestamp,
			finding.DID,
			finding.Component,
			finding.EventType,
			finding.Actor,
			normalizedSeverity(finding.Severity),
			finding.RuleID,
			finding.Message,
			finding.Requirement,
			formatReportValue(finding.ActualValue),
			formatReportValue(finding.ExpectedValue),
			formatReportValue(finding.ExpectedValues),
			firstNonEmpty(finding.FieldIri, finding.Path),
		})
	}
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func renderAuditReportPDF(report auditReport) []byte {
	lines := []string{
		"FACIS Audit Report",
		"Report ID: " + report.ReportID,
		"Scope: " + report.Scope,
		"Generated at: " + report.GeneratedAt,
		"Generated by: " + report.GeneratedBy,
		fmt.Sprintf("Summary: %d events, %d checks, %d passed, %d failed, %d warnings, %d needs review", report.Summary.TotalEvents, report.Summary.TotalChecks, report.Summary.Passed, report.Summary.Failed, report.Summary.Warnings, report.Summary.NeedsReview),
		"",
		"Findings",
	}
	for _, finding := range report.Findings {
		lines = append(lines, wrapPDFLine(fmt.Sprintf("%s [%s] %s %s", finding.Timestamp, finding.Severity, finding.RuleID, finding.Message))...)
		if finding.Requirement != "" {
			lines = append(lines, wrapPDFLine("Requirement: "+finding.Requirement)...)
		}
	}
	if len(report.Findings) == 0 {
		lines = append(lines, "No compliance findings.")
	}
	lines = append(lines, "", "Lifecycle Events")
	for _, event := range report.Events {
		lines = append(lines, wrapPDFLine(fmt.Sprintf("%s actor=%s %s %s", event.Timestamp, event.Actor, event.EventType, event.DID))...)
	}
	if len(report.Events) == 0 {
		lines = append(lines, "No lifecycle events.")
	}
	return simplePDF(lines)
}

func simplePDF(lines []string) []byte {
	var text bytes.Buffer
	text.WriteString("BT\n/F1 10 Tf\n50 780 Td\n14 TL\n")
	for _, line := range lines {
		text.WriteString("(")
		text.WriteString(escapePDFText(line))
		text.WriteString(") Tj\nT*\n")
	}
	text.WriteString("ET\n")
	stream := text.String()
	objects := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>",
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream),
	}
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n")
	offsets := make([]int, 0, len(objects)+1)
	offsets = append(offsets, 0)
	for i, obj := range objects {
		offsets = append(offsets, out.Len())
		out.WriteString(strconv.Itoa(i + 1))
		out.WriteString(" 0 obj\n")
		out.WriteString(obj)
		out.WriteString("\nendobj\n")
	}
	xref := out.Len()
	out.WriteString("xref\n0 ")
	out.WriteString(strconv.Itoa(len(objects) + 1))
	out.WriteString("\n0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", offset)
	}
	out.WriteString("trailer\n<< /Size ")
	out.WriteString(strconv.Itoa(len(objects) + 1))
	out.WriteString(" /Root 1 0 R >>\nstartxref\n")
	out.WriteString(strconv.Itoa(xref))
	out.WriteString("\n%%EOF\n")
	return out.Bytes()
}

func wrapPDFLine(line string) []string {
	const max = 95
	if len(line) <= max {
		return []string{line}
	}
	var lines []string
	for len(line) > max {
		lines = append(lines, line[:max])
		line = line[max:]
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines
}

func escapePDFText(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, "(", `\(`, ")", `\)`, "\r", " ", "\n", " ")
	return replacer.Replace(value)
}

func formatReportValue(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	bytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(bytes)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
