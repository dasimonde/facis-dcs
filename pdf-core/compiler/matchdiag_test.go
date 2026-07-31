package compiler

import (
	"context"
	"strings"
	"testing"
)

// The verify diagnostic must pinpoint WHERE two renders diverge: the page number
// and a snippet of both sides around the first differing byte.
func TestMatchPageContentReportsDivergence(t *testing.T) {
	ctx := context.Background()
	a, err := CompilePDF(WithSigner(ctx, NewCapturingSigner()), []byte(richFilledContractPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}
	// A visible change (section title) so the page content genuinely differs.
	other := strings.Replace(richFilledContractPayload, "1. Payment", "1. Charges", 1)
	b, err := CompilePDF(WithSigner(ctx, NewCapturingSigner()), []byte(other), CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}
	err = MatchPageContent(a, b)
	if err == nil {
		t.Fatal("expected a page-content mismatch")
	}
	msg := err.Error()
	for _, want := range []string{"page 1", "at byte", "candidate=", "reference="} {
		if !strings.Contains(msg, want) {
			t.Fatalf("diagnostic missing %q in error: %s", want, msg)
		}
	}
}
