package compiler

import (
	"bytes"
	"regexp"
	"strconv"
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
	ctx := WithSigner(testChainContext(), NewCapturingSigner())
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

// attachmentByName resolves an embedded file the way a conforming reader does —
// through the CURRENT document catalog's /EmbeddedFiles name tree, then the
// filespec's /EF — rather than by scanning for the last matching filespec.
func attachmentByName(t *testing.T, pdf []byte, name string) []byte {
	t.Helper()
	root, ok := currentRootObjID(pdf)
	if !ok {
		t.Fatal("no /Root in the trailer")
	}
	start, end, ok := lastObjectBody(pdf, root)
	if !ok {
		t.Fatalf("catalog object %d not found", root)
	}
	entry := nameTreeEntryRE(name).Find(pdf[start:end])
	if entry == nil {
		t.Fatalf("%s has no /EmbeddedFiles name-tree entry in catalog %d", name, root)
	}
	specID := objRefID(t, entry)

	specStart, specEnd, ok := lastObjectBody(pdf, specID)
	if !ok {
		t.Fatalf("filespec object %d (%s) not found", specID, name)
	}
	spec := pdf[specStart:specEnd]
	ef := regexp.MustCompile(`/EF << /F (\d+) 0 R`).Find(spec)
	if ef == nil {
		t.Fatalf("filespec %d (%s) carries no /EF reference: %s", specID, name, spec)
	}
	fileID := objRefID(t, ef)

	streamStart, streamEnd, ok := lastObjectStreamData(pdf, fileID)
	if !ok {
		t.Fatalf("embedded file object %d (%s) carries no stream", fileID, name)
	}
	return pdf[streamStart:streamEnd]
}

func objRefID(t *testing.T, ref []byte) int {
	t.Helper()
	m := regexp.MustCompile(`(\d+) 0 R`).FindSubmatch(ref)
	if m == nil {
		t.Fatalf("no object reference in %q", ref)
	}
	id, err := strconv.Atoi(string(m[1]))
	if err != nil {
		t.Fatalf("invalid object reference %q: %v", ref, err)
	}
	return id
}

// TestEverySigningEvidenceAttachmentIsAssociatedAndNamed guards ISO 19005-3
// clause 6.8 for the multi-party case: a contract signed by both DCS instances
// carries one evidence attachment per party, and EACH must be listed in the
// catalog /AF array and resolvable under its own name in the /EmbeddedFiles
// name tree. Re-using one filename would make the name tree resolve a single
// entry and hide every earlier party's evidence from Acrobat, pypdf and any
// wallet that reads attachments by name.
func TestEverySigningEvidenceAttachmentIsAssociatedAndNamed(t *testing.T) {
	ctx := WithSigner(testChainContext(), NewCapturingSigner())
	fresh, err := CompilePDF(ctx, []byte(filledContractPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}
	first := []byte(`{"party":"a","type":["ContractSigningSummaryCredential"]}`)
	second := []byte(`{"party":"b","type":["ContractSigningSummaryCredential"]}`)

	embedded, err := EmbedSigningEvidence(fresh, first)
	if err != nil {
		t.Fatal(err)
	}
	embedded, err = EmbedSigningEvidence(embedded, second)
	if err != nil {
		t.Fatal(err)
	}

	root, ok := currentRootObjID(embedded)
	if !ok {
		t.Fatal("no /Root in the trailer")
	}
	start, end, ok := lastObjectBody(embedded, root)
	if !ok {
		t.Fatalf("no superseded catalog (obj %d) after embedding signing evidence", root)
	}
	cat := embedded[start:end]

	m := regexp.MustCompile(`/AF \[([^\]]*)\]`).FindSubmatch(cat)
	if m == nil {
		t.Fatal("no /AF array in the catalog after embedding signing evidence")
	}
	refs := regexp.MustCompile(`\d+ 0 R`).FindAll(m[1], -1)
	if len(refs) != 4 {
		t.Fatalf("catalog /AF lists %d associated files, want 4 (C2PA + JSON-LD + two signing evidence attachments)", len(refs))
	}

	if got := attachmentByName(t, embedded, "signing-evidence.json"); !bytes.Equal(got, first) {
		t.Errorf("signing-evidence.json resolves to %s, want %s", got, first)
	}
	if got := attachmentByName(t, embedded, "signing-evidence-2.json"); !bytes.Equal(got, second) {
		t.Errorf("signing-evidence-2.json resolves to %s, want %s", got, second)
	}
}

// TestSigningEvidenceStillExtractableAfterAssociation keeps the association from
// breaking the reader: the evidence must remain retrievable from the bytes a
// PAdES signature covers.
func TestSigningEvidenceStillExtractableAfterAssociation(t *testing.T) {
	ctx := WithSigner(testChainContext(), NewCapturingSigner())
	fresh, err := CompilePDF(ctx, []byte(filledContractPayload), CanonicalCompiledAt)
	if err != nil {
		t.Fatal(err)
	}
	evidence := []byte(`{"type":["ContractSigningSummaryCredential"],"id":"urn:uuid:1"}`)

	embedded, err := EmbedSigningEvidence(fresh, evidence)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExtractSigningEvidence(embedded)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("extracted %d signing evidence attachments after embedding one", len(got))
	}
	if !bytes.Equal(got[0], evidence) {
		t.Fatalf("extracted evidence differs from what was embedded:\n got %s\nwant %s", got[0], evidence)
	}
	if !bytes.HasPrefix(embedded, fresh) {
		t.Fatal("original bytes are no longer a prefix; a later PAdES ByteRange would not cover the evidence")
	}
}
