package compiler

import (
	_ "embed"

	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// liberationSansTTF is the Liberation Sans Regular font program (SIL Open Font
// License). It is metrically compatible with Helvetica/Arial and is embedded
// in every generated PDF to satisfy ISO 19005-3:2012 clause 6.2.11.4.1 which
// requires that all font programs used for rendering be embedded in the file.
//
//go:embed testdata/fonts/LiberationSans-Regular.ttf
var liberationSansTTF []byte

// CanonicalCompiledAt is the fixed render epoch stamped as every compiled PDF's
// lifecycle effective_at, keeping compilation deterministic (same payload →
// byte-identical output). It matches the PDF's hardcoded metadata dates
// (2026-06-04); the trusted contract time is the PAdES B-T timestamp applied at
// signing, not this render epoch.
var CanonicalCompiledAt = time.Date(2026, 6, 4, 0, 0, 0, 0, time.UTC)

func CompilePDF(ctx context.Context, payload []byte, compiledAt time.Time) ([]byte, error) {
	// The FileID and the rendered "Payload hash" backlink are the sha256 of the
	// EXACT verbatim embedded bytes — a content-address any verifier recomputes
	// from the attachment. Under the verbatim boundary the embed bytes are fixed,
	// so A's render and B's recompile derive the identical value. (URDNA2015 graph
	// hashing is NOT used for this: blank-node labeling is not byte-deterministic,
	// which made the self-referential hash diverge A↔B.) The render model is
	// extracted directly from the payload's @list-ordered documentStructure.
	sum := sha256.Sum256(payload)
	hashHex := hex.EncodeToString(sum[:])

	doc, err := extractDocumentModel(payload, hashHex)
	if err != nil {
		return nil, err
	}
	// Carry the attachment verbatim: the exact submitted bytes are embedded, never
	// a re-canonicalized form. The visible render is fully determined by
	// documentStructure (already parsed into doc), so verify still reproduces it.
	doc.EmbeddedPayload = payload
	doc.PayloadCID = payloadCID(payload)
	doc.CompiledAt = compiledAt
	return renderPDF(ctx, doc)
}

// ExtractLifecycleEffectiveAt extracts the effective_at timestamp from the
// dcs.lifecycle assertion in the first (original compile) manifest of pdf.
// Used by /verify to re-compile with the same timestamp for byte equality.
func ExtractLifecycleEffectiveAt(pdf []byte) (time.Time, error) {
	c2paBytes, err := extractEmbeddedStreamByFileSpecName(pdf, "content_credential.c2pa")
	if err != nil {
		return time.Time{}, fmt.Errorf("extract C2PA: %w", err)
	}
	return extractLifecycleEffectiveAt(c2paBytes, 0)
}

// ExtractLifecycleAuthority returns the DID recorded as the authority of the
// PDF's genesis lifecycle assertion, or "" when the document names none.
func ExtractLifecycleAuthority(pdf []byte) (string, error) {
	c2paBytes, err := extractEmbeddedStreamByFileSpecName(pdf, "content_credential.c2pa")
	if err != nil {
		return "", fmt.Errorf("extract C2PA: %w", err)
	}
	return extractLifecycleAuthority(c2paBytes, 0)
}

func ExtractEmbeddedJSONLD(pdf []byte) ([]byte, error) {
	return extractJSONLDStream(pdf, false)
}

// ExtractLatestEmbeddedJSONLD returns the JSON-LD stream from the most recent
// definition of the embedded-file object. For an incrementally updated PDF
// this is the superseding object written by UpdatePDF; for a plain compiled PDF
// it is the same object returned by ExtractEmbeddedJSONLD.
func ExtractLatestEmbeddedJSONLD(pdf []byte) ([]byte, error) {
	return extractJSONLDStream(pdf, true)
}

// extractJSONLDStream finds the embedded contract.jsonld stream in a PDF.
// When lastOccurrence is true, the last (superseding) object definition is used,
// mirroring PDF cross-reference resolution rules for incremental updates.
func extractJSONLDStream(pdf []byte, lastOccurrence bool) ([]byte, error) {
	fileSpecPos := bytes.Index(pdf, []byte("/F (contract.jsonld)"))
	if fileSpecPos < 0 {
		return nil, fmt.Errorf("embedded JSON-LD filespec not found")
	}
	efPos := bytes.Index(pdf[fileSpecPos:], []byte("/EF << /F "))
	if efPos < 0 {
		return nil, fmt.Errorf("embedded JSON-LD object reference not found")
	}
	efPos += fileSpecPos + len("/EF << /F ")
	refEnd := bytes.Index(pdf[efPos:], []byte(" 0 R"))
	if refEnd < 0 {
		return nil, fmt.Errorf("embedded JSON-LD object reference malformed")
	}
	objID, err := strconv.Atoi(strings.TrimSpace(string(pdf[efPos : efPos+refEnd])))
	if err != nil {
		return nil, fmt.Errorf("embedded JSON-LD object id invalid: %w", err)
	}

	var streamStart, streamEnd int
	var ok bool
	if lastOccurrence {
		streamStart, streamEnd, ok = lastObjectStreamData(pdf, objID)
	} else {
		streamStart, streamEnd, ok = firstObjectStreamData(pdf, objID)
	}
	if !ok {
		return nil, fmt.Errorf("embedded JSON-LD stream not found in object %d", objID)
	}
	return append([]byte(nil), pdf[streamStart:streamEnd]...), nil
}

func AppendVerificationWitness(ctx context.Context, pdf []byte, payload []byte) ([]byte, error) {
	nquads, err := NormalizePayload(payload)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256(nquads)
	hashHex := hex.EncodeToString(hash[:])
	startxref, err := previousStartXref(pdf)
	if err != nil {
		return nil, err
	}
	originalC2PA, err := extractEmbeddedStreamByFileSpecName(pdf, "content_credential.c2pa")
	if err != nil {
		return nil, err
	}

	fileID := extractTrailerID(pdf)
	hardBindingHash := make([]byte, 32)
	exclusions := []c2paExclusion{}
	var candidate []byte
	for iteration := 0; iteration < 6; iteration++ {
		// A verification witness names no contract, so it carries no lifecycle
		// assertion at all — the status is unreachable, not merely unset.
		updatedC2PA, err := renderVerificationManifestStore(ctx, originalC2PA, witnessManifestLabel(hardBindingHash), "", hashHex, lifecycleStatusAmended, hardBindingHash, exclusions, CanonicalCompiledAt, "")
		if err != nil {
			return nil, err
		}

		appendix := buildVerificationAppendixBytes(len(pdf), startxref, hashHex, updatedC2PA, fileID)
		candidate = append(append([]byte(nil), pdf...), appendix...)

		streamStart, streamLen, found := findLastObjectStreamRange(candidate, 9)
		if !found {
			return candidate, nil
		}
		nextExclusions := buildC2PAExclusions(streamStart, streamLen)
		nextHash := sha256WithExclusions(candidate, nextExclusions)
		if bytes.Equal(hardBindingHash, nextHash[:]) && exclusionsEqual(exclusions, nextExclusions) {
			return candidate, nil
		}

		hardBindingHash = append([]byte(nil), nextHash[:]...)
		exclusions = nextExclusions
	}

	// All human-visible page content must remain within C2PA coverage after the
	// incremental witness update. Fail verification explicitly if this invariant
	// is violated rather than silently returning an unprovenanced PDF.
	if err := CheckPageContentC2PACoverage(candidate); err != nil {
		return nil, fmt.Errorf("C2PA coverage invariant violated after verification witness: %w", err)
	}

	return candidate, nil
}

// encodeXRefEntry encodes one XRef stream entry using /W [1 4 2]:
// 1-byte type (1=in-use), 4-byte big-endian offset, 2-byte big-endian generation.
func encodeXRefEntry(typ byte, offset int) []byte {
	return []byte{
		typ,
		byte(offset >> 24), byte(offset >> 16), byte(offset >> 8), byte(offset),
		0, 0,
	}
}

// buildVerificationAppendixBytes constructs the incremental update section
// appended to a PDF by the /verify endpoint. It supersedes obj 9 (the C2PA
// manifest) and adds two new objects:
//   - 9000: /Type /Sig  — carries the verification-witness hash payload
//   - 9001: /Type /XRef — cross-reference stream that also serves as the
//     incremental trailer, carrying /ID and /Prev per ISO 19005-3:2012 §6.1
//
// Using a cross-reference stream (rather than a traditional xref section) lets
// the /ID and metadata travel together and satisfies veraPDF's stream-length
// consistency check (clause 6.1.7.1).
func buildVerificationAppendixBytes(baseLen int, previousStartXref int, witnessHashHex string, c2paManifest []byte, fileID string) []byte {
	var appendix bytes.Buffer
	appendix.WriteString("\n% dcs-pdf-core incremental witness\n")

	c2paObjOffset := baseLen + appendix.Len()
	appendix.WriteString("9 0 obj\n")
	appendix.Write(streamObject(c2paManifest, fmt.Sprintf("<< /Type /EmbeddedFile /Subtype /application#2Fc2pa /Length %d >>", len(c2paManifest))))
	appendix.WriteString("\nendobj\n")

	witnessObjOffset := baseLen + appendix.Len()
	appendix.WriteString(fmt.Sprintf("9000 0 obj\n<< /Type /Sig /Name (verification-witness) /Reason (symmetric-rerender-match) /M (D:19700101000000Z) /Contents <%s> >>\nendobj\n", witnessHashHex))

	// Pre-record the offset of the XRef stream object so we can include it in
	// the stream data (self-referential cross-reference entry for obj 9001).
	xrefObjOffset := baseLen + appendix.Len()

	// Build the cross-reference stream data for /W [1 4 2] and
	// /Index [9 1 9000 2]: three entries covering objs 9, 9000, 9001.
	var xrefData bytes.Buffer
	xrefData.Write(encodeXRefEntry(1, c2paObjOffset))
	xrefData.Write(encodeXRefEntry(1, witnessObjOffset))
	xrefData.Write(encodeXRefEntry(1, xrefObjOffset))
	xrefBytes := xrefData.Bytes()

	idEntry := ""
	if fileID != "" {
		idEntry = " /ID " + fileID
	}
	xrefStreamDict := fmt.Sprintf(
		"<< /Type /XRef /W [1 4 2] /Index [9 1 9000 2] /Size 9002"+
			" /Root 1 0 R /Prev %d%s"+
			" /Info << /verification-witness (sha256:%s) >>"+
			" /Length %d >>",
		previousStartXref, idEntry, witnessHashHex, len(xrefBytes),
	)
	appendix.WriteString("9001 0 obj\n")
	appendix.Write(streamObject(xrefBytes, xrefStreamDict))
	appendix.WriteString("\nendobj\n")

	appendix.WriteString(fmt.Sprintf("startxref\n%d\n%%%%EOF\n", xrefObjOffset))
	return appendix.Bytes()
}

func extractEmbeddedStreamByFileSpecName(pdf []byte, fileName string) ([]byte, error) {
	needle := []byte(fmt.Sprintf("/F (%s)", fileName))
	fileSpecPos := bytes.Index(pdf, needle)
	if fileSpecPos < 0 {
		return nil, fmt.Errorf("filespec %s not found", fileName)
	}
	efPos := bytes.Index(pdf[fileSpecPos:], []byte("/EF << /F "))
	if efPos < 0 {
		return nil, fmt.Errorf("embedded stream reference for %s not found", fileName)
	}
	efPos += fileSpecPos + len("/EF << /F ")
	refEnd := bytes.Index(pdf[efPos:], []byte(" 0 R"))
	if refEnd < 0 {
		return nil, fmt.Errorf("embedded stream reference for %s malformed", fileName)
	}
	objID, err := strconv.Atoi(strings.TrimSpace(string(pdf[efPos : efPos+refEnd])))
	if err != nil {
		return nil, fmt.Errorf("embedded stream object id invalid for %s: %w", fileName, err)
	}
	streamStart, streamEnd, ok := lastObjectStreamData(pdf, objID)
	if !ok {
		return nil, fmt.Errorf("embedded stream not found in object %d for %s", objID, fileName)
	}
	return append([]byte(nil), pdf[streamStart:streamEnd]...), nil
}
