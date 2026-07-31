package compiler

import (
	"bytes"
	"testing"
	"time"
)

// sampleVC is a minimal W3C VC JSON blob used as test fixture.
const sampleVC = `{"@context":["https://www.w3.org/2018/credentials/v1"],"type":["VerifiableCredential"],"id":"urn:dcs:vc:test","issuer":"did:example:issuer","issuanceDate":"2026-01-01T00:00:00Z","credentialSubject":{"id":"did:example:contract","status":"draft"}}`

// TestUpdatePDFWithVCEmbedsAttachment verifies that UpdatePDFWithVC appends the
// VC bytes as an embedded "contract-lifecycle-vc.json" attachment in the
// incremental update section.
func TestUpdatePDFWithVCEmbedsAttachment(t *testing.T) {
	original, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	result, err := UpdatePDFWithVC(testSigningContext(), original, []byte(minimalPayloadAmended), []byte(sampleVC), time.Now())
	if err != nil {
		t.Fatalf("UpdatePDFWithVC: %v", err)
	}

	// Original bytes must be preserved as a prefix (signature invariant).
	if !bytes.HasPrefix(result, original) {
		t.Error("updated PDF must start with the original bytes unchanged")
	}

	// VC bytes must appear verbatim in the incremental section.
	if !bytes.Contains(result[len(original):], []byte(sampleVC)) {
		t.Error("incremental section must contain VC bytes verbatim")
	}

	// The attachment name must appear so ExtractVC can locate the preceding stream.
	if !bytes.Contains(result, []byte("contract-lifecycle-vc.json")) {
		t.Error("PDF must contain the string \"contract-lifecycle-vc.json\" for ExtractVC to locate it")
	}
}

// TestUpdatePDFWithVCUnchangedPayloadProceeds verifies that UpdatePDFWithVC
// succeeds even when the JSON-LD payload is identical to the current embedded
// one, because the VC attachment itself is a meaningful provenance event.
func TestUpdatePDFWithVCUnchangedPayloadProceeds(t *testing.T) {
	original, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	// Same payload — UpdatePDF would return "no changes" here.
	result, err := UpdatePDFWithVC(testSigningContext(), original, []byte(minimalPayloadBase), []byte(sampleVC), time.Now())
	if err != nil {
		t.Fatalf("UpdatePDFWithVC with unchanged payload: %v", err)
	}

	if !bytes.HasPrefix(result, original) {
		t.Error("updated PDF must start with the original bytes unchanged")
	}

	if !bytes.Contains(result, []byte("contract-lifecycle-vc.json")) {
		t.Error("PDF must contain the VC attachment name even when payload is unchanged")
	}
}

// TestExtractEmbeddedVC_ReturnsBytesWhenPresent verifies that ExtractEmbeddedVC
// returns the VC bytes that were embedded by UpdatePDFWithVC.
func TestExtractEmbeddedVC_ReturnsBytesWhenPresent(t *testing.T) {
	original, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	result, err := UpdatePDFWithVC(testSigningContext(), original, []byte(minimalPayloadAmended), []byte(sampleVC), time.Now())
	if err != nil {
		t.Fatalf("UpdatePDFWithVC: %v", err)
	}

	got, ok, err := ExtractEmbeddedVC(result)
	if err != nil {
		t.Fatalf("ExtractEmbeddedVC: %v", err)
	}
	if !ok {
		t.Fatal("expected VC to be found")
	}
	if string(got) != sampleVC {
		t.Errorf("VC bytes mismatch: got %q, want %q", got, sampleVC)
	}
}

// TestExtractEmbeddedVC_AbsentWhenNoVC verifies that ExtractEmbeddedVC returns
// ok=false for a PDF that has no VC attachment.
func TestExtractEmbeddedVC_AbsentWhenNoVC(t *testing.T) {
	original, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	_, ok, err := ExtractEmbeddedVC(original)
	if err != nil {
		t.Fatalf("ExtractEmbeddedVC on plain PDF: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false for a PDF with no VC attachment")
	}
}

// TestUpdatePDFWithVCNilVCBehavesLikeUpdatePDF verifies that when vcBytes is
// nil UpdatePDFWithVC behaves identically to UpdatePDF.
func TestUpdatePDFWithVCNilVCBehavesLikeUpdatePDF(t *testing.T) {
	original, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	now := time.Now()
	want, err := UpdatePDF(testSigningContext(), original, []byte(minimalPayloadAmended), now)
	if err != nil {
		t.Fatalf("UpdatePDF: %v", err)
	}

	got, err := UpdatePDFWithVC(testSigningContext(), original, []byte(minimalPayloadAmended), nil, now)
	if err != nil {
		t.Fatalf("UpdatePDFWithVC nil vc: %v", err)
	}

	if !bytes.Equal(want, got) {
		t.Error("UpdatePDFWithVC with nil vcBytes must produce byte-identical output to UpdatePDF")
	}
}
