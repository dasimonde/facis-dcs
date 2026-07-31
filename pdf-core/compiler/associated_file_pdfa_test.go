package compiler

import (
	"bytes"
	"context"
	"regexp"
	"testing"
)

// TestVCAttachmentListedInCatalogAF guards ISO 19005-3 clause 6.8: a lifecycle-VC
// filespec attached by an update carries /AFRelationship, so it must also appear
// in the (superseded) document catalog's /AF array — otherwise veraPDF PDF/A-3a
// rejects it. Without the VC the catalog /AF lists two files (C2PA + JSON-LD);
// with it, three.
func TestVCAttachmentListedInCatalogAF(t *testing.T) {
	ctx := WithSigner(context.Background(), NewCapturingSigner())
	fresh, err := CompilePDF(ctx, []byte(filledContractPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}
	upd, err := UpdatePDFWithOptions(ctx, fresh, []byte(filledContractPayload+" "), []byte(`{"type":["VerifiableCredential"]}`), "", CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}
	start, end, ok := lastObjectBody(upd, 1)
	if !ok {
		t.Fatal("superseded catalog (obj 1) not found in updated PDF")
	}
	cat := upd[start:end]
	m := regexp.MustCompile(`/AF \[([^\]]*)\]`).FindSubmatch(cat)
	if m == nil {
		t.Fatal("no /AF array in the superseded catalog")
	}
	refs := regexp.MustCompile(`\d+ 0 R`).FindAll(m[1], -1)
	if len(refs) != 3 {
		t.Fatalf("catalog /AF lists %d associated files, want 3 (C2PA + JSON-LD + VC)", len(refs))
	}
	if !bytes.Contains(cat, []byte("contract-lifecycle-vc.json")) {
		t.Fatal("VC not added to the catalog /EmbeddedFiles name tree")
	}
}
