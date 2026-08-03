package compiler

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
)

// pdfContentsRefRE matches a /Contents reference in a PDF page dictionary.
var pdfContentsRefRE = regexp.MustCompile(`/Contents (\d+) 0 R`)

// StripEmbeddedJSONLD returns a copy of pdf with the embedded JSON-LD stream
// content replaced by null bytes of the same length.  All object offsets remain
// unchanged so the resulting PDF is structurally valid.  This simulates what a
// mail client or document-management system does when it strips attachments.
func StripEmbeddedJSONLD(pdf []byte) ([]byte, error) {
	start, length, err := findEmbeddedJSONLDStreamRange(pdf)
	if err != nil {
		return nil, err
	}
	result := append([]byte(nil), pdf...)
	for i := start; i < start+length; i++ {
		result[i] = 0
	}
	return result, nil
}

// findEmbeddedJSONLDStreamRange returns the byte offset and length of the
// JSON-LD stream content inside the definition of the embedded-file object a
// reader resolves — the last one, since an incremental update supersedes an
// object by appending a new definition.
func findEmbeddedJSONLDStreamRange(pdf []byte) (start, length int, err error) {
	fileSpecPos := bytes.Index(pdf, []byte("/F (contract.jsonld)"))
	if fileSpecPos < 0 {
		return 0, 0, fmt.Errorf("embedded JSON-LD filespec not found")
	}
	efPos := bytes.Index(pdf[fileSpecPos:], []byte("/EF << /F "))
	if efPos < 0 {
		return 0, 0, fmt.Errorf("embedded JSON-LD object reference not found")
	}
	efPos += fileSpecPos + len("/EF << /F ")
	refEnd := bytes.Index(pdf[efPos:], []byte(" 0 R"))
	if refEnd < 0 {
		return 0, 0, fmt.Errorf("embedded JSON-LD object reference malformed")
	}
	objIDStr := string(pdf[efPos : efPos+refEnd])
	objID, err := strconv.Atoi(objIDStr)
	if err != nil {
		return 0, 0, fmt.Errorf("embedded JSON-LD object id invalid: %w", err)
	}

	streamStart, streamEnd, ok := lastObjectStreamData(pdf, objID)
	if !ok {
		return 0, 0, fmt.Errorf("embedded JSON-LD stream not found in object %d", objID)
	}
	return streamStart, streamEnd - streamStart, nil
}

// MatchPageContent verifies that the page content streams of candidate match
// those of reference byte-for-byte.  Both PDFs must have been produced by the
// dcs-pdf-core compiler.  Returns nil when all pages match, or an error
// describing the first mismatch.
func MatchPageContent(candidate, reference []byte) error {
	candStreams, err := extractPageContentStreams(candidate)
	if err != nil {
		return fmt.Errorf("extract candidate page content: %w", err)
	}
	refStreams, err := extractPageContentStreams(reference)
	if err != nil {
		return fmt.Errorf("extract reference page content: %w", err)
	}
	if len(candStreams) != len(refStreams) {
		return fmt.Errorf("page count mismatch: submitted PDF has %d pages, compiled PDF has %d",
			len(candStreams), len(refStreams))
	}
	for i := range refStreams {
		if !bytes.Equal(candStreams[i], refStreams[i]) {
			return fmt.Errorf("page %d content does not match compiled output %s", i+1, firstDiffSnippet(candStreams[i], refStreams[i]))
		}
	}
	return nil
}

// firstDiffSnippet locates the first differing byte between two page content
// streams and returns a short window of each side around it — a diagnostic for
// the verify path to pinpoint WHERE the human-readable render diverges.
func firstDiffSnippet(candidate, reference []byte) string {
	n := len(candidate)
	if len(reference) < n {
		n = len(reference)
	}
	diff := n
	for i := 0; i < n; i++ {
		if candidate[i] != reference[i] {
			diff = i
			break
		}
	}
	window := func(b []byte) string {
		lo := diff - 20
		if lo < 0 {
			lo = 0
		}
		hi := diff + 25
		if hi > len(b) {
			hi = len(b)
		}
		return string(b[lo:hi])
	}
	return fmt.Sprintf("(candidate len=%d reference len=%d; at byte %d: candidate=%q reference=%q)",
		len(candidate), len(reference), diff, window(candidate), window(reference))
}

// pageContentStream is one page's content stream: the object holding it and the
// half-open range of its raw data.
type pageContentStream struct {
	objID      int
	start, end int
}

// pageContentStreamRanges follows the PDF page tree of pdf, returning each
// page's content stream in document order. Every page is reached through the
// page tree and every stream's extent comes from its own /Length, so no keyword
// is ever sought inside stream data — see objectheader.go for what searching
// there costs.
func pageContentStreamRanges(pdf []byte) ([]pageContentStream, error) {
	pageIDs, err := parseCurrentPagesKids(pdf)
	if err != nil {
		return nil, err
	}
	streams := make([]pageContentStream, 0, len(pageIDs))
	for _, pageID := range pageIDs {
		dictStart, dictEnd, ok := lastObjectBody(pdf, pageID)
		if !ok {
			return nil, fmt.Errorf("page object %d not found", pageID)
		}
		m := pdfContentsRefRE.FindSubmatch(pdf[dictStart:dictEnd])
		if len(m) < 2 {
			return nil, fmt.Errorf("page object %d has no /Contents reference", pageID)
		}
		contentID, err := strconv.Atoi(string(m[1]))
		if err != nil {
			return nil, fmt.Errorf("page object %d /Contents ref invalid: %w", pageID, err)
		}
		start, end, ok := lastObjectStreamData(pdf, contentID)
		if !ok {
			return nil, fmt.Errorf("content stream %d: object %d has no stream", contentID, contentID)
		}
		streams = append(streams, pageContentStream{objID: contentID, start: start, end: end})
	}
	return streams, nil
}

// extractPageContentStreams returns the raw bytes of each page's content stream
// in document order.
func extractPageContentStreams(pdf []byte) ([][]byte, error) {
	ranges, err := pageContentStreamRanges(pdf)
	if err != nil {
		return nil, err
	}
	streams := make([][]byte, 0, len(ranges))
	for _, r := range ranges {
		streams = append(streams, append([]byte(nil), pdf[r.start:r.end]...))
	}
	return streams, nil
}

// extractStreamContentByObjID returns the raw stream data of the definition of
// the given object a reader resolves (see objectheader.go for what "resolves"
// costs to get right).
func extractStreamContentByObjID(pdf []byte, objID int) ([]byte, error) {
	header, found := lastObjectHeader(pdf, objID)
	if !found {
		return nil, fmt.Errorf("object %d not found", objID)
	}
	streamStart, streamEnd, ok := objectStreamData(pdf, header)
	if !ok {
		return nil, fmt.Errorf("object %d has no stream", objID)
	}
	return append([]byte(nil), pdf[streamStart:streamEnd]...), nil
}
