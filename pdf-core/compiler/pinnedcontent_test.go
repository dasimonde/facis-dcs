package compiler

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// appendRevision appends a PDF incremental update defining objs (object id ->
// dictionary/stream body written after the "N 0 obj\n" header), with its own
// xref subsections and a /Prev chain back to the previous one. This is the shape
// every legitimately appended layer takes — a PAdES signature, a C2PA manifest,
// a document security store — and also the shape the attack takes, the only
// difference being WHICH objects it redefines.
func appendRevision(t *testing.T, pdf []byte, objs map[int]string) []byte {
	t.Helper()
	prevStartXref, err := previousStartXref(pdf)
	if err != nil {
		t.Fatalf("previousStartXref: %v", err)
	}
	maxObjID, err := findTrailerMaxObjID(pdf)
	if err != nil {
		t.Fatalf("findTrailerMaxObjID: %v", err)
	}
	root, ok := currentRootObjID(pdf)
	if !ok {
		t.Fatal("no /Root in the trailer")
	}
	ids := make([]int, 0, len(objs))
	for id := range objs {
		ids = append(ids, id)
		if id > maxObjID {
			maxObjID = id
		}
	}
	sort.Ints(ids)

	base := len(pdf)
	var buf bytes.Buffer
	buf.WriteString("\n")
	offsets := make(map[int]int, len(ids))
	for _, id := range ids {
		offsets[id] = base + buf.Len()
		buf.WriteString(fmt.Sprintf("%d 0 obj\n%s\nendobj\n", id, objs[id]))
	}
	xrefStart := base + buf.Len()
	buf.WriteString("xref\n")
	for _, id := range ids {
		buf.WriteString(fmt.Sprintf("%d 1\n%010d 00000 n \n", id, offsets[id]))
	}
	idEntry := ""
	if fileID := extractTrailerID(pdf); fileID != "" {
		idEntry = " /ID " + fileID
	}
	buf.WriteString(fmt.Sprintf("trailer\n<< /Size %d /Root %d 0 R /Prev %d%s >>\nstartxref\n%d\n%%%%EOF\n",
		maxObjID+1, root, prevStartXref, idEntry, xrefStart))

	return append(append([]byte(nil), pdf...), buf.Bytes()...)
}

// objectBody returns the body of the latest definition of object id.
func objectBody(t *testing.T, pdf []byte, id int) string {
	t.Helper()
	start, end, ok := lastObjectBody(pdf, id)
	if !ok {
		t.Fatalf("object %d not found", id)
	}
	return string(pdf[start:end])
}

// firstPageAndContentObjID returns the object ids of the first page and of the
// content stream it currently points at.
func firstPageAndContentObjID(t *testing.T, pdf []byte) (pageID, contentID int) {
	t.Helper()
	pageIDs, err := parseCurrentPagesKids(pdf)
	if err != nil {
		t.Fatalf("parseCurrentPagesKids: %v", err)
	}
	if len(pageIDs) == 0 {
		t.Fatal("no pages")
	}
	pageID = pageIDs[0]
	m := pdfContentsRefRE.FindStringSubmatch(objectBody(t, pdf, pageID))
	if len(m) < 2 {
		t.Fatalf("page %d has no /Contents reference", pageID)
	}
	if _, err := fmt.Sscanf(m[1], "%d", &contentID); err != nil {
		t.Fatalf("page %d /Contents ref invalid: %v", pageID, err)
	}
	return pageID, contentID
}

// appendPAdESRevision appends the incremental update a PAdES signer writes: a
// signature value dictionary, a signature widget annotation, the page object
// superseded to carry that annotation, and the catalog superseded to carry the
// /AcroForm. Every object a real signer touches is redefined here EXCEPT the
// page content stream — which is exactly why the content comparison must
// tolerate it.
func appendPAdESRevision(t *testing.T, pdf []byte, fieldName string) []byte {
	t.Helper()
	pageID, _ := firstPageAndContentObjID(t, pdf)
	maxObjID, err := findTrailerMaxObjID(pdf)
	if err != nil {
		t.Fatalf("findTrailerMaxObjID: %v", err)
	}
	sigID, annotID := maxObjID+1, maxObjID+2

	page := objectBody(t, pdf, pageID)
	annotsRE := regexp.MustCompile(`/Annots \[([^\]]*)\]`)
	if !annotsRE.MatchString(page) {
		t.Fatalf("test setup: page %d carries no /Annots array to extend", pageID)
	}
	newPage := annotsRE.ReplaceAllString(page, fmt.Sprintf("/Annots [${1} %d 0 R]", annotID))
	catalog := strings.TrimSpace(objectBody(t, pdf, 1))
	newCatalog := strings.TrimSuffix(catalog, ">>") +
		fmt.Sprintf("/AcroForm << /Fields [%d 0 R] /SigFlags 3 >> >>", annotID)

	return appendRevision(t, pdf, map[int]string{
		1:      newCatalog,
		pageID: newPage,
		sigID: "<< /Type /Sig /Filter /Adobe.PPKLite /SubFilter /ETSI.CAdES.detached " +
			fmt.Sprintf("/ByteRange [0 %d %d 4096] /Contents <308006092a864886f70d010702a0803080020101> /M (D:20260729120000Z) >>", len(pdf), len(pdf)+512),
		annotID: fmt.Sprintf("<< /Type /Annot /Subtype /Widget /FT /Sig /T (%s) /Rect [54 72 234 132] /V %d 0 R /P %d 0 R /F 132 >>",
			fieldName, sigID, pageID),
	})
}

// appendDSSRevision appends the document security store a validation-data
// (LTV / archive timestamp) layer adds: revocation material plus the catalog
// superseded to reference it.
func appendDSSRevision(t *testing.T, pdf []byte) []byte {
	t.Helper()
	maxObjID, err := findTrailerMaxObjID(pdf)
	if err != nil {
		t.Fatalf("findTrailerMaxObjID: %v", err)
	}
	crlID, dssID := maxObjID+1, maxObjID+2
	crl := "revocation-material"
	catalog := strings.TrimSpace(objectBody(t, pdf, 1))
	newCatalog := strings.TrimSuffix(catalog, ">>") + fmt.Sprintf("/DSS %d 0 R >>", dssID)

	return appendRevision(t, pdf, map[int]string{
		1:     newCatalog,
		crlID: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(crl), crl),
		dssID: fmt.Sprintf("<< /Type /DSS /CRLs [%d 0 R] >>", crlID),
	})
}

// preparedContractPDF builds the document a signing ceremony pins: a compiled
// contract with the signing-evidence attachment embedded before signing, which
// is itself an appended incremental update.
func preparedContractPDF(t *testing.T) []byte {
	t.Helper()
	base, err := CompilePDF(testSigningContext(), []byte(claimBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	prepared, err := EmbedSigningEvidence(base, []byte(`[{"type":"ContractSigningSummaryCredential"}]`))
	if err != nil {
		t.Fatalf("EmbedSigningEvidence: %v", err)
	}
	return prepared
}

// A submission is accepted only if it still RENDERS the pinned document, so
// every layer a legitimate signer, a re-anchor, an LTV store or a
// countersignature appends must pass unchanged — a false positive here would
// block every real signature. Each layer below is the real appended artifact:
// the evidence attachment via EmbedSigningEvidence, the C2PA provenance via
// ReanchorProvenance, and PAdES/DSS revisions that supersede the catalog and
// the page object the way a real signer does.
func TestMatchPageContentAcceptsEveryLegitimatelyAppendedLayer(t *testing.T) {
	prepared := preparedContractPDF(t)

	submitted := appendPAdESRevision(t, prepared, "Signature1")
	reanchored, err := ReanchorProvenance(testSigningContext(), submitted, "", time.Now())
	if err != nil {
		t.Fatalf("ReanchorProvenance: %v", err)
	}
	submitted = appendDSSRevision(t, reanchored)
	submitted = appendPAdESRevision(t, submitted, "Signature2")

	if !bytes.HasPrefix(submitted, prepared) {
		t.Fatal("test setup: the appended layers must leave the prepared bytes untouched")
	}
	if err := MatchPageContent(submitted, prepared); err != nil {
		t.Fatalf("legitimately appended layers must not diverge from the pinned content: %v", err)
	}
}

// embeddedPayloadObjID returns the object id of the embedded contract.jsonld
// stream, read from the filespec exactly as the extractor reads it.
func embeddedPayloadObjID(t *testing.T, pdf []byte) int {
	t.Helper()
	specPos := bytes.Index(pdf, []byte("/F (contract.jsonld)"))
	if specPos < 0 {
		t.Fatal("no contract.jsonld filespec")
	}
	efPos := bytes.Index(pdf[specPos:], []byte("/EF << /F "))
	if efPos < 0 {
		t.Fatal("filespec carries no /EF reference")
	}
	efPos += specPos + len("/EF << /F ")
	refEnd := bytes.Index(pdf[efPos:], []byte(" 0 R"))
	if refEnd < 0 {
		t.Fatal("malformed /EF reference")
	}
	var id int
	if _, err := fmt.Sscanf(strings.TrimSpace(string(pdf[efPos:efPos+refEnd])), "%d", &id); err != nil {
		t.Fatalf("embedded payload object id: %v", err)
	}
	return id
}

// appendPayloadRevision appends the attack: an incremental update that
// supersedes ONLY the embedded-file object, replacing the machine-readable
// contract while every page object — and therefore every rendered byte the
// signatory read — stays exactly as prepared.
func appendPayloadRevision(t *testing.T, pdf []byte, payload []byte) []byte {
	t.Helper()
	sum := sha256.Sum256(payload)
	body := fmt.Sprintf(
		"<< /Type /EmbeddedFile /Subtype /application#2Fld+json /Length %d /Params << /Size %d /CheckSum <%s> >> >>\nstream\n%s\nendstream",
		len(payload), len(payload), hex.EncodeToString(sum[:])[:32], payload,
	)
	return appendRevision(t, pdf, map[int]string{embeddedPayloadObjID(t, pdf): body})
}

// The machine-readable half of the pinned-content guarantee: the payload gate
// compares the attachment a reader resolves in the submission against the
// attachment of the pinned document, so every layer a legitimate signer, a
// re-anchor, an LTV store or a countersignature appends must leave that
// attachment byte-identical. A PAdES revision supersedes the catalog, the page
// and the signature objects; a re-anchor rewrites the embedded payload with the
// SAME bytes it extracted. None of them is a payload change.
func TestEmbeddedPayloadSurvivesEveryLegitimatelyAppendedLayer(t *testing.T) {
	prepared := preparedContractPDF(t)

	submitted := appendPAdESRevision(t, prepared, "Signature1")
	reanchored, err := ReanchorProvenance(testSigningContext(), submitted, "", time.Now())
	if err != nil {
		t.Fatalf("ReanchorProvenance: %v", err)
	}
	submitted = appendDSSRevision(t, reanchored)
	submitted = appendPAdESRevision(t, submitted, "Signature2")

	preparedPayload, err := ExtractLatestEmbeddedJSONLD(prepared)
	if err != nil {
		t.Fatalf("extract prepared payload: %v", err)
	}
	submittedPayload, err := ExtractLatestEmbeddedJSONLD(submitted)
	if err != nil {
		t.Fatalf("extract submitted payload: %v", err)
	}
	if !bytes.Equal(submittedPayload, preparedPayload) {
		t.Fatalf("legitimately appended layers must not change the embedded payload:\nprepared:  %s\nsubmitted: %s",
			preparedPayload, submittedPayload)
	}
}

// The hole the payload gate closes, and the reason it cannot be folded into the
// content gate: a revision superseding only the embedded-file object leaves the
// page content streams untouched, so MatchPageContent — the visible-content
// check — accepts it, while the machine-readable contract that drives policy
// evaluation, catalogue publication and the peer's copy has been swapped, under
// a signature whose /ByteRange covers the whole file.
func TestARevisionSupersedingTheEmbeddedPayloadEscapesTheContentMatch(t *testing.T) {
	prepared := preparedContractPDF(t)
	signed := appendPAdESRevision(t, prepared, "Signature1")

	// A machine-only term: nothing in documentStructure changes, so not one
	// rendered byte differs — only what a policy engine reads.
	tampered := strings.Replace(claimBase, `"@type": "ContractTemplate",`,
		`"@type": "ContractTemplate",
  "odrl:permission": [{"@type": "odrl:Permission", "odrl:action": "odrl:distribute"}],`, 1)
	if tampered == claimBase {
		t.Fatal("test setup: payload anchor not found")
	}
	submitted := appendPayloadRevision(t, signed, []byte(tampered))

	if !bytes.HasPrefix(submitted, prepared) {
		t.Fatal("the attack is append-only: the append-only check alone cannot see it")
	}
	if err := MatchPageContent(submitted, prepared); err != nil {
		t.Fatalf("the visible-content check is blind to this by construction, but reported: %v", err)
	}

	preparedPayload, err := ExtractLatestEmbeddedJSONLD(prepared)
	if err != nil {
		t.Fatalf("extract prepared payload: %v", err)
	}
	submittedPayload, err := ExtractLatestEmbeddedJSONLD(submitted)
	if err != nil {
		t.Fatalf("extract submitted payload: %v", err)
	}
	if bytes.Equal(submittedPayload, preparedPayload) {
		t.Fatal("the substituted attachment must be what a reader resolves, or the gate has nothing to catch")
	}
	if !bytes.Contains(submittedPayload, []byte("odrl:distribute")) {
		t.Fatalf("the resolved attachment must be the superseding one, got: %s", submittedPayload)
	}
}

// The hole this closes: a PDF incremental update may redefine ANY object, page
// content streams included. Such a submission is still a byte-prefix extension
// of the prepared document — the append-only check passes — while the visible
// contract text has changed, under a signature whose /ByteRange covers the whole
// file. Only resolving the LAST definition of each object catches it.
func TestMatchPageContentRefusesARevisionThatSupersedesPageContent(t *testing.T) {
	prepared := preparedContractPDF(t)
	signed := appendPAdESRevision(t, prepared, "Signature1")

	_, contentID := firstPageAndContentObjID(t, signed)
	content, err := extractStreamContentByObjID(signed, contentID)
	if err != nil {
		t.Fatalf("extract page content stream: %v", err)
	}
	original := string(content)
	tampered := strings.Replace(original, "(Original clause for claim verification.) Tj",
		"(Substituted clause the signatory never prepared.) Tj", 1)
	if tampered == original {
		t.Fatal("test setup: clause literal not found in the page content stream")
	}
	submitted := appendRevision(t, signed, map[int]string{
		contentID: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(tampered), tampered),
	})

	if !bytes.HasPrefix(submitted, prepared) {
		t.Fatal("the attack is append-only: the append-only check alone cannot see it")
	}
	err = MatchPageContent(submitted, prepared)
	if err == nil {
		t.Fatal("a revision superseding page content must be refused")
	}
	if !strings.Contains(err.Error(), "page 1 content does not match") {
		t.Fatalf("the refusal must name what diverged, got: %v", err)
	}
	if !strings.Contains(err.Error(), "Substituted clause") {
		t.Fatalf("the refusal must show the divergence, got: %v", err)
	}
}
