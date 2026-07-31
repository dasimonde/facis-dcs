package compiler

import (
	"bytes"
	"encoding/binary"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
)

// minimalDoc returns a documentModel suitable for PDF/A compliance tests.
func minimalDoc() documentModel {
	return documentModel{
		Title:         "pdfa-compliance-test",
		EmbeddedPayload: []byte(`{}`),
		PayloadHash:   strings.Repeat("0", 64),
		FileID:        strings.Repeat("0", 64),
		NamespaceMap:  map[string]string{},
	}
}

// minimalDocWithSigFields returns a doc that triggers AcroForm generation.
func minimalDocWithSigFields() documentModel {
	doc := minimalDoc()
	doc.SignatureFields = []sigFieldDef{{Name: "Sig1", Label: "Signer 1"}}
	return doc
}

// TestPDFHeaderBinaryComment verifies clause 6.1.2 of ISO 19005-3: the comment
// line immediately following the %PDF header must start with % and then contain
// at least four bytes each with a decimal value greater than 127. This marker
// signals to tools that the file may contain binary data.
func TestPDFHeaderBinaryComment(t *testing.T) {
	pdf, err := renderPDF(testSigningContext(), minimalDoc())
	if err != nil {
		t.Fatal(err)
	}

	// Find the end of the first line ("%PDF-1.7\n")
	firstNL := bytes.IndexByte(pdf, '\n')
	if firstNL < 0 {
		t.Fatal("no newline found after %PDF header")
	}
	// Second line starts at firstNL+1
	rest := pdf[firstNL+1:]
	secondNL := bytes.IndexByte(rest, '\n')
	if secondNL < 0 {
		t.Fatal("no second line found in PDF header")
	}
	secondLine := rest[:secondNL]

	if len(secondLine) < 5 || secondLine[0] != '%' {
		t.Fatalf("second header line %q does not start with '%%'", secondLine)
	}
	for i, b := range secondLine[1:5] {
		if b <= 127 {
			t.Errorf("header binary comment byte[%d] = %d, want > 127 (ISO 19005-3 clause 6.1.2)", i, b)
		}
	}
}

// TestAcroFormNoNeedAppearances verifies clause 6.4.1 of ISO 19005-3: the
// NeedAppearances flag of the interactive form dictionary shall either not be
// present or shall be false. veraPDF rejects any PDF where NeedAppearances is
// explicitly set to true.
func TestAcroFormNoNeedAppearances(t *testing.T) {
	// Use a doc with signature fields so the AcroForm object is emitted.
	pdf, err := renderPDF(testSigningContext(), minimalDocWithSigFields())
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Contains(pdf, []byte("/NeedAppearances true")) {
		t.Error("AcroForm contains /NeedAppearances true; must be absent or false (ISO 19005-3 clause 6.4.1)")
	}
}

// TestFontIsEmbedded verifies clause 6.2.11.4.1 of ISO 19005-3: the font
// programs for all fonts used for rendering shall be embedded within the file.
// A TrueType font requires /FontFile2 in its /FontDescriptor, and the
// /FontDescriptor must be referenced from the /Font object.
func TestFontIsEmbedded(t *testing.T) {
	pdf, err := renderPDF(testSigningContext(), minimalDoc())
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(pdf, []byte("/FontFile2")) {
		t.Error("PDF does not contain /FontFile2; font program must be embedded (ISO 19005-3 clause 6.2.11.4.1)")
	}
	if !bytes.Contains(pdf, []byte("/FontDescriptor")) {
		t.Error("PDF does not contain /FontDescriptor; required for embedded TrueType font")
	}
}

// TestXMPHasPDFAIdentification verifies clause 6.6.4 of ISO 19005-3: the
// PDF/A version and conformance level shall be specified using the PDF/A
// Identification extension schema (pdfaid:part and pdfaid:conformance).
func TestXMPHasPDFAIdentification(t *testing.T) {
	pdf, err := renderPDF(testSigningContext(), minimalDoc())
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(pdf, []byte(`pdfaid:part="3"`)) {
		t.Error(`XMP metadata missing pdfaid:part="3" (ISO 19005-3 clause 6.6.4)`)
	}
	if !bytes.Contains(pdf, []byte(`pdfaid:conformance="A"`)) {
		t.Error(`XMP metadata missing pdfaid:conformance="A" (ISO 19005-3 clause 6.6.4)`)
	}
}

// TestLinkAnnotationsHaveFKey verifies clause 6.3.2 of ISO 19005-3: except for
// Popup annotations, all annotation dictionaries shall contain the /F key.
// The test checks the raw output of renderAnnotationObject since that function
// produces the full annotation dictionary string.
func TestLinkAnnotationsHaveFKey(t *testing.T) {
	stubPages := []pageLayout{{ObjectID: 5}}
	internalAnnot := annotationRef{
		ObjectID:    99,
		Term:        "prov:Entity",
		Rect:        [4]float64{10, 20, 100, 30},
		DestPageIdx: 0,
		DestY:       700,
	}
	externalAnnot := annotationRef{
		ObjectID: 100,
		Term:     "prov:Entity",
		Rect:     [4]float64{10, 20, 100, 30},
		URI:      "http://www.w3.org/ns/prov#Entity",
	}

	for _, annot := range []annotationRef{internalAnnot, externalAnnot} {
		out := renderAnnotationObject(annot, stubPages)
		if !strings.Contains(out, "/F ") {
			t.Errorf("renderAnnotationObject(%+v) = %q; missing /F key (ISO 19005-3 clause 6.3.2)", annot, out)
		}
	}
}

// TestGlossaryURIArrowIsASCII verifies that the glossary URI prefix arrow
// written into the PDF content stream uses only ASCII bytes. Using a
// multi-byte UTF-8 arrow (→, U+2192) causes bytes 0xE2, 0x86, 0x92 to be
// interpreted as WinAnsiEncoding codes 226, 134, 146 which are outside the
// declared font FirstChar–LastChar range and fail ISO 19005-3:2012 clause
// 6.2.11.5 (glyph width consistency).
func TestGlossaryURIArrowIsASCII(t *testing.T) {
	doc := documentModel{
		Title: "arrow-test",
		Glossary: []glossaryTerm{{
			Term:       "prov:Entity",
			Definition: "An entity",
			TermURI:    "http://www.w3.org/ns/prov#Entity",
		}},
		NamespaceMap:  map[string]string{"prov": "http://www.w3.org/ns/prov#"},
		EmbeddedPayload: []byte(`{}`),
		PayloadHash:   strings.Repeat("0", 64),
		FileID:        strings.Repeat("0", 64),
	}

	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}

	// The PDF content must not contain UTF-8 multi-byte sequences. Scan for
	// any Tj string operand that contains bytes > 127.
	pos := 0
	for {
		tj := bytes.Index(pdf[pos:], []byte(") Tj"))
		if tj < 0 {
			break
		}
		absEnd := pos + tj
		// Walk backwards to find the opening '('
		start := bytes.LastIndexByte(pdf[:absEnd], '(')
		if start >= 0 {
			str := pdf[start+1 : absEnd]
			for _, b := range str {
				if b > 127 {
					t.Errorf("PDF content stream contains non-ASCII byte 0x%02x inside a Tj string; use ASCII-only text in content streams (ISO 19005-3 clause 6.2.11.5)", b)
					return
				}
			}
		}
		pos = absEnd + 1
	}
}

// TestVerificationAppendixHasIDAndValidXRefStream verifies that the incremental
// verification witness produced by AppendVerificationWitness:
//   - includes /ID in the XRef stream dictionary (ISO 19005-3:2012 clause 6.1.3)
//   - produces an XRef stream whose /Length matches the actual stream byte count
//     (ISO 19005-3:2012 clause 6.1.7.1)
func TestVerificationAppendixHasIDAndValidXRefStream(t *testing.T) {
	base, err := CompilePDF(testSigningContext(), []byte(minimalPayloadBase), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	result, err := AppendVerificationWitness(testSigningContext(), base, []byte(minimalPayloadBase))
	if err != nil {
		t.Fatalf("AppendVerificationWitness: %v", err)
	}
	appendix := result[len(base):]

	if !bytes.Contains(appendix, []byte("/ID [")) {
		t.Error("verification appendix XRef stream missing /ID key (ISO 19005-3 clause 6.1.3)")
	}

	// Find /Length N in the XRef stream dict and verify the stream data is N bytes.
	idx := bytes.Index(appendix, []byte("/Type /XRef"))
	if idx < 0 {
		t.Fatal("verification appendix does not contain /Type /XRef")
	}
	// Find /Length value in the surrounding dict
	lenIdx := bytes.Index(appendix[idx:], []byte("/Length "))
	if lenIdx < 0 {
		t.Fatal("XRef stream dict missing /Length key")
	}
	lenStart := idx + lenIdx + len("/Length ")
	lenEnd := bytes.IndexAny(appendix[lenStart:], " \n\r/>>")
	if lenEnd < 0 {
		t.Fatal("could not parse /Length value")
	}
	declaredLen, err := strconv.Atoi(string(appendix[lenStart : lenStart+lenEnd]))
	if err != nil {
		t.Fatalf("invalid /Length value: %v", err)
	}
	// Find stream data: after "stream\n", before "\nendstream"
	streamKeyword := bytes.Index(appendix[idx:], []byte("stream\n"))
	if streamKeyword < 0 {
		t.Fatal("XRef stream body not found")
	}
	dataStart := idx + streamKeyword + len("stream\n")
	dataEnd := bytes.Index(appendix[dataStart:], []byte("\nendstream"))
	if dataEnd < 0 {
		t.Fatal("XRef stream endstream not found")
	}
	if dataEnd != declaredLen {
		t.Errorf("XRef stream /Length=%d but actual stream data is %d bytes (ISO 19005-3 clause 6.1.7.1)", declaredLen, dataEnd)
	}
}

// TestXMPNoUnregisteredC2PANamespace verifies clauses 6.6.2.3.1 (testNumber 1
// and 2) of ISO 19005-3: all XMP properties must use predefined schemas or
// extension schemas with proper declarations. The c2pa namespace
// (http://c2pa.org/c2pa) is not predefined in XMP 2005 or ISO 19005; using it
// without a schema extension declaration causes veraPDF failures. C2PA data
// belongs in the binary JUMBF attachment, not in XMP.
func TestXMPNoUnregisteredC2PANamespace(t *testing.T) {
	pdf, err := renderPDF(testSigningContext(), minimalDoc())
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Contains(pdf, []byte("c2pa.org/c2pa")) {
		t.Error("XMP metadata references unregistered c2pa namespace (http://c2pa.org/c2pa); " +
			"use the binary JUMBF attachment for C2PA data (ISO 19005-3 clause 6.6.2.3.1)")
	}
}

// TestXMPWrappedInXpacket verifies that the XMP metadata stream is wrapped in
// xpacket processing instructions as required by the XMP specification and
// ISO 19005-3:2012. The absence of the xpacket wrapper is flagged by veraPDF
// as "XMP not included in 'xpacket'" (clause 6.6.3).
func TestXMPWrappedInXpacket(t *testing.T) {
	pdf, err := renderPDF(testSigningContext(), minimalDoc())
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(pdf, []byte("<?xpacket begin=")) {
		t.Error("XMP metadata missing opening xpacket processing instruction (ISO 19005-3 clause 6.6.3)")
	}
	if !bytes.Contains(pdf, []byte("<?xpacket end=")) {
		t.Error("XMP metadata missing closing xpacket processing instruction (ISO 19005-3 clause 6.6.3)")
	}
}

// TestXMPCreatorToolContainsRendererVersion verifies that the XMP CreatorTool
// property embeds the renderer version so consumers can identify which build
// produced the PDF without any out-of-band version registry.
func TestXMPCreatorToolContainsRendererVersion(t *testing.T) {
	pdf, err := renderPDF(testSigningContext(), minimalDoc())
	if err != nil {
		t.Fatal(err)
	}

	expected := []byte(`xmp:CreatorTool="DCS-PDF-CORE ` + RendererVersion + `"`)
	if !bytes.Contains(pdf, expected) {
		t.Errorf("XMP metadata missing renderer version in CreatorTool: want %q present in XMP", string(expected))
	}
}

// TestEmbeddedFileParamsHasModDate verifies that the embedded JSON-LD file
// stream's /Params dictionary contains a /ModDate entry, as required by
// ISO 19005-3:2012 clause 6.4.7 (Table 4). veraPDF flags the absence of
// /ModDate as "Embedded file Params has no ModDate entry".
func TestEmbeddedFileParamsHasModDate(t *testing.T) {
	pdf, err := renderPDF(testSigningContext(), minimalDoc())
	if err != nil {
		t.Fatal(err)
	}

	// The embedded file stream dict contains /Params << ... >>. Verify
	// /ModDate appears inside it.
	paramsIdx := bytes.Index(pdf, []byte("/Params <<"))
	if paramsIdx < 0 {
		t.Fatal("PDF does not contain an embedded file /Params dictionary")
	}
	paramsEnd := bytes.Index(pdf[paramsIdx:], []byte(">>"))
	if paramsEnd < 0 {
		t.Fatal("could not find closing >> for /Params dictionary")
	}
	paramsBlock := pdf[paramsIdx : paramsIdx+paramsEnd+2]
	if !bytes.Contains(paramsBlock, []byte("/ModDate")) {
		t.Error("embedded file /Params dictionary is missing /ModDate entry (ISO 19005-3 clause 6.4.7)")
	}
}

// TestC2PAExclusionsWithinFileBounds verifies that every exclusion recorded in
// the c2pa.hash.data assertion covers bytes that actually exist within the
// compiled PDF. The C2PA spec does not permit exclusion ranges that extend
// beyond the asset boundary; such ranges are flagged by validators as
// "extra data hash exclusions found" and cause hard-binding hash failures.
func TestC2PAExclusionsWithinFileBounds(t *testing.T) {
	pdf, err := renderPDF(testSigningContext(), minimalDoc())
	if err != nil {
		t.Fatal(err)
	}

	// The exclusion ranges are serialised as CBOR inside the C2PA JUMBF
	// stream. Look for the CBOR unsigned-integer encoding of the PDF length.
	// A tail exclusion at Start=len(pdf) would encode len(pdf) as a CBOR
	// uint; we detect this as evidence that such an exclusion exists.
	//
	// More directly: the PDF must not contain the CBOR encoding of a
	// {start, length} pair whose start >= len(pdf).  We check that no
	// exclusion start can equal the exact file size, which is the sentinel
	// value written by buildC2PAExclusions for the tail exclusion.
	pdfLen := len(pdf)

	// Encode pdfLen as CBOR unsigned int (1, 2, or 4 bytes for typical sizes)
	cborPDFLen := cborUint(pdfLen)
	if bytes.Contains(pdf, cborPDFLen) {
		// Check if this integer appears as the "start" field in an exclusion
		// map. The CBOR map entry for "start" is: 65 73 74 61 72 74 ("start"
		// as CBOR text), followed immediately by the uint value.
		startKey := append([]byte{0x65, 0x73, 0x74, 0x61, 0x72, 0x74}, cborPDFLen...) // "start" text + value
		if bytes.Contains(pdf, startKey) {
			t.Errorf("C2PA exclusion has start=%d which equals the PDF file length — tail exclusion beyond EOF is invalid per C2PA spec", pdfLen)
		}
	}
}

// parseTTFWidths extracts advance widths (in PDF 1/1000-em units) for char
// codes [firstChar, lastChar] from an embedded TrueType font, using the
// WinAnsiEncoding mapping (chars 32-127 correspond to Unicode U+0020-U+007F).
func parseTTFWidths(t *testing.T, ttf []byte, firstChar, lastChar int) []int {
	t.Helper()

	if len(ttf) < 12 {
		t.Fatal("TTF too short to be valid")
	}

	u16 := func(b []byte, off int) int { return int(binary.BigEndian.Uint16(b[off:])) }
	u32 := func(b []byte, off int) int { return int(binary.BigEndian.Uint32(b[off:])) }

	numTables := u16(ttf, 4)
	findTable := func(name string) []byte {
		t.Helper()
		for i := 0; i < numTables; i++ {
			rec := ttf[12+i*16:]
			if string(rec[:4]) == name {
				off := u32(rec, 8)
				ln := u32(rec, 12)
				return ttf[off : off+ln]
			}
		}
		t.Fatalf("TTF table %q not found", name)
		return nil
	}

	// unitsPerEm is at offset 18 in the head table
	head := findTable("head")
	unitsPerEm := u16(head, 18)

	// numberOfHMetrics is at offset 34 in the hhea table
	hhea := findTable("hhea")
	numberOfHMetrics := u16(hhea, 34)

	// hmtx: each hMetric is 4 bytes (advanceWidth uint16, lsb int16)
	hmtx := findTable("hmtx")
	advanceWidth := func(glyphID int) int {
		if glyphID < numberOfHMetrics {
			return u16(hmtx, glyphID*4)
		}
		return u16(hmtx, (numberOfHMetrics-1)*4)
	}

	// cmap: find Platform=3, Encoding=1 (Windows Unicode BMP), Format=4
	cmap := findTable("cmap")
	numSubtables := u16(cmap, 2)
	var sub []byte
	for i := 0; i < numSubtables; i++ {
		rec := cmap[4+i*8:]
		platformID := u16(rec, 0)
		encodingID := u16(rec, 2)
		subOff := u32(rec, 4)
		if platformID == 3 && encodingID == 1 {
			sub = cmap[subOff:]
			break
		}
	}
	if sub == nil {
		// Fallback: Platform=0 (Unicode)
		for i := 0; i < numSubtables; i++ {
			rec := cmap[4+i*8:]
			if u16(rec, 0) == 0 {
				sub = cmap[u32(rec, 4):]
				break
			}
		}
	}
	if sub == nil {
		t.Fatal("no suitable cmap subtable found in TTF")
	}
	if u16(sub, 0) != 4 {
		t.Fatalf("cmap subtable format=%d, want 4", u16(sub, 0))
	}

	// Format-4 glyph lookup
	segCount := u16(sub, 6) / 2
	endOff := 14
	startOff := endOff + segCount*2 + 2 // +2 for reservedPad
	deltaOff := startOff + segCount*2
	rangeOff := deltaOff + segCount*2

	glyphForChar := func(c int) int {
		for i := 0; i < segCount; i++ {
			end := u16(sub, endOff+i*2)
			if c > end {
				continue
			}
			start := u16(sub, startOff+i*2)
			if c < start {
				return 0 // .notdef
			}
			delta := int(int16(binary.BigEndian.Uint16(sub[deltaOff+i*2:])))
			rangeOffset := u16(sub, rangeOff+i*2)
			if rangeOffset == 0 {
				return (c + delta) & 0xFFFF
			}
			// rangeOffset is relative to &idRangeOffset[i]
			pos := rangeOff + i*2 + rangeOffset + (c-start)*2
			if pos+2 > len(sub) {
				return 0
			}
			gid := u16(sub, pos)
			if gid == 0 {
				return 0
			}
			return (gid + delta) & 0xFFFF
		}
		return 0
	}

	count := lastChar - firstChar + 1
	result := make([]int, count)
	for i := 0; i < count; i++ {
		gid := glyphForChar(firstChar + i)
		aw := advanceWidth(gid)
		result[i] = int(math.Round(float64(aw) * 1000.0 / float64(unitsPerEm)))
	}
	return result
}

// TestFontWidthsConsistentWithEmbeddedTTF verifies ISO 19005-3:2012 clause
// 6.2.11.5: the widths declared in the /Widths array of the font dictionary
// must be consistent with the advance widths in the embedded font program.
// veraPDF flags mismatches as "Widths in embedded font are inconsistent with
// /Widths entry in the font dictionary".
func TestFontWidthsConsistentWithEmbeddedTTF(t *testing.T) {
	// Extract actual advance widths from the embedded Liberation Sans TTF.
	ttfWidths := parseTTFWidths(t, liberationSansTTF, 32, 127)

	// Extract the /Widths array from a compiled PDF.
	pdf, err := renderPDF(testSigningContext(), minimalDoc())
	if err != nil {
		t.Fatal(err)
	}
	widthsStart := bytes.Index(pdf, []byte("/Widths ["))
	if widthsStart < 0 {
		t.Fatal("PDF does not contain /Widths array in font dictionary")
	}
	widthsEnd := bytes.Index(pdf[widthsStart:], []byte("]"))
	if widthsEnd < 0 {
		t.Fatal("could not find closing ] for /Widths array")
	}
	raw := string(pdf[widthsStart+len("/Widths [") : widthsStart+widthsEnd])

	pdfWidths := make([]int, 0, 96)
	for _, tok := range strings.Fields(raw) {
		v, err := strconv.Atoi(tok)
		if err != nil {
			t.Fatalf("non-integer token %q in /Widths array", tok)
		}
		pdfWidths = append(pdfWidths, v)
	}

	if len(pdfWidths) != len(ttfWidths) {
		t.Fatalf("/Widths has %d entries, TTF has %d entries for chars 32-127", len(pdfWidths), len(ttfWidths))
	}

	for i, charCode := 0, 32; charCode <= 127; charCode, i = charCode+1, i+1 {
		if pdfWidths[i] != ttfWidths[i] {
			t.Errorf("char %d (%q): /Widths=%d, TTF actual=%d",
				charCode, string(rune(charCode)), pdfWidths[i], ttfWidths[i])
		}
	}
}
