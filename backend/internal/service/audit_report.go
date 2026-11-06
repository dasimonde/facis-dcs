package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"digital-contracting-service/internal/base/artifactstore"
	"digital-contracting-service/internal/base/validation"
	"digital-contracting-service/internal/processauditandcompliance/auditexecutor"
)

type persistedAuditRun struct {
	Scopes                      []artifactstore.Scope
	RequestBytes, ResponseBytes []byte
	RequestHash, ResponseHash   string
}

type persistedAuditEvidenceResource struct {
	DID        string                        `json:"did"`
	Component  string                        `json:"component"`
	AuditTrail []persistedAuditEvidenceEntry `json:"audit_trail"`
}

type persistedAuditEvidenceEntry struct {
	ID        int64          `json:"id"`
	Component string         `json:"component"`
	EventType string         `json:"event_type"`
	EventData map[string]any `json:"event_data"`
	DID       string         `json:"did"`
	CreatedAt string         `json:"created_at"`
	Kind      string         `json:"kind"`
	Reason    string         `json:"reason"`
}

type auditReport struct {
	ContractVersion string                 `json:"contractVersion"`
	CorrelationID   string                 `json:"correlationId"`
	Executor        auditexecutor.Executor `json:"executor"`
	ExecutedAt      string                 `json:"executedAt"`
	Receipt         json.RawMessage        `json:"receipt,omitempty"`
	RequestHash     string                 `json:"requestHash"`
	ResponseHash    string                 `json:"responseHash"`
	ReportID        string                 `json:"reportId"`
	Scope           string                 `json:"scope"`
	GeneratedAt     string                 `json:"generatedAt"`
	GeneratedBy     string                 `json:"generatedBy"`
	Format          string                 `json:"format"`
	DID             string                 `json:"did,omitempty"`
	Summary         auditReportSummary     `json:"summary"`
	Resources       []auditReportResource  `json:"resources"`
	Events          []auditReportEvent     `json:"events"`
	Findings        []auditReportFinding   `json:"findings"`
}

type auditReportSummary struct {
	TotalEvents int `json:"totalEvents"`
	TotalChecks int `json:"totalChecks"`
	Passed      int `json:"passed"`
	Failed      int `json:"failed"`
	Warnings    int `json:"warnings"`
	NeedsReview int `json:"needsReview"`
	// NotEvaluated counts the checks nobody in the DCS reached a verdict on
	// (ADR-33). They are not passed, not failed and not a review task.
	NotEvaluated int `json:"notEvaluated"`
}

type auditReportResource struct {
	DID          string `json:"did"`
	Component    string `json:"component"`
	EventCount   int    `json:"eventCount"`
	FindingCount int    `json:"findingCount"`
}

type auditReportEvent struct {
	ID        int64          `json:"id"`
	Timestamp string         `json:"timestamp"`
	Actor     string         `json:"actor,omitempty"`
	Component string         `json:"component"`
	EventType string         `json:"eventType"`
	DID       string         `json:"did,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

type auditReportFinding struct {
	Result         string   `json:"result"`
	EvidenceRefs   []string `json:"evidenceRefs,omitempty"`
	Timestamp      string   `json:"timestamp"`
	Component      string   `json:"component"`
	EventType      string   `json:"eventType"`
	DID            string   `json:"did,omitempty"`
	RuleID         string   `json:"ruleId,omitempty"`
	Title          string   `json:"title,omitempty"`
	Severity       string   `json:"severity,omitempty"`
	Message        string   `json:"message,omitempty"`
	Requirement    string   `json:"requirement,omitempty"`
	ActualValue    any      `json:"actualValue,omitempty"`
	ExpectedValue  any      `json:"expectedValue,omitempty"`
	ExpectedValues []any    `json:"expectedValues,omitempty"`
	Operator       string   `json:"operator,omitempty"`
	Path           string   `json:"path,omitempty"`
	FieldIri       string   `json:"fieldIri,omitempty"`
	OntologyTerm   string   `json:"ontologyTerm,omitempty"`
	Actor          string   `json:"actor,omitempty"`
}

func buildExecutorAuditReport(response auditexecutor.Response, generatedBy string, generatedAt time.Time) auditReport {
	did := ""
	if response.Resource != nil {
		did = response.Resource.DID
	}
	report := auditReport{
		ReportID: response.AuditID, Scope: response.Scope, DID: did,
		ContractVersion: response.ContractVersion, CorrelationID: response.CorrelationID,
		Executor: response.Executor, ExecutedAt: response.ExecutedAt, Receipt: response.Receipt,
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
			Result: finding.Result, EvidenceRefs: finding.EvidenceRefs,
		})
	}
	report.Summary = summarizeAuditReport(report)
	return report
}

// Every format uses the evidence frozen when the executor ran, never live audit rows.
func renderPersistedExecutorReport(run persistedAuditRun, format, generatedBy string, generatedAt time.Time) ([]byte, auditReport, error) {
	if hashBytes(run.RequestBytes) != run.RequestHash || hashBytes(run.ResponseBytes) != run.ResponseHash {
		return nil, auditReport{}, fmt.Errorf("persisted PAC audit snapshot hash mismatch")
	}
	var request struct {
		auditexecutor.Request
		Evidence map[string]json.RawMessage `json:"evidence"`
	}
	var response auditexecutor.Response
	if err := json.Unmarshal(run.RequestBytes, &request); err != nil {
		return nil, auditReport{}, fmt.Errorf("decode persisted PAC audit request: %w", err)
	}
	if err := json.Unmarshal(run.ResponseBytes, &response); err != nil {
		return nil, auditReport{}, fmt.Errorf("decode persisted PAC audit response: %w", err)
	}
	sameResource := request.Resource == nil && response.Resource == nil ||
		request.Resource != nil && response.Resource != nil && request.Resource.DID == response.Resource.DID
	if request.AuditID == "" || request.AuditID != response.AuditID || request.CorrelationID != response.CorrelationID || request.Scope != response.Scope || request.ContractVersion != response.ContractVersion || !sameResource {
		return nil, auditReport{}, fmt.Errorf("persisted PAC audit request and response do not match")
	}
	var resources []persistedAuditEvidenceResource
	evidence, ok := request.Evidence[request.Scope]
	if !ok || string(evidence) == "null" {
		return nil, auditReport{}, fmt.Errorf("persisted PAC audit request lacks scope evidence")
	}
	decoder := json.NewDecoder(bytes.NewReader(evidence))
	decoder.UseNumber()
	if err := decoder.Decode(&resources); err != nil {
		return nil, auditReport{}, fmt.Errorf("decode persisted PAC audit evidence: %w", err)
	}
	report := buildExecutorAuditReport(response, generatedBy, generatedAt)
	report.Format, report.RequestHash, report.ResponseHash = format, run.RequestHash, run.ResponseHash
	report.Resources = []auditReportResource{}
	resourceIndexes := map[string]int{}
	for _, resource := range resources {
		index, exists := resourceIndexes[resource.DID]
		if !exists {
			index = len(report.Resources)
			resourceIndexes[resource.DID] = index
			report.Resources = append(report.Resources, auditReportResource{DID: resource.DID, Component: resource.Component})
		}
		for _, entry := range resource.AuditTrail {
			if entry.Kind != "TIMELINE" {
				continue
			}
			if _, err := time.Parse(time.RFC3339Nano, entry.CreatedAt); err != nil {
				return nil, auditReport{}, fmt.Errorf("invalid persisted lifecycle timestamp: %w", err)
			}
			actorData, err := json.Marshal(entry.EventData)
			if err != nil {
				return nil, auditReport{}, err
			}
			report.Events = append(report.Events, auditReportEvent{
				ID: entry.ID, Timestamp: entry.CreatedAt, Actor: auditEntryActor(actorData),
				Component: firstNonEmpty(entry.Component, resource.Component), EventType: entry.EventType,
				DID: firstNonEmpty(entry.DID, resource.DID), Details: entry.EventData,
			})
			report.Resources[index].EventCount++
		}
	}
	for _, finding := range report.Findings {
		if index, exists := resourceIndexes[finding.DID]; exists && finding.DID != "" {
			report.Resources[index].FindingCount++
		}
	}
	sort.SliceStable(report.Events, func(i, j int) bool {
		left, _ := time.Parse(time.RFC3339Nano, report.Events[i].Timestamp)
		right, _ := time.Parse(time.RFC3339Nano, report.Events[j].Timestamp)
		if left.Equal(right) {
			return report.Events[i].ID < report.Events[j].ID
		}
		return left.Before(right)
	})
	report.Summary = summarizeAuditReport(report)
	switch format {
	case "json":
		content, err := json.Marshal(report)
		return content, report, err
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
		case "not_evaluated":
			summary.NotEvaluated++
		default:
			summary.NeedsReview++
		}
	}
	return summary
}

// normalizedSeverity buckets a finding's severity for the report. A deferred
// finding is a check nobody ran, so it has its own bucket: counting it as
// passed prints a green row as evidence (ADR-33).
func normalizedSeverity(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case validation.SeveritySatisfied, "ok", "passed", "pass", "success", "successful", "compliant":
		return "passed"
	case "error", "critical", "blocking", "failed", "fail", "violation", "non_compliant":
		return "failed"
	case "warning", "warn":
		return "warning"
	case validation.SeverityDeferred, "not_evaluated":
		return "not_evaluated"
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
		{"section", "timestamp", "did", "component", "eventType", "actor", "result", "ruleId", "message", "requirement", "actualValue", "expectedValue", "expectedValues", "path", "details", "evidenceRefs"},
	}
	for _, event := range report.Events {
		rows = append(rows, []string{"event", event.Timestamp, event.DID, event.Component, event.EventType, event.Actor, "", "", "", "", "", "", "", "", formatReportValue(event.Details), ""})
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
			"", formatReportValue(finding.EvidenceRefs),
		})
	}
	rows[0] = append(rows[0], "reportId", "correlationId", "executorId", "executorVersion", "executedAt", "receipt", "requestHash", "responseHash")
	if len(rows) == 1 {
		row := make([]string, 16)
		row[0] = "report"
		rows = append(rows, row)
	}
	for i := 1; i < len(rows); i++ {
		rows[i] = append(rows[i], report.ReportID, report.CorrelationID, report.Executor.ID, report.Executor.Version, report.ExecutedAt, string(report.Receipt), report.RequestHash, report.ResponseHash)
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
		"Executor: " + report.Executor.ID + " " + report.Executor.Version,
		"Executed at: " + report.ExecutedAt,
		"Request hash: " + report.RequestHash,
		"Response hash: " + report.ResponseHash,
		"Receipt: " + string(report.Receipt),
		"Generated at: " + report.GeneratedAt,
		"Generated by: " + report.GeneratedBy,
		fmt.Sprintf("Summary: %d events, %d checks, %d passed, %d failed, %d warnings, %d needs review, %d not evaluated", report.Summary.TotalEvents, report.Summary.TotalChecks, report.Summary.Passed, report.Summary.Failed, report.Summary.Warnings, report.Summary.NeedsReview, report.Summary.NotEvaluated),
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
	const pageHeight, topMargin, bottomMargin, lineHeight = 842, 62, 50, 14
	const linesPerPage = (pageHeight - topMargin - bottomMargin) / lineHeight
	var wrapped []string
	for _, line := range lines {
		wrapped = append(wrapped, wrapPDFLine(line)...)
	}
	const headerLines = 2
	const capacity = linesPerPage - headerLines
	pageCount := (len(wrapped) + capacity - 1) / capacity
	if pageCount == 0 {
		pageCount = 1
	}
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "", "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"}
	var kids []string
	for page := 0; page < pageCount; page++ {
		pageID := len(objects) + 1
		kids = append(kids, fmt.Sprintf("%d 0 R", pageID))
		var text bytes.Buffer
		fmt.Fprintf(&text, "BT\n/F1 10 Tf\n50 %d Td\n%d TL\n", pageHeight-topMargin, lineHeight)
		pageLines := []string{fmt.Sprintf("FACIS Audit Report - Page %d/%d", page+1, pageCount), ""}
		end := min((page+1)*capacity, len(wrapped))
		pageLines = append(pageLines, wrapped[page*capacity:end]...)
		for _, line := range pageLines {
			fmt.Fprintf(&text, "(%s) Tj\nT*\n", escapePDFText(line))
		}
		text.WriteString("ET\n")
		objects = append(objects,
			fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 %d] /Resources << /Font << /F1 3 0 R >> >> /Contents %d 0 R >>", pageHeight, pageID+1),
			fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", text.Len(), text.String()))
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), pageCount)

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
	runes := []rune(line)
	if len(runes) == 0 {
		return []string{""}
	}
	var lines []string
	for len(runes) > 0 {
		n := min(max, len(runes))
		lines = append(lines, string(runes[:n]))
		runes = runes[n:]
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
