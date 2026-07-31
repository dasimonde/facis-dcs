package compiler

import (
	"bytes"
	"testing"
	"time"
)

// TestExtractManifestStore_SucceedsOnCompiledPDF verifies that a freshly
// compiled PDF contains an extractable C2PA manifest store.
func TestExtractManifestStore_SucceedsOnCompiledPDF(t *testing.T) {
	pdf, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	manifest, err := ExtractManifestStore(pdf)
	if err != nil {
		t.Fatalf("ExtractManifestStore: %v", err)
	}
	if len(manifest) == 0 {
		t.Fatal("ExtractManifestStore returned empty bytes")
	}
	// JUMBF manifest store starts with a BMFF box whose type is "jumb".
	if !bytes.Contains(manifest, []byte("jumb")) {
		t.Error("manifest store bytes do not contain JUMBF box marker 'jumb'")
	}
}

// TestExtractManifestStore_SucceedsOnUpdatedPDF verifies that an
// incrementally-updated PDF also has an extractable manifest store.
func TestExtractManifestStore_SucceedsOnUpdatedPDF(t *testing.T) {
	original, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	updated, err := UpdatePDF(testSigningContext(), original, []byte(minimalPayloadAmended), time.Now())
	if err != nil {
		t.Fatalf("UpdatePDF: %v", err)
	}

	manifest, err := ExtractManifestStore(updated)
	if err != nil {
		t.Fatalf("ExtractManifestStore on updated PDF: %v", err)
	}
	if len(manifest) == 0 {
		t.Fatal("ExtractManifestStore returned empty bytes for updated PDF")
	}
}
