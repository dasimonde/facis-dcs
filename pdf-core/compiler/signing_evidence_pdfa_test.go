package compiler

import (
	"bytes"
	"context"
	"regexp"
	"testing"
)

// TestSigningEvidenceListedInCatalogAF guards ISO 19005-3 clause 6.8 for the
// signing-evidence attachment: the filespec carries /AFRelationship, so it must
// also appear in the (superseded) document catalog's /AF array. Without it
// veraPDF PDF/A-3a rejects the signed contract with "the file specification
// dictionary for an embedded file is not associated with the PDF document or
// any of its parts" — which is how every signed artifact failed conformance,
// unnoticed because only the two-instance vertical runs veraPDF on a signed PDF.
func TestSigningEvidenceListedInCatalogAF(t *testing.T) {
	ctx := WithSigner(context.Background(), NewCapturingSigner())
	fresh, err := CompilePDF(ctx, []byte(filledContractPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}

	embedded, err := EmbedSigningEvidence(fresh, []byte(`{"type":["ContractSigningSummaryCredential"]}`))
	if err != nil {
		t.Fatal(err)
	}

	start, end, ok := lastObjectBody(embedded, 1)
	if !ok {
		t.Fatal("no superseded catalog (obj 1) after embedding signing evidence")
	}
	cat := embedded[start:end]

	m := regexp.MustCompile(`/AF \[([^\]]*)\]`).FindSubmatch(cat)
	if m == nil {
		t.Fatal("no /AF array in the catalog after embedding signing evidence")
	}
	// The base document associates the C2PA manifest and the JSON-LD payload;
	// the evidence attachment must join them rather than dangle unassociated.
	refs := regexp.MustCompile(`\d+ 0 R`).FindAll(m[1], -1)
	if len(refs) != 3 {
		t.Fatalf("catalog /AF lists %d associated files, want 3 (C2PA + JSON-LD + signing evidence)", len(refs))
	}
	if !bytes.Contains(cat, []byte(signingEvidenceFileName)) {
		t.Fatalf("%s not added to the catalog /EmbeddedFiles name tree", signingEvidenceFileName)
	}
}

// TestSigningEvidenceStillExtractableAfterAssociation keeps the association from
// breaking the reader: the evidence must remain retrievable from the bytes a
// PAdES signature covers.
func TestSigningEvidenceStillExtractableAfterAssociation(t *testing.T) {
	ctx := WithSigner(context.Background(), NewCapturingSigner())
	fresh, err := CompilePDF(ctx, []byte(filledContractPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}
	evidence := []byte(`{"type":["ContractSigningSummaryCredential"],"id":"urn:uuid:1"}`)

	embedded, err := EmbedSigningEvidence(fresh, evidence)
	if err != nil {
		t.Fatal(err)
	}
	got, present, err := ExtractSigningEvidence(embedded)
	if err != nil {
		t.Fatal(err)
	}
	if !present {
		t.Fatal("signing evidence not present after embedding")
	}
	if !bytes.Equal(got, evidence) {
		t.Fatalf("extracted evidence differs from what was embedded:\n got %s\nwant %s", got, evidence)
	}
	if !bytes.HasPrefix(embedded, fresh) {
		t.Fatal("original bytes are no longer a prefix; a later PAdES ByteRange would not cover the evidence")
	}
}
