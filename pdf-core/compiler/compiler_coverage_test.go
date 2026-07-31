package compiler

import (
	"bytes"
	"testing"
	"time"
)

// TestPageContentIsFullyCoveredByC2PA compiles a reference document and verifies
// that no page content stream byte falls within a C2PA exclusion window. This is
// the machine-enforced proof that what the reader sees is what was signed.
func TestPageContentIsFullyCoveredByC2PA(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{Heading: "1. Coverage", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "All visible text must be provenanced."}}},
		}},
	})
	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckPageContentC2PACoverage(pdf); err != nil {
		t.Errorf("page content C2PA coverage check failed: %v", err)
	}
}

// TestExtractPageContentByteRanges verifies that the extractor returns non-empty,
// within-bounds ranges that correspond to page content streams (i.e. streams
// containing BT operators).
func TestExtractPageContentByteRanges(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{Heading: "Section", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "Visible text."}}},
		}},
	})
	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}

	ranges, err := ExtractPageContentByteRanges(pdf)
	if err != nil {
		t.Fatalf("ExtractPageContentByteRanges: %v", err)
	}
	if len(ranges) == 0 {
		t.Fatal("no page content byte ranges returned — test setup error or extractor broken")
	}
	for i, r := range ranges {
		if r[0] < 0 || r[1] > len(pdf) || r[0] >= r[1] {
			t.Errorf("range %d [%d, %d) is invalid for PDF of length %d", i, r[0], r[1], len(pdf))
		}
		// The range must contain BT (confirms it is a content stream, not an embedded file).
		if !bytes.Contains(pdf[r[0]:r[1]], []byte("BT")) {
			t.Errorf("range %d [%d, %d) does not contain BT — not a content stream", i, r[0], r[1])
		}
	}
}

// TestCoverageFailsWhenExclusionOverlapsContent verifies that
// checkCoverageWithExclusions returns an error when given an exclusion window
// that overlaps a real page content stream. This is the negative-path test for
// the coverage enforcement function.
func TestCoverageFailsWhenExclusionOverlapsContent(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{Heading: "Test Section", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "Visible content."}}},
		}},
	})
	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}

	contentRanges, err := ExtractPageContentByteRanges(pdf)
	if err != nil || len(contentRanges) == 0 {
		t.Fatal("could not get content ranges for test setup")
	}

	// Build an exclusion that exactly covers the first page content stream.
	r := contentRanges[0]
	badExclusion := c2paExclusion{Start: r[0], Length: r[1] - r[0]}

	err = checkCoverageWithExclusions(pdf, []c2paExclusion{badExclusion})
	if err == nil {
		t.Error("expected error when exclusion overlaps page content, got nil")
	}
}

// TestCoverageRangesDoNotOverlapC2PAExclusion verifies that the page content
// byte ranges returned for a compiled PDF do not intersect with the actual C2PA
// exclusion window (the JUMBF manifest stream). This is the structural proof
// that the current compiler produces a correctly covered PDF.
func TestCoverageRangesDoNotOverlapC2PAExclusion(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{Heading: "Coverage Proof", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "This text is signed."}}},
		}},
	})
	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}

	streamStart, streamLen, found := findLastObjectStreamRange(pdf, 9) // c2paEmbeddedID = 9
	if !found {
		t.Fatal("C2PA manifest stream not found in compiled PDF")
	}
	exclusion := c2paExclusion{Start: streamStart, Length: streamLen}

	contentRanges, err := ExtractPageContentByteRanges(pdf)
	if err != nil || len(contentRanges) == 0 {
		t.Fatalf("ExtractPageContentByteRanges: %v (ranges: %v)", err, contentRanges)
	}
	for _, r := range contentRanges {
		if rangesOverlap(r[0], r[1], exclusion.Start, exclusion.Start+exclusion.Length) {
			t.Errorf("content range [%d, %d) overlaps C2PA exclusion [%d, %d)",
				r[0], r[1], exclusion.Start, exclusion.Start+exclusion.Length)
		}
	}
}

// TestCoverageHoldsAfterVerification compiles a PDF, appends a verification
// witness, and checks that all page content in the verified PDF is still covered
// by C2PA. The incremental update must not shift page content into an uncovered
// region.
func TestCoverageHoldsAfterVerification(t *testing.T) {
	payload := []byte(referencePayload)
	pdf, err := CompilePDF(testSigningContext(), payload, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	verified, err := AppendVerificationWitness(testSigningContext(), pdf, payload)
	if err != nil {
		t.Fatalf("AppendVerificationWitness: %v", err)
	}

	if err := CheckPageContentC2PACoverage(verified); err != nil {
		t.Errorf("C2PA coverage broken after verification: %v", err)
	}
}

// TestReRenderingStableAfterVerification extracts the embedded JSON-LD from
// a verified PDF, recompiles it, and asserts the page content streams are
// identical to the original. The verification witness must not corrupt the
// re-rendering guarantee.
func TestReRenderingStableAfterVerification(t *testing.T) {
	payload := []byte(referencePayload)
	pdf1, err := CompilePDF(testSigningContext(), payload, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}

	verified, err := AppendVerificationWitness(testSigningContext(), pdf1, payload)
	if err != nil {
		t.Fatalf("AppendVerificationWitness: %v", err)
	}

	extracted, err := ExtractLatestEmbeddedJSONLD(verified)
	if err != nil {
		t.Fatalf("ExtractLatestEmbeddedJSONLD from verified PDF: %v", err)
	}

	pdf2, err := CompilePDF(testSigningContext(), extracted, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF from extracted JSON-LD: %v", err)
	}

	blocks1 := extractBTETBlocks(pdf1)
	blocks2 := extractBTETBlocks(pdf2)
	if !btBlocksEqual(blocks1, blocks2) {
		t.Fatalf("re-render after verification has different page content (%d blocks vs %d) — re-rendering guarantee violated",
			len(blocks1), len(blocks2))
	}
}

// The C2PA manifest stream must never be counted as page content: it is the one
// permitted exclusion window, so counting it made it "overlap" its own window
// exactly and panicked /sign under load. It used to be classified by scanning
// the file for "stream\n" and testing each payload for "BT" — the manifest's
// binary JUMBF incidentally contains those bytes. Pages are now reached through
// the page tree, so nothing outside a page's /Contents can be classified as page
// content whatever its bytes say. Clause text asserts the same for the other
// half of that scan: a clause containing "endstream" no longer truncates a range.
func TestOnlyThePageTreesContentStreamsCountAsPageContent(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{Heading: "1. Markers", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "This clause writes endstream and BT into the page."}}},
		}},
	})
	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}

	ranges, err := ExtractPageContentByteRanges(pdf)
	if err != nil {
		t.Fatalf("ExtractPageContentByteRanges: %v", err)
	}
	streams, err := pageContentStreamRanges(pdf)
	if err != nil {
		t.Fatalf("pageContentStreamRanges: %v", err)
	}
	if len(ranges) != len(streams) {
		t.Fatalf("extracted %d ranges for %d pages", len(ranges), len(streams))
	}
	for i, r := range ranges {
		if r[0] != streams[i].start || r[1] != streams[i].end {
			t.Errorf("range %d is [%d,%d), but page %d's content stream is [%d,%d)",
				i, r[0], r[1], i, streams[i].start, streams[i].end)
		}
		if !bytes.Contains(pdf[r[0]:r[1]], []byte("endstream")) {
			t.Errorf("range %d stops short of the clause text that says \"endstream\"", i)
		}
	}

	manifestStart, manifestLen, found := findLastObjectStreamRange(pdf, 9)
	if !found {
		t.Fatal("the compiled PDF carries no C2PA manifest stream")
	}
	for i, r := range ranges {
		if rangesOverlap(r[0], r[1], manifestStart, manifestStart+manifestLen) {
			t.Errorf("page content range %d [%d,%d) covers the C2PA manifest stream [%d,%d)",
				i, r[0], r[1], manifestStart, manifestStart+manifestLen)
		}
	}
	if err := CheckPageContentC2PACoverage(pdf); err != nil {
		t.Errorf("coverage check must not flag the manifest stream against its own exclusion: %v", err)
	}
}
