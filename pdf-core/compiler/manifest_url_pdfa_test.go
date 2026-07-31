package compiler

import (
	"bytes"
	"context"
	"testing"
)

// TestManifestURLXMPDeclaresPDFAExtensionSchema guards ISO 19005-3 clause
// 6.6.2.3.1: the dcterms:provenance property a remote-manifest URL adds (DCS-OR-
// C2PA-008) uses the non-predefined dcterms schema, so it must be declared as a
// PDF/A extension schema (clause 6.6.2.3.2) or veraPDF PDF/A-3a validation fails.
func TestManifestURLXMPDeclaresPDFAExtensionSchema(t *testing.T) {
	ctx := WithSigner(context.Background(), NewCapturingSigner())
	fresh, err := CompilePDF(ctx, []byte(filledContractPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}
	upd, err := UpdatePDFWithOptions(ctx, fresh, []byte(filledContractPayload+" "), nil, "https://dcs.example/m/abc", CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(upd, []byte("dcterms:provenance")) {
		t.Fatal("remote manifest URL did not add dcterms:provenance")
	}
	if !bytes.Contains(upd, []byte("pdfaExtension:schemas")) {
		t.Fatal("dcterms not declared as a PDF/A extension schema (clause 6.6.2.3.2)")
	}
	if !bytes.Contains(upd, []byte("http://purl.org/dc/terms/")) {
		t.Fatal("missing dcterms namespaceURI in the extension schema")
	}
}
