package compiler

import (
	"bytes"
	"testing"
)

const minimalPDFForEvidence = "%PDF-1.7\n" +
	"1 0 obj\n<< /Type /Catalog >>\nendobj\n" +
	"xref\n0 2\n0000000000 65535 f \n0000000009 00000 n \n" +
	"trailer\n<< /Size 2 /Root 1 0 R >>\nstartxref\n40\n%%EOF\n"

func TestEmbedAndExtractSigningEvidenceRoundTrip(t *testing.T) {
	base := []byte(minimalPDFForEvidence)
	evidence := []byte(`{"type":["VerifiableCredential","ContractSigningSummaryCredential"],"pid":"eyJ.aaa~bbb~ccc"}`)

	embedded, err := EmbedSigningEvidence(base, evidence)
	if err != nil {
		t.Fatalf("EmbedSigningEvidence: %v", err)
	}
	if !bytes.HasPrefix(embedded, base) {
		t.Fatal("embedded PDF must preserve the original bytes as a prefix so a later signature's ByteRange covers the evidence")
	}
	if !bytes.Contains(embedded, evidence) {
		t.Fatal("verbatim evidence bytes must appear in the embedded PDF")
	}

	got, found, err := ExtractSigningEvidence(embedded)
	if err != nil {
		t.Fatalf("ExtractSigningEvidence: %v", err)
	}
	if !found {
		t.Fatal("expected embedded evidence to be found")
	}
	if !bytes.Equal(got, evidence) {
		t.Fatalf("extracted evidence mismatch:\n got %q\nwant %q", got, evidence)
	}
}

func TestEmbedSigningEvidenceEmptyIsNoop(t *testing.T) {
	base := []byte(minimalPDFForEvidence)
	out, err := EmbedSigningEvidence(base, nil)
	if err != nil {
		t.Fatalf("EmbedSigningEvidence(nil): %v", err)
	}
	if !bytes.Equal(out, base) {
		t.Fatal("empty evidence must return the PDF unchanged")
	}
	if _, found, _ := ExtractSigningEvidence(base); found {
		t.Fatal("a PDF without evidence must report not found")
	}
}

