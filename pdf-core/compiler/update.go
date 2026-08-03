package compiler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ErrNoChanges is returned by UpdatePDF when the new payload is semantically
// identical to the current embedded one and no VC attachment is present.
var ErrNoChanges = errors.New("no changes: payloads are semantically identical")

// lifecycleVCFileName is the attachment filename under which a contract's
// current lifecycle credential is embedded.
const lifecycleVCFileName = "contract-lifecycle-vc.json"

// lifecycleStatusFromVC returns the lifecycle state an incremental update
// records in its dcs.lifecycle assertion: the state asserted by the credential
// the same update attaches. That credential is what the caller knows and what
// the eventual PAdES signature commits to, so it is what the provenance chain
// must say — hardcoding "amended" on every hop left a signed contract's chain
// reading draft -> amended -> amended while the credential beside it said
// "active", so the artifact and /pdf/verify's DB-derived lifecycle_status could
// disagree with nothing able to notice (ADR-13 requires the federation state to
// be derivable from the artifact alone).
//
// A hop that attaches no credential, or one naming no status, records no
// lifecycle event: it is a content amendment, and "amended" is the whole truth
// about it.
func lifecycleStatusFromVC(vcBytes []byte) string {
	var vc struct {
		CredentialSubject struct {
			Status string `json:"status"`
		} `json:"credentialSubject"`
	}
	if err := json.Unmarshal(vcBytes, &vc); err != nil || vc.CredentialSubject.Status == "" {
		return lifecycleStatusAmended
	}
	return vc.CredentialSubject.Status
}

var pdfTrailerSizeRE = regexp.MustCompile(`/Size (\d+)`)
var pdfTrailerIDRE = regexp.MustCompile(`/ID\s*(\[[^\]]*\])`)
var pdfTrailerRootRE = regexp.MustCompile(`/Root (\d+) 0 R`)
var pdfKidsRE = regexp.MustCompile(`/Kids \[([^\]]+)\]`)
var pdfObjRefRE = regexp.MustCompile(`(\d+) 0 R`)

// currentRootObjID returns the object number of /Root in the PDF's most recent
// trailer. A PAdES signer supersedes the document catalog with a new object
// carrying an inline AcroForm; a later incremental update must keep /Root
// pointing at that signed catalog so the signature field's /V link stays the
// current one the reader resolves.
func currentRootObjID(pdf []byte) (int, bool) {
	idx := bytes.LastIndex(pdf, []byte("trailer\n"))
	if idx < 0 {
		return 0, false
	}
	m := pdfTrailerRootRE.FindSubmatch(pdf[idx:])
	if len(m) < 2 {
		return 0, false
	}
	id, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return 0, false
	}
	return id, true
}

// IsPAdESSigned reports whether pdf carries a PAdES signature: a signature
// value dictionary (/Type /Sig) with a /ByteRange. Exported for the service
// layer's offline-tamper check on the plain re-render verify path.
func IsPAdESSigned(pdf []byte) bool { return isPAdESSigned(pdf) }

// isPAdESSigned: a C2PA lifecycle update over a signed PDF must be
// provenance-only — re-rendering the pages or re-stamping the AcroForm
// signature field would drop the signed field's /V and invalidate the
// signature (DCS-OR-C2PA-010).
func isPAdESSigned(pdf []byte) bool {
	if !bytes.Contains(pdf, []byte("/ByteRange")) {
		return false
	}
	return bytes.Contains(pdf, []byte("/Type /Sig")) || bytes.Contains(pdf, []byte("/Type/Sig"))
}

// extractTrailerID returns the raw /ID array string (e.g. "[<abc…> <def…>]")
// from the last trailer in the PDF, or an empty string if not found.
func extractTrailerID(pdf []byte) string {
	idx := bytes.LastIndex(pdf, []byte("trailer\n"))
	if idx < 0 {
		return ""
	}
	m := pdfTrailerIDRE.FindSubmatch(pdf[idx:])
	if len(m) < 2 {
		return ""
	}
	return string(m[1])
}

// findTrailerMaxObjID returns the maximum object ID in use (/Size - 1 from the last trailer).
func findTrailerMaxObjID(pdf []byte) (int, error) {
	idx := bytes.LastIndex(pdf, []byte("trailer\n"))
	if idx < 0 {
		return 0, fmt.Errorf("trailer not found in PDF")
	}
	m := pdfTrailerSizeRE.FindSubmatch(pdf[idx:])
	if len(m) < 2 {
		return 0, fmt.Errorf("/Size not found in PDF trailer")
	}
	size, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return 0, fmt.Errorf("invalid trailer /Size: %w", err)
	}
	return size - 1, nil
}

// parseCurrentPagesKids returns the page object IDs of the page tree the
// document's current Catalog points at.
//
// The tree is FOUND, not assumed to be object 2: an appended revision may
// supersede the Catalog to name a different /Pages while leaving object 2
// untouched, and a checker reading object 2 then compares pages a reader never
// renders.
func parseCurrentPagesKids(pdf []byte) ([]int, error) {
	pagesID, err := currentPagesObjID(pdf)
	if err != nil {
		return nil, err
	}
	start, end, ok := lastObjectBody(pdf, pagesID)
	if !ok {
		return nil, fmt.Errorf("Pages object (%d) not found", pagesID)
	}
	kidsMatch := pdfKidsRE.Find(pdf[start:end])
	if kidsMatch == nil {
		return nil, fmt.Errorf("/Kids not found in Pages object")
	}
	refs := pdfObjRefRE.FindAllSubmatch(kidsMatch, -1)
	ids := make([]int, 0, len(refs))
	for _, m := range refs {
		id, err := strconv.Atoi(string(m[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid page ref %q: %w", m[0], err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

// UpdatePDF appends a PDF incremental update to oldPDF that replaces the
// visible page content with a freshly compiled version of newPayload.
// The original PDF bytes are preserved unchanged as a prefix so existing
// C2PA hard-binding signatures remain verifiable over the original byte range.
func UpdatePDF(ctx context.Context, oldPDF []byte, newPayload []byte, compiledAt time.Time) ([]byte, error) {
	return updatePDF(ctx, oldPDF, newPayload, nil, "", compiledAt, false)
}

// UpdatePDFWithVC appends a PDF incremental update that replaces visible page
// content with a freshly compiled version of newPayload AND embeds vcBytes as
// a "contract-lifecycle-vc.json" attached file.
//
// Unlike UpdatePDF, this function proceeds even when newPayload is semantically
// identical to the current embedded payload, because the VC attachment is itself
// a provenance event (e.g. attaching the initial lifecycle credential to a
// freshly compiled base PDF).
//
// When vcBytes is nil the call delegates to UpdatePDF unchanged.
func UpdatePDFWithVC(ctx context.Context, oldPDF []byte, newPayload []byte, vcBytes []byte, compiledAt time.Time) ([]byte, error) {
	if len(vcBytes) == 0 {
		return UpdatePDF(ctx, oldPDF, newPayload, compiledAt)
	}
	return updatePDF(ctx, oldPDF, newPayload, vcBytes, "", compiledAt, false)
}

// UpdatePDFWithOptions is the full-control entry point used by the DCS backend.
// It behaves like UpdatePDFWithVC (vcBytes may be nil) but additionally accepts
// a remoteManifestURL that, when non-empty, is embedded as the C2PA claim's
// remote_manifests field (DCS-OR-C2PA-008 AC3). When remoteManifestURL is empty
// the output is identical to UpdatePDF / UpdatePDFWithVC.
func UpdatePDFWithOptions(ctx context.Context, oldPDF []byte, newPayload []byte, vcBytes []byte, remoteManifestURL string, compiledAt time.Time) ([]byte, error) {
	return updatePDF(ctx, oldPDF, newPayload, vcBytes, remoteManifestURL, compiledAt, false)
}

// ReanchorProvenance appends a provenance-only C2PA manifest whose hard
// binding covers the document's current bytes, without changing its payload.
//
// A PAdES signature is applied after the lifecycle manifest so that the
// signature commits to the provenance (ADR-26). That leaves the manifest's
// whole-file binding covering less than the file it now lives in, and no
// amendment can fix it: the payload has not changed, and the amendment path
// refuses an unchanged document. This appends, so the signature's byte range
// is untouched and the signature keeps verifying in external tools.
func ReanchorProvenance(ctx context.Context, oldPDF []byte, remoteManifestURL string, compiledAt time.Time) ([]byte, error) {
	payload, err := ExtractLatestEmbeddedJSONLD(oldPDF)
	if err != nil {
		return nil, fmt.Errorf("extract embedded payload to re-anchor: %w", err)
	}
	return updatePDF(ctx, oldPDF, payload, nil, remoteManifestURL, compiledAt, true)
}

// ExtractManifestStore returns the raw JUMBF C2PA manifest store bytes
// embedded in the PDF under the "content_credential.c2pa" file attachment.
func ExtractManifestStore(pdf []byte) ([]byte, error) {
	return extractEmbeddedStreamByFileSpecName(pdf, "content_credential.c2pa")
}

// updatePDF is the shared implementation behind all Update*/Reanchor entry
// points. The "no changes" guard is bypassed when vcBytes is non-nil or the
// call is a re-anchor.
func updatePDF(ctx context.Context, oldPDF []byte, newPayload []byte, vcBytes []byte, remoteManifestURL string, compiledAt time.Time, reanchor bool) ([]byte, error) {
	oldPayload, err := ExtractEmbeddedJSONLD(oldPDF)
	if err != nil {
		return nil, fmt.Errorf("extract embedded JSON-LD: %w", err)
	}

	// Hash the verbatim bytes — the same content-address CompilePDF renders, so the
	// backlink + FileID match a fresh compile, and the "no changes" guard compares
	// the exact old vs new attachment bytes.
	oldHash := sha256.Sum256(oldPayload)
	newHash := sha256.Sum256(newPayload)
	oldHashHex := hex.EncodeToString(oldHash[:])
	newHashHex := hex.EncodeToString(newHash[:])

	// A re-anchor deliberately carries an unchanged payload; every other caller
	// gets the no-changes guard.
	if oldHashHex == newHashHex && len(vcBytes) == 0 && !reanchor {
		return nil, ErrNoChanges
	}

	maxObjID, err := findTrailerMaxObjID(oldPDF)
	if err != nil {
		return nil, fmt.Errorf("find max object ID: %w", err)
	}
	prevStartXref, err := previousStartXref(oldPDF)
	if err != nil {
		return nil, fmt.Errorf("find startxref: %w", err)
	}

	newDoc, err := extractDocumentModel(newPayload, newHashHex)
	if err != nil {
		return nil, err
	}
	// Carry the amended attachment verbatim (same rule as the initial compile):
	// the superseding embedded object holds the exact submitted bytes.
	newDoc.EmbeddedPayload = newPayload
	newDoc.PayloadCID = payloadCID(newPayload)

	// A PAdES signature freezes the visible content: the signature's /ByteRange
	// covers the pages and its DocMDP permissions forbid altering them. A
	// lifecycle update over a signed PDF is therefore provenance-only — it must
	// not re-render pages or re-stamp the signed AcroForm field, both of which
	// would strip the field's /V and invalidate the signature. It keeps /Root
	// pointing at the signer's catalog (whose inline AcroForm carries the signed
	// field) rather than reverting to the base catalog.
	signed := isPAdESSigned(oldPDF)
	rootObjID := 1
	// The manifest a provenance-only update embeds describes the frozen, already
	// embedded payload — not the caller's newPayload, which post-signing must
	// equal it. Building from the embedded payload keeps the update reproducible
	// from the PDF's own bytes: VerifyIncrementalUpdate re-extracts that same
	// (unchanged) embedded payload and rebuilds the identical manifest.
	manifestDoc := newDoc
	manifestHashHex := newHashHex
	if signed {
		if r, ok := currentRootObjID(oldPDF); ok {
			rootObjID = r
		}
		manifestDoc, err = extractDocumentModel(oldPayload, oldHashHex)
		if err != nil {
			return nil, fmt.Errorf("extract frozen document model: %w", err)
		}
		manifestHashHex = oldHashHex
	}

	// Compile the new document into full page layouts and assign new object IDs
	// beyond the current maximum so the original objects are never overwritten.
	// Skipped for signed PDFs, whose pages must stay byte-for-byte as signed.
	nextID := maxObjID + 1
	var newPages []pageLayout
	if !signed {
		newPages = layoutDocumentPages(newDoc)
		for i := range newPages {
			newPages[i].ObjectID = nextID
			nextID++
			newPages[i].ContentID = nextID
			nextID++
			for j := range newPages[i].Annotations {
				newPages[i].Annotations[j].ObjectID = nextID
				nextID++
			}
			for j := range newPages[i].SigFields {
				newPages[i].SigFields[j].AppearanceObjectID = nextID
				nextID++
				newPages[i].SigFields[j].WidgetObjectID = nextID
				nextID++
			}
		}
	}

	// Reserve IDs for VC embedded-file and filespec objects when a VC is present.
	vcFileObjID := 0
	vcSpecObjID := 0
	if vcBytes != nil {
		vcFileObjID = nextID
		nextID++
		vcSpecObjID = nextID
	}

	// When a lifecycle VC is attached, supersede the catalog so the VC filespec
	// is a listed associated file (ISO 19005-3 clause 6.8): its /AFRelationship
	// requires membership in the document /AF array and /EmbeddedFiles tree.
	var patchedCatalog []byte
	if vcBytes != nil {
		patchedCatalog, err = catalogWithAssociatedFile(oldPDF, rootObjID, vcSpecObjID, lifecycleVCFileName)
		if err != nil {
			return nil, fmt.Errorf("associate lifecycle VC in catalog: %w", err)
		}
	}

	originalC2PA, err := extractEmbeddedStreamByFileSpecName(oldPDF, "content_credential.c2pa")
	if err != nil {
		return nil, fmt.Errorf("extract original C2PA: %w", err)
	}

	oldSize := maxObjID + 1
	fileID := extractTrailerID(oldPDF)
	hardBindingHash := make([]byte, 32)
	exclusions := []c2paExclusion{}
	var result []byte

	for range 6 {
		updatedC2PA, err := renderVerificationManifestStore(ctx, originalC2PA, updateManifestLabel(hardBindingHash), manifestDoc.ContractID, manifestHashHex, lifecycleStatusFromVC(vcBytes), hardBindingHash, exclusions, compiledAt, remoteManifestURL)
		if err != nil {
			return nil, fmt.Errorf("render update manifest: %w", err)
		}
		var appendix []byte
		if signed {
			appendix = buildSignedUpdateAppendixBytes(
				len(oldPDF), prevStartXref, oldSize, rootObjID, fileID,
				updatedC2PA, newDoc.EmbeddedPayload, newDoc.PayloadHash,
				vcBytes, vcFileObjID, vcSpecObjID, patchedCatalog, remoteManifestURL,
			)
		} else {
			appendix = buildUpdateAppendixBytes(
				len(oldPDF), prevStartXref, oldSize, fileID,
				updatedC2PA, newDoc.EmbeddedPayload, newDoc.PayloadHash,
				newPages, vcBytes, vcFileObjID, vcSpecObjID,
				rootObjID, patchedCatalog, remoteManifestURL,
			)
		}
		result = append(append([]byte(nil), oldPDF...), appendix...)

		streamStart, streamLen, found := findLastObjectStreamRange(result, 9)
		if !found {
			return result, nil
		}
		nextExclusions := buildC2PAExclusions(streamStart, streamLen)
		nextHash := sha256WithExclusions(result, nextExclusions)
		if bytes.Equal(hardBindingHash, nextHash[:]) && exclusionsEqual(exclusions, nextExclusions) {
			return result, nil
		}
		hardBindingHash = append([]byte(nil), nextHash[:]...)
		exclusions = nextExclusions
	}
	return result, nil
}

// catalogWithAssociatedFile reads the document catalog (objID) from pdf and
// returns its dictionary bytes with the filespec specObjID listed as the
// associated file called fileName: appended to the /AF array and resolvable
// under fileName in the /EmbeddedFiles name tree, which is what ISO 19005-3
// clause 6.8 requires of an attachment carrying /AFRelationship. Returns the
// dict without the object header/trailer, to be re-emitted as a superseded
// object.
//
// A document accumulates one filespec per attachment: a contract is amended
// under its "draft" lifecycle credential, then stamped "active" under a fresh
// one just before signing. /AF grows — every revision's attachment stays
// reachable — but a name tree holds exactly one entry per name, so that entry
// must be re-pointed at the current filespec. Leaving the first one in place is
// what made every reader that resolves an attachment BY NAME (pypdf, Acrobat's
// attachment panel, a wallet) report "draft" for a signed contract; only the
// backend was spared, because ExtractEmbeddedVC scans for the last filespec
// instead of asking the name tree.
func catalogWithAssociatedFile(pdf []byte, objID, specObjID int, fileName string) ([]byte, error) {
	start, end, ok := lastObjectBody(pdf, objID)
	if !ok {
		return nil, fmt.Errorf("catalog object %d not found", objID)
	}
	dict := append([]byte(nil), pdf[start:end]...)
	ref := []byte(fmt.Sprintf("%d 0 R", specObjID))
	if af := catalogAFRE.FindSubmatchIndex(dict); af != nil && !bytes.Contains(dict[af[2]:af[3]], ref) {
		dict = catalogAFRE.ReplaceAll(dict, []byte("/AF [${1} "+string(ref)+"]"))
	}
	ef := catalogEFRE.FindSubmatchIndex(dict)
	if ef == nil {
		return dict, nil
	}
	names := dict[ef[4]:ef[5]]
	entry := []byte("(" + fileName + ") " + string(ref))
	if existing := nameTreeEntryRE(fileName).FindIndex(names); existing != nil {
		names = append(append(append([]byte(nil), names[:existing[0]]...), entry...), names[existing[1]:]...)
	} else {
		names = append(append(append([]byte(nil), names...), ' '), entry...)
	}
	return append(dict[:ef[4]:ef[4]], append(names, dict[ef[5]:]...)...), nil
}

// nameTreeEntryRE matches one /EmbeddedFiles name-tree entry — the key string
// followed by the filespec reference it resolves to.
func nameTreeEntryRE(fileName string) *regexp.Regexp {
	return regexp.MustCompile(regexp.QuoteMeta("("+fileName+")") + `\s*\d+ 0 R`)
}

var (
	catalogAFRE = regexp.MustCompile(`/AF \[([^\]]*)\]`)
	catalogEFRE = regexp.MustCompile(`(/EmbeddedFiles << /Names \[)([^\]]*)(\])`)
)

// buildUpdateAppendixBytes constructs the raw bytes of the PDF incremental
// update section. It supersedes:
//   - obj 2  (Pages)        — updated /Kids list pointing to new page objects
//   - obj 9  (C2PA manifest) — updated hard-binding hash and provenance chain
//   - obj 11 (embedded JSON-LD) — replaced with the new payload, carried verbatim
//
// New objects (page content streams, page dictionaries, annotations) are
// appended with IDs beyond the existing maximum so originals are unreachable
// via the updated xref chain but their bytes remain intact for signature
// verification.
func buildUpdateAppendixBytes(
	baseLen, prevStartXref, oldSize int,
	fileID string,
	updatedC2PA, newEmbeddedPayload []byte,
	newPayloadHash string,
	newPages []pageLayout,
	vcBytes []byte, vcFileObjID, vcSpecObjID int,
	rootObjID int, patchedCatalog []byte,
	remoteManifestURL string,
) []byte {
	const (
		fontObjID     = 6
		pagesObjID    = 2
		c2paObjID     = 9
		embFileID     = 11
		acroFormID    = 14
		metadataObjID = 13
	)

	type objEntry struct{ id, offset int }
	var entries []objEntry

	var buf bytes.Buffer
	buf.WriteString("\n% dcs-pdf-core incremental update\n")

	for _, page := range newPages {
		entries = append(entries, objEntry{page.ContentID, baseLen + buf.Len()})
		content := renderContentStream(page)
		buf.WriteString(fmt.Sprintf("%d 0 obj\n", page.ContentID))
		buf.Write(streamObject([]byte(content), fmt.Sprintf("<< /Length %d >>", len(content))))
		buf.WriteString("\nendobj\n")

		entries = append(entries, objEntry{page.ObjectID, baseLen + buf.Len()})
		buf.WriteString(fmt.Sprintf("%d 0 obj\n", page.ObjectID))
		buf.WriteString(renderPageObject(page, fontObjID, pagesObjID))
		buf.WriteString("\nendobj\n")

		for _, annotation := range page.Annotations {
			entries = append(entries, objEntry{annotation.ObjectID, baseLen + buf.Len()})
			buf.WriteString(fmt.Sprintf("%d 0 obj\n", annotation.ObjectID))
			buf.WriteString(renderAnnotationObject(annotation, newPages))
			buf.WriteString("\nendobj\n")
		}

		// Re-emit signature field appearance streams and widget objects so the
		// new pages reference valid (non-null) annotation objects. The original
		// widget objects on the superseded pages are no longer reachable via the
		// updated xref chain, so new ones must be written for each sig field.
		for _, sigField := range page.SigFields {
			appearance := renderSigFieldAppearanceStream(sigField)
			entries = append(entries, objEntry{sigField.AppearanceObjectID, baseLen + buf.Len()})
			buf.WriteString(fmt.Sprintf("%d 0 obj\n", sigField.AppearanceObjectID))
			buf.Write(streamObject([]byte(appearance), fmt.Sprintf(
				"<< /Type /XObject /Subtype /Form /BBox [0 0 %.2f %.2f] /Resources << /Font << /F1 %d 0 R >> >> /Length %d >>",
				sigField.Rect[2]-sigField.Rect[0], sigField.Rect[3]-sigField.Rect[1],
				fontObjID, len(appearance),
			)))
			buf.WriteString("\nendobj\n")

			entries = append(entries, objEntry{sigField.WidgetObjectID, baseLen + buf.Len()})
			buf.WriteString(fmt.Sprintf("%d 0 obj\n", sigField.WidgetObjectID))
			buf.WriteString(renderSigFieldWidgetObject(sigField, page.ObjectID, sigField.AppearanceObjectID))
			buf.WriteString("\nendobj\n")
		}
	}

	// Supersede obj 14 (AcroForm) when the new document has signature fields.
	// The original AcroForm referenced widgets on the old pages; those pages are
	// no longer current, so the AcroForm must point to the freshly emitted widgets.
	var sigFieldRefs []string
	for _, page := range newPages {
		for _, sigField := range page.SigFields {
			sigFieldRefs = append(sigFieldRefs, fmt.Sprintf("%d 0 R", sigField.WidgetObjectID))
		}
	}
	if len(sigFieldRefs) > 0 {
		entries = append(entries, objEntry{acroFormID, baseLen + buf.Len()})
		buf.WriteString(fmt.Sprintf("%d 0 obj\n<< /Fields [%s] /SigFlags 3 /DA (/F1 10 Tf 0 g) >>\nendobj\n",
			acroFormID, strings.Join(sigFieldRefs, " ")))
	}

	// Supersede obj 11: updated embedded JSON-LD.
	entries = append(entries, objEntry{embFileID, baseLen + buf.Len()})
	buf.WriteString(fmt.Sprintf("%d 0 obj\n", embFileID))
	buf.Write(streamObject(newEmbeddedPayload, fmt.Sprintf(
		"<< /Type /EmbeddedFile /Subtype /application#2Fld+json /Length %d /Params << /Size %d /CheckSum <%s> >> >>",
		len(newEmbeddedPayload), len(newEmbeddedPayload), newPayloadHash[:32],
	)))
	buf.WriteString("\nendobj\n")

	// Supersede obj 2: Pages now points only to the new compiled page objects,
	// replacing the old pages in the reader's view while leaving original
	// objects intact in the byte stream.
	newKids := make([]string, len(newPages))
	for i, p := range newPages {
		newKids[i] = fmt.Sprintf("%d 0 R", p.ObjectID)
	}
	entries = append(entries, objEntry{pagesObjID, baseLen + buf.Len()})
	buf.WriteString(fmt.Sprintf("%d 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n",
		pagesObjID, strings.Join(newKids, " "), len(newPages)))

	// Append VC embedded-file and filespec objects when a credential is supplied.
	// The EmbeddedFile stream is written before the Filespec so that ExtractVC
	// (which scans backwards from the filename marker) finds the correct stream.
	if len(vcBytes) > 0 {
		entries = append(entries, objEntry{vcFileObjID, baseLen + buf.Len()})
		buf.WriteString(fmt.Sprintf("%d 0 obj\n", vcFileObjID))
		buf.Write(streamObject(vcBytes, fmt.Sprintf(
			"<< /Type /EmbeddedFile /Subtype /application#2Fjson /Length %d >>", len(vcBytes))))
		buf.WriteString("\nendobj\n")

		entries = append(entries, objEntry{vcSpecObjID, baseLen + buf.Len()})
		buf.WriteString(fmt.Sprintf(
			"%d 0 obj\n<< /Type /Filespec /F (contract-lifecycle-vc.json) /UF (contract-lifecycle-vc.json) /AFRelationship /Supplement /EF << /F %d 0 R >> >>\nendobj\n",
			vcSpecObjID, vcFileObjID))
	}

	// Supersede obj 13 (XMP Metadata) when a remote manifest URL is supplied,
	// adding a dcterms:provenance property (DCS-OR-C2PA-008 AC3) — the
	// C2PA-normative remote-manifest-discovery mechanism (see renderXMPMetadata's
	// doc comment for why this replaced a non-standard C2PA claim field).
	if remoteManifestURL != "" {
		updatedXMP := renderXMPMetadata(remoteManifestURL)
		entries = append(entries, objEntry{metadataObjID, baseLen + buf.Len()})
		buf.WriteString(fmt.Sprintf("%d 0 obj\n", metadataObjID))
		buf.Write(streamObject(updatedXMP, fmt.Sprintf(
			"<< /Type /Metadata /Subtype /XML /Length %d >>", len(updatedXMP))))
		buf.WriteString("\nendobj\n")
	}

	// Supersede obj 9: updated C2PA manifest — written last so stream offset stabilises.
	entries = append(entries, objEntry{c2paObjID, baseLen + buf.Len()})
	buf.WriteString(fmt.Sprintf("%d 0 obj\n", c2paObjID))
	buf.Write(streamObject(updatedC2PA, fmt.Sprintf(
		"<< /Type /EmbeddedFile /Subtype /application#2Fc2pa /Length %d >>", len(updatedC2PA))))
	buf.WriteString("\nendobj\n")

	// Write xref with contiguous subsections.
	if len(patchedCatalog) > 0 {
		entries = append(entries, objEntry{rootObjID, baseLen + buf.Len()})
		buf.WriteString(fmt.Sprintf("%d 0 obj\n", rootObjID))
		buf.Write(patchedCatalog)
		buf.WriteString("\nendobj\n")
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].id < entries[j].id })
	xrefStart := baseLen + buf.Len()
	buf.WriteString("xref\n")
	i := 0
	for i < len(entries) {
		j := i + 1
		for j < len(entries) && entries[j].id == entries[j-1].id+1 {
			j++
		}
		buf.WriteString(fmt.Sprintf("%d %d\n", entries[i].id, j-i))
		for k := i; k < j; k++ {
			buf.WriteString(fmt.Sprintf("%010d 00000 n \n", entries[k].offset))
		}
		i = j
	}

	newMaxID := 0
	for _, e := range entries {
		if e.id > newMaxID {
			newMaxID = e.id
		}
	}
	newSize := oldSize
	if newMaxID+1 > newSize {
		newSize = newMaxID + 1
	}

	// ISO 19005-3:2012 clause 6.1.3 requires /ID in every trailer, including
	// incremental update trailers. Carry the original file's /ID forward.
	idEntry := ""
	if fileID != "" {
		idEntry = " /ID " + fileID
	}
	buf.WriteString(fmt.Sprintf(
		"trailer\n<< /Size %d /Root 1 0 R /Prev %d%s >>\nstartxref\n%d\n%%%%EOF\n",
		newSize, prevStartXref, idEntry, xrefStart,
	))
	return buf.Bytes()
}

// buildSignedUpdateAppendixBytes constructs a provenance-only incremental
// update for a PDF that already carries a PAdES signature. It supersedes only
// the C2PA manifest stream (obj 9) and appends the optional lifecycle VC, all
// after the signed byte range so the PAdES /ByteRange stays valid. It leaves
// the pages, the AcroForm, and the signature field widget (with its /V link)
// untouched, and keeps /Root pointing at the signer's catalog (rootObjID) so
// that signed AcroForm remains the one the reader resolves.
func buildSignedUpdateAppendixBytes(
	baseLen, prevStartXref, oldSize, rootObjID int,
	fileID string,
	updatedC2PA, newEmbeddedPayload []byte, newPayloadHash string,
	vcBytes []byte, vcFileObjID, vcSpecObjID int,
	patchedCatalog []byte,
	remoteManifestURL string,
) []byte {
	const (
		c2paObjID     = 9
		embFileID     = 11
		metadataObjID = 13
	)

	type objEntry struct{ id, offset int }
	var entries []objEntry

	var buf bytes.Buffer
	buf.WriteString("\n% dcs-pdf-core incremental update\n")

	// Lifecycle VC attachment (a provenance event), appended as new objects.
	if len(vcBytes) > 0 {
		entries = append(entries, objEntry{vcFileObjID, baseLen + buf.Len()})
		buf.WriteString(fmt.Sprintf("%d 0 obj\n", vcFileObjID))
		buf.Write(streamObject(vcBytes, fmt.Sprintf(
			"<< /Type /EmbeddedFile /Subtype /application#2Fjson /Length %d >>", len(vcBytes))))
		buf.WriteString("\nendobj\n")

		entries = append(entries, objEntry{vcSpecObjID, baseLen + buf.Len()})
		buf.WriteString(fmt.Sprintf(
			"%d 0 obj\n<< /Type /Filespec /F (contract-lifecycle-vc.json) /UF (contract-lifecycle-vc.json) /AFRelationship /Supplement /EF << /F %d 0 R >> >>\nendobj\n",
			vcSpecObjID, vcFileObjID))
	}

	// Supersede obj 13 (XMP Metadata) when a remote manifest URL is supplied —
	// see the identical block in buildUpdateAppendixBytes for the rationale.
	if remoteManifestURL != "" {
		updatedXMP := renderXMPMetadata(remoteManifestURL)
		entries = append(entries, objEntry{metadataObjID, baseLen + buf.Len()})
		buf.WriteString(fmt.Sprintf("%d 0 obj\n", metadataObjID))
		buf.Write(streamObject(updatedXMP, fmt.Sprintf(
			"<< /Type /Metadata /Subtype /XML /Length %d >>", len(updatedXMP))))
		buf.WriteString("\nendobj\n")
	}

	// Supersede obj 11 (embedded JSON-LD) with the new payload, carried VERBATIM,
	// as an appended incremental-update object (SRS DCS-OR-C2PA-002: the amend must
	// use incremental updates so the existing PAdES signature stays valid — the
	// original signed bytes are preserved as a prefix; the new payload lives here,
	// beyond the sealed range). Content-changing amends of a signed PDF are a
	// first-class "amended" lifecycle transition, not a frozen provenance-only one.
	entries = append(entries, objEntry{embFileID, baseLen + buf.Len()})
	buf.WriteString(fmt.Sprintf("%d 0 obj\n", embFileID))
	buf.Write(streamObject(newEmbeddedPayload, fmt.Sprintf(
		"<< /Type /EmbeddedFile /Subtype /application#2Fld+json /Length %d /Params << /Size %d /CheckSum <%s> >> >>",
		len(newEmbeddedPayload), len(newEmbeddedPayload), newPayloadHash[:32],
	)))
	buf.WriteString("\nendobj\n")

	// Supersede obj 9: updated C2PA manifest — written last so its stream offset
	// stabilises across the hard-binding hash iterations. The catalog reaches it
	// unchanged via the content_credential.c2pa filespec's /EF reference.
	entries = append(entries, objEntry{c2paObjID, baseLen + buf.Len()})
	buf.WriteString(fmt.Sprintf("%d 0 obj\n", c2paObjID))
	buf.Write(streamObject(updatedC2PA, fmt.Sprintf(
		"<< /Type /EmbeddedFile /Subtype /application#2Fc2pa /Length %d >>", len(updatedC2PA))))
	buf.WriteString("\nendobj\n")

	if len(patchedCatalog) > 0 {
		entries = append(entries, objEntry{rootObjID, baseLen + buf.Len()})
		buf.WriteString(fmt.Sprintf("%d 0 obj\n", rootObjID))
		buf.Write(patchedCatalog)
		buf.WriteString("\nendobj\n")
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].id < entries[j].id })
	xrefStart := baseLen + buf.Len()
	buf.WriteString("xref\n")
	i := 0
	for i < len(entries) {
		j := i + 1
		for j < len(entries) && entries[j].id == entries[j-1].id+1 {
			j++
		}
		buf.WriteString(fmt.Sprintf("%d %d\n", entries[i].id, j-i))
		for k := i; k < j; k++ {
			buf.WriteString(fmt.Sprintf("%010d 00000 n \n", entries[k].offset))
		}
		i = j
	}

	newMaxID := 0
	for _, e := range entries {
		if e.id > newMaxID {
			newMaxID = e.id
		}
	}
	newSize := oldSize
	if newMaxID+1 > newSize {
		newSize = newMaxID + 1
	}

	idEntry := ""
	if fileID != "" {
		idEntry = " /ID " + fileID
	}
	buf.WriteString(fmt.Sprintf(
		"trailer\n<< /Size %d /Root %d 0 R /Prev %d%s >>\nstartxref\n%d\n%%%%EOF\n",
		newSize, rootObjID, prevStartXref, idEntry, xrefStart,
	))
	return buf.Bytes()
}

// ExtractEmbeddedVC extracts the raw bytes of the "contract-lifecycle-vc.json"
// embedded-file attachment from a PDF produced by UpdatePDFWithVC.
// Returns (vcBytes, true, nil) when the attachment is present; (nil, false, nil)
// when absent; and (nil, false, err) on a malformed reference.
func ExtractEmbeddedVC(pdf []byte) ([]byte, bool, error) {
	specPos := bytes.LastIndex(pdf, []byte("/F (contract-lifecycle-vc.json)"))
	if specPos < 0 {
		return nil, false, nil
	}
	efPos := bytes.Index(pdf[specPos:], []byte("/EF << /F "))
	if efPos < 0 {
		return nil, false, fmt.Errorf("contract-lifecycle-vc.json filespec missing /EF reference")
	}
	efPos += specPos + len("/EF << /F ")
	refEnd := bytes.Index(pdf[efPos:], []byte(" 0 R"))
	if refEnd < 0 {
		return nil, false, fmt.Errorf("contract-lifecycle-vc.json object reference malformed")
	}
	objIDStr := strings.TrimSpace(string(pdf[efPos : efPos+refEnd]))
	objID, err := strconv.Atoi(objIDStr)
	if err != nil {
		return nil, false, fmt.Errorf("contract-lifecycle-vc.json object id invalid: %w", err)
	}
	// The most recent definition wins (incremental update semantics).
	streamStart, streamEnd, ok := lastObjectStreamData(pdf, objID)
	if !ok {
		return nil, false, fmt.Errorf("contract-lifecycle-vc.json stream not found in object %d", objID)
	}
	return append([]byte(nil), pdf[streamStart:streamEnd]...), true, nil
}

// incrementalUpdateMarker is the comment written as the very first line of
// every incremental update section produced by UpdatePDF.
var incrementalUpdateMarker = []byte("\n% dcs-pdf-core incremental update\n")

// SplitAtIncrementalUpdate returns the original PDF prefix that precedes the
// first dcs-pdf-core incremental update marker, and ok=true.  If the PDF has
// no such marker (it is a plain compiled document) ok is false.
func SplitAtIncrementalUpdate(pdf []byte) (original []byte, ok bool) {
	idx := bytes.Index(pdf, incrementalUpdateMarker)
	if idx < 0 {
		return nil, false
	}
	return pdf[:idx], true
}

// incrementalUpdateMarkerOffsets returns the byte offset of every occurrence
// of incrementalUpdateMarker in pdf, in file order — one per C2PA lifecycle
// update chained onto the original compiled document.
func incrementalUpdateMarkerOffsets(pdf []byte) []int {
	var offsets []int
	base := 0
	for {
		idx := bytes.Index(pdf[base:], incrementalUpdateMarker)
		if idx < 0 {
			return offsets
		}
		offsets = append(offsets, base+idx)
		base += idx + len(incrementalUpdateMarker)
	}
}

// VerifyIncrementalUpdate checks that an incrementally-updated PDF was produced
// deterministically from its embedded payloads.  It provides the same guarantee
// as the plain /verify check — the human-readable content is fully determined
// by the machine-readable JSON-LD — extended to cover the ENTIRE amendment
// history, not just a single hop: a contract's PDF typically accumulates
// several C2PA lifecycle updates before it is ever signed (e.g. submitted,
// approved) and may accumulate more afterwards (e.g. revoked); a PAdES
// signature and its signing-evidence attachment may sit, opaque to this check,
// between any two updates. For each hop i (1-based, in chain order):
//
//  1. i==1: CompilePDF(oldPayload) == boundary[0]  (the base compile was deterministic)
//  2. UpdatePDF(boundary[i-1], newPayload_i) == boundary[i]  (hop i was deterministic)
//
// where boundary[i] is the PDF prefix ending right after the i-th update's
// appendix (boundary[N] == the full pdf). All hops together prove the current
// visible state is reproducible, end to end, from its embedded payloads.
//
// The returned bytes are the deterministic reproduction the verdict was reached
// on: the replay of the last hop when every hop held, and the replay of the hop
// that diverged when one did not. A caller that must show WHY it decided as it
// did (e.g. by digesting both sides) gets the evidence rather than having to
// recompute it; nil is returned only when the chain failed before any
// reproduction could be produced.
func VerifyIncrementalUpdate(ctx context.Context, pdf []byte) ([]byte, error) {
	offsets := incrementalUpdateMarkerOffsets(pdf)
	if len(offsets) == 0 {
		return nil, fmt.Errorf("no incremental update marker found")
	}

	boundary := pdf[:offsets[0]]
	var reproduced []byte

	oldPayload, err := ExtractEmbeddedJSONLD(boundary)
	if err != nil {
		return nil, fmt.Errorf("extract old payload from original prefix: %w", err)
	}
	originalC2PA, err := extractEmbeddedStreamByFileSpecName(boundary, "content_credential.c2pa")
	if err != nil {
		return nil, fmt.Errorf("extract original C2PA: %w", err)
	}
	originalCompiledAt, err := extractLifecycleEffectiveAt(originalC2PA, 0)
	if err != nil {
		return nil, fmt.Errorf("extract original lifecycle timestamp: %w", err)
	}
	// The asserting instance's DID is carried by the manifest, not the payload,
	// so a verifier that never saw it must read it back off the document for the
	// recompilation to reproduce the stored bytes — as with the timestamp above.
	originalAuthority, err := extractLifecycleAuthority(originalC2PA, 0)
	if err != nil {
		return nil, fmt.Errorf("extract original lifecycle authority: %w", err)
	}
	// Same reasoning as the authority above, applied to the signing leaf: the
	// x5chain sits in the signed COSE headers, and this process's configured
	// chain is its own instance's. Substituting it could never reproduce a
	// document compiled by a federation peer.
	originalChain, err := extractManifestX5Chain(originalC2PA, 0)
	if err != nil {
		return nil, fmt.Errorf("extract original signing chain: %w", err)
	}
	originalCtx := WithSigningChain(WithLifecycleAuthority(ctx, originalAuthority), originalChain)
	freshOriginal, err := CompilePDF(originalCtx, oldPayload, originalCompiledAt)
	if err != nil {
		return nil, fmt.Errorf("recompile original payload: %w", err)
	}
	// boundary is the compiled PDF possibly followed by append-only PAdES
	// signature updates. PAdES appends bytes after %%EOF without altering the
	// preceding bytes, so the compiled output must be a byte-for-byte prefix.
	if !bytes.HasPrefix(ZeroCOSESignatures(boundary), ZeroCOSESignatures(freshOriginal)) {
		return freshOriginal, fmt.Errorf("original PDF prefix does not match deterministic recompilation from its embedded payload")
	}

	for hop := 1; hop <= len(offsets); hop++ {
		hopEnd := pdf
		if hop < len(offsets) {
			hopEnd = pdf[:offsets[hop]]
		}

		newPayload, err := ExtractLatestEmbeddedJSONLD(hopEnd)
		if err != nil {
			return nil, fmt.Errorf("extract payload for update %d: %w", hop, err)
		}
		hopC2PA, err := extractEmbeddedStreamByFileSpecName(hopEnd, "content_credential.c2pa")
		if err != nil {
			return nil, fmt.Errorf("extract C2PA for update %d: %w", hop, err)
		}
		updateCompiledAt, err := extractLifecycleEffectiveAt(hopC2PA, hop)
		if err != nil {
			return nil, fmt.Errorf("extract lifecycle timestamp for update %d: %w", hop, err)
		}
		hopAuthority, err := extractLifecycleAuthority(hopC2PA, hop)
		if err != nil {
			return nil, fmt.Errorf("extract lifecycle authority for update %d: %w", hop, err)
		}
		hopChain, err := extractManifestX5Chain(hopC2PA, hop)
		if err != nil {
			return nil, fmt.Errorf("extract signing chain for update %d: %w", hop, err)
		}
		hopCtx := WithSigningChain(WithLifecycleAuthority(ctx, hopAuthority), hopChain)

		// Re-apply this hop's amendment to the bytes preceding it (which may
		// themselves embed a PAdES signature or signing-evidence attachment —
		// opaque to this check, same as the base boundary above). Re-use the
		// VC and any remote-manifest provenance link already embedded for this
		// hop so the deterministic output is byte-for-byte identical:
		// ExtractEmbeddedVC / extractRemoteManifestURLFromXMP operate on hopEnd,
		// so they see exactly the attachment this specific hop wrote, not one
		// written by a later hop.
		embeddedVC, vcPresent, _ := ExtractEmbeddedVC(hopEnd)
		remoteManifestURL := extractRemoteManifestURLFromXMP(hopEnd)

		// Which kind of revision this hop is decides how it must be replayed.
		// ExtractEmbeddedVC sees the latest VC in hopEnd, which for a hop that
		// added none is the one an EARLIER hop wrote — so "this hop carries a
		// VC" means the attachment differs from the preceding bytes', not merely
		// that one is present.
		prevVC, prevVCPresent, _ := ExtractEmbeddedVC(boundary)
		hopAddedVC := vcPresent && len(embeddedVC) > 0 &&
			(!prevVCPresent || !bytes.Equal(prevVC, embeddedVC))

		// With no new VC and an unchanged payload the hop is a re-anchor
		// (ADR-26): provenance appended over a signature so the binding covers
		// the signed bytes. Replaying that as an amendment hits the no-changes
		// guard and fails to reproduce it.
		prevPayload, prevErr := ExtractLatestEmbeddedJSONLD(boundary)
		reanchorHop := !hopAddedVC && prevErr == nil && bytes.Equal(prevPayload, newPayload)

		var freshUpdated []byte
		switch {
		case hopAddedVC:
			freshUpdated, err = UpdatePDFWithOptions(hopCtx, boundary, newPayload, embeddedVC, remoteManifestURL, updateCompiledAt)
		case reanchorHop:
			freshUpdated, err = updatePDF(hopCtx, boundary, newPayload, nil, remoteManifestURL, updateCompiledAt, true)
		default:
			freshUpdated, err = UpdatePDFWithOptions(hopCtx, boundary, newPayload, nil, remoteManifestURL, updateCompiledAt)
		}
		if err != nil {
			return nil, fmt.Errorf("re-apply update %d: %w", hop, err)
		}
		if !bytes.HasPrefix(ZeroCOSESignatures(hopEnd), ZeroCOSESignatures(freshUpdated)) {
			return freshUpdated, fmt.Errorf("amended PDF does not match deterministic re-application of update %d", hop)
		}

		boundary = hopEnd
		reproduced = freshUpdated
	}
	return reproduced, nil
}

// pdfPagesRefRE matches a Catalog's /Pages reference.
var pdfPagesRefRE = regexp.MustCompile(`/Pages\s+(\d+)\s+0\s+R`)

// currentPagesObjID resolves the page tree through the document's current
// Catalog, falling back to the conventional object 2 only when no Catalog
// declares one.
func currentPagesObjID(pdf []byte) (int, error) {
	catalogID, ok := currentRootObjID(pdf)
	if ok {
		if start, end, found := lastObjectBody(pdf, catalogID); found {
			if m := pdfPagesRefRE.FindSubmatch(pdf[start:end]); m != nil {
				id, convErr := strconv.Atoi(string(m[1]))
				if convErr == nil {
					return id, nil
				}
			}
		}
	}
	return 2, nil
}
