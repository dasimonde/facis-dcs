package compiler

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"
)

// compiledClaimPDF compiles the claim fixture and returns it with the ids of
// its first page and that page's font.
func compiledClaimPDF(t *testing.T) (pdf []byte, pageID, fontID int) {
	t.Helper()
	pdf, err := CompilePDF(testSigningContext(), []byte(claimBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	pageID, _ = firstPageAndContentObjID(t, pdf)
	m := regexp.MustCompile(`/F1 (\d+) 0 R`).FindStringSubmatch(objectBody(t, pdf, pageID))
	if m == nil {
		t.Fatal("test setup: the page names no /F1 font")
	}
	if _, err := fmt.Sscanf(m[1], "%d", &fontID); err != nil {
		t.Fatal(err)
	}
	return pdf, pageID, fontID
}

// flateStream returns a stream object body holding data compressed with
// /FlateDecode.
func flateStream(t *testing.T, data []byte) string {
	t.Helper()
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("<< /Filter /FlateDecode /Length %d >>\nstream\n%s\nendstream", buf.Len(), buf.Bytes())
}

// A FreeText annotation added to a page changes what the page shows while every
// content stream stays as it was. It must not pass, in either mode.
func TestMatchPageContentRejectsAddedAnnotation(t *testing.T) {
	original, pageID, _ := compiledClaimPDF(t)
	maxID, err := findTrailerMaxObjID(original)
	if err != nil {
		t.Fatal(err)
	}
	overlayID := maxID + 1
	page := regexp.MustCompile(`/Annots \[([^\]]*)\]`).ReplaceAllString(objectBody(t, original, pageID),
		fmt.Sprintf("/Annots [${1} %d 0 R]", overlayID))
	tampered := appendRevision(t, original, map[int]string{
		overlayID: "<< /Type /Annot /Subtype /FreeText /Rect [72 600 500 700] /Contents (Price: 1 EUR) /DA (/Helv 24 Tf 0 g) >>",
		pageID:    page,
	})

	if err := MatchPageContent(tampered, original); err == nil {
		t.Fatal("a page with an added FreeText annotation matched")
	}
	if err := MatchPageContentExact(tampered, original); err == nil {
		t.Fatal("a page with an added FreeText annotation matched exactly")
	}
}

// A font the page uses can be redefined in an appended revision, which changes
// every glyph drawn with it while the content stream stays as it was.
func TestMatchPageContentRejectsRedefinedFontResource(t *testing.T) {
	original, _, fontID := compiledClaimPDF(t)
	font := strings.Replace(objectBody(t, original, fontID), "/Widths [278 ", "/Widths [900 ", 1)
	if font == objectBody(t, original, fontID) {
		t.Fatal("test setup: font /Widths not found")
	}
	tampered := appendRevision(t, original, map[int]string{fontID: font})

	if err := MatchPageContent(tampered, original); err == nil {
		t.Fatal("a page whose font was redefined matched")
	}
}

// A tool that re-saves a stripped copy may renumber objects, compress streams,
// reorder dictionary keys and split the page content into a /Contents array.
// None of that changes what the page shows, so the copy still matches.
func TestStrippedCopyResavedWithContentsArrayVerifies(t *testing.T) {
	original, pageID, fontID := compiledClaimPDF(t)
	_, contentID := firstPageAndContentObjID(t, original)
	content, err := extractStreamContentByObjID(original, contentID)
	if err != nil {
		t.Fatal(err)
	}
	maxID, err := findTrailerMaxObjID(original)
	if err != nil {
		t.Fatal(err)
	}
	firstID, secondID, fontCopyID := maxID+1, maxID+2, maxID+3
	split := bytes.Index(content, []byte(" Tj"))
	if split < 0 {
		t.Fatal("test setup: no text operator to split the content at")
	}
	split += len(" Tj")

	// The font dictionary with its keys in reverse order, under a new number.
	fontDict, err := resolvePDFObject(original, fontID)
	if err != nil {
		t.Fatal(err)
	}
	if fontDict.stream != nil {
		t.Fatal("test setup: the font is expected to be a dictionary")
	}
	body := strings.TrimSpace(objectBody(t, original, fontID))
	entries := regexp.MustCompile(`/\w+ (?:\[[^\]]*\]|\d+ 0 R|/\w+|\d+)`).FindAllString(body, -1)
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	reordered := "<< " + strings.Join(entries, " ") + " >>"

	page := objectBody(t, original, pageID)
	page = strings.Replace(page, fmt.Sprintf("/Contents %d 0 R", contentID), fmt.Sprintf("/Contents [%d 0 R %d 0 R]", firstID, secondID), 1)
	page = strings.Replace(page, fmt.Sprintf("/F1 %d 0 R", fontID), fmt.Sprintf("/F1 %d 0 R", fontCopyID), 1)
	resaved := appendRevision(t, original, map[int]string{
		firstID:    flateStream(t, content[:split]),
		secondID:   flateStream(t, content[split:]),
		fontCopyID: reordered,
		pageID:     page,
	})

	if err := MatchPageContentExact(resaved, original); err != nil {
		t.Fatalf("a re-saved copy that shows the same page did not match: %v", err)
	}

	// The same re-save with one glyph changed does not match.
	changed := bytes.Replace(content[split:], []byte("Tj"), []byte("TJ"), 1)
	altered := appendRevision(t, original, map[int]string{
		firstID:    flateStream(t, content[:split]),
		secondID:   flateStream(t, changed),
		fontCopyID: reordered,
		pageID:     page,
	})
	if err := MatchPageContentExact(altered, original); err == nil {
		t.Fatal("a re-saved copy with changed content matched")
	}
}

// A re-save that moves the XMP metadata to a new object number and compresses
// it and the contract attachment keeps both readable: the manifest reference
// is found through the catalog's /Metadata, and both streams are decoded.
func TestResavedCopyKeepsItsManifestReferenceAndPayload(t *testing.T) {
	const manifestURL = "https://dcs.example/c2pa/manifest/did:example:resaved"
	compiled, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	updated, err := UpdatePDFWithOptions(testSigningContext(), compiled, []byte(minimalPayloadAmended), nil, manifestURL, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rootID, _ := currentRootObjID(updated)
	catalog := objectBody(t, updated, rootID)
	metadataRef := regexp.MustCompile(`/Metadata (\d+) 0 R`).FindStringSubmatch(catalog)
	if metadataRef == nil {
		t.Fatal("test setup: the catalog names no /Metadata")
	}
	var metadataID int
	if _, err := fmt.Sscanf(metadataRef[1], "%d", &metadataID); err != nil {
		t.Fatal(err)
	}
	xmp, err := extractStreamContentByObjID(updated, metadataID)
	if err != nil {
		t.Fatal(err)
	}
	payloadID := embeddedPayloadObjID(t, updated)
	payload, err := ExtractLatestEmbeddedJSONLD(updated)
	if err != nil {
		t.Fatal(err)
	}
	maxID, err := findTrailerMaxObjID(updated)
	if err != nil {
		t.Fatal(err)
	}
	newMetadataID := maxID + 1
	resaved := appendRevision(t, updated, map[int]string{
		rootID:        strings.Replace(catalog, metadataRef[0], fmt.Sprintf("/Metadata %d 0 R", newMetadataID), 1),
		newMetadataID: strings.Replace(flateStream(t, xmp), "<< ", "<< /Type /Metadata /Subtype /XML ", 1),
		payloadID:     flateStream(t, payload),
	})

	if got := ExtractRemoteManifestURL(resaved); got != manifestURL {
		t.Fatalf("manifest reference of the re-saved copy = %q, want %q", got, manifestURL)
	}
	got, err := ExtractLatestEmbeddedJSONLD(resaved)
	if err != nil || !bytes.Equal(got, payload) {
		t.Fatalf("payload of the re-saved copy differs (err %v)", err)
	}
}

// A signer may add the widget of a signature field as a kid of the field
// rather than merged into it; the widget then names the field as /Parent and
// carries no /FT of its own. It is still the signature layer.
func TestMatchPageContentLeavesOutAWidgetWhoseParentIsTheSignatureField(t *testing.T) {
	prepared := preparedContractPDF(t)
	pageID, _ := firstPageAndContentObjID(t, prepared)
	maxID, err := findTrailerMaxObjID(prepared)
	if err != nil {
		t.Fatal(err)
	}
	fieldID, widgetID := maxID+1, maxID+2
	page := regexp.MustCompile(`/Annots \[([^\]]*)\]`).ReplaceAllString(objectBody(t, prepared, pageID),
		fmt.Sprintf("/Annots [${1} %d 0 R]", widgetID))
	signed := appendRevision(t, prepared, map[int]string{
		fieldID:  fmt.Sprintf("<< /FT /Sig /T (Signature9) /Kids [%d 0 R] >>", widgetID),
		widgetID: fmt.Sprintf("<< /Type /Annot /Subtype /Widget /Parent %d 0 R /Rect [54 72 234 132] /P %d 0 R >>", fieldID, pageID),
		pageID:   page,
	})

	if err := MatchPageContent(signed, prepared); err != nil {
		t.Fatalf("a signature widget under its field must pass the signing pin: %v", err)
	}
	if err := MatchPageContentExact(signed, prepared); err == nil {
		t.Fatal("an added signature widget matched exactly")
	}
}

// The signing pin leaves signature widgets out, since a PAdES signer adds or
// fills one. A copy checked exactly must show the signatures as the reference
// does.
func TestMatchPageContentExactComparesSignatureWidgets(t *testing.T) {
	prepared := preparedContractPDF(t)
	signed := appendPAdESRevision(t, prepared, "Signature1")

	if err := MatchPageContent(signed, prepared); err != nil {
		t.Fatalf("the signing pin must tolerate the signature layer: %v", err)
	}
	if err := MatchPageContentExact(signed, prepared); err == nil {
		t.Fatal("an added signature widget matched exactly")
	}
	if err := MatchPageContentExact(signed, signed); err != nil {
		t.Fatalf("a document must match itself exactly: %v", err)
	}
}
