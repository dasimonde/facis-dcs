package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	pdfgeneration "digital-contracting-service/gen/pdf_generation"
)

// TestErrorFormatterPreservesBundleExportFindings verifies that a
// *pdfgeneration.BundleExportRefusedError keeps its findings array (and gets a
// 422 status) instead of being collapsed into the generic Goa error hull —
// clients read the findings out of the refusal response body.
func TestErrorFormatterPreservesBundleExportFindings(t *testing.T) {
	findings := []string{
		"contract abc: no exported PDF (export the contract PDF before bundling)",
		"contract abc: exported PDF carries no C2PA manifest store",
	}
	err := &pdfgeneration.BundleExportRefusedError{
		Name:     "refused",
		Message:  "contract bundle export refused by structural-integrity pre-flight",
		Findings: findings,
	}

	statuser := errorFormatter(context.Background(), err)
	if got := statuser.StatusCode(); got != http.StatusUnprocessableEntity {
		t.Fatalf("StatusCode() = %d, want %d", got, http.StatusUnprocessableEntity)
	}

	body, marshalErr := json.Marshal(statuser)
	if marshalErr != nil {
		t.Fatalf("marshal refusal body: %v", marshalErr)
	}

	var decoded struct {
		Name     string   `json:"name"`
		Message  string   `json:"message"`
		Findings []string `json:"findings"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal refusal body: %v", err)
	}

	if decoded.Name != "refused" {
		t.Errorf("name = %q, want %q", decoded.Name, "refused")
	}
	if len(decoded.Findings) != len(findings) {
		t.Fatalf("findings = %v, want %v", decoded.Findings, findings)
	}
	for i, f := range findings {
		if decoded.Findings[i] != f {
			t.Errorf("findings[%d] = %q, want %q", i, decoded.Findings[i], f)
		}
	}
}

// TestErrorFormatterRefusalNilFindings ensures a refusal with no findings still
// serializes a present (empty, non-null) findings array, matching the generated
// response body shape.
func TestErrorFormatterRefusalNilFindings(t *testing.T) {
	err := &pdfgeneration.BundleExportRefusedError{Name: "refused", Message: "refused"}

	body, marshalErr := json.Marshal(errorFormatter(context.Background(), err))
	if marshalErr != nil {
		t.Fatalf("marshal refusal body: %v", marshalErr)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("unmarshal refusal body: %v", err)
	}
	raw, ok := decoded["findings"]
	if !ok {
		t.Fatalf("findings key missing from refusal body: %s", body)
	}
	if string(raw) != "[]" {
		t.Errorf("findings = %s, want []", raw)
	}
}
