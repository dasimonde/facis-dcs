package compiler

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// referencePayload is a self-contained JSON-LD payload used across determinism
// tests. It uses @vocab so all terms expand to dcs ontology IRIs and does not
// reference any external namespace URIs that would trigger HTTP fetches.
const referencePayload = `{
	"@context": {
		"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
		"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
	},
	"@id": "urn:doc:determinism-ref",
	"@type": "ContractTemplate",
	"metadata": {"@type": "TemplateMetadata", "title": "Determinism Reference Document"},
	"documentStructure": {
		"@type": "DocumentStructure",
		"layout": [
			{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:determinism-ref#s1", "urn:doc:determinism-ref#s2"]},
			{"@type": "LayoutNode", "@id": "urn:doc:determinism-ref#s1", "children": ["urn:doc:determinism-ref#c1"]},
			{"@type": "LayoutNode", "@id": "urn:doc:determinism-ref#s2", "children": ["urn:doc:determinism-ref#c2"]}
		],
		"blocks": [
			{"@type": "Section", "@id": "urn:doc:determinism-ref#s1", "title": "1. Introduction"},
			{"@type": "Clause", "@id": "urn:doc:determinism-ref#c1", "content": ["This is the first clause of the introduction section."]},
			{"@type": "Section", "@id": "urn:doc:determinism-ref#s2", "title": "2. Background"},
			{"@type": "Clause", "@id": "urn:doc:determinism-ref#c2", "content": ["Background material is provided in this section."]}
		]
	}
}`

// extractBTETBlocks returns all BT...ET text-rendering blocks from pdf in
// document order. These operators bound all human-visible text in a PDF content
// stream and are the canonical definition of "human-readable page content" for
// the purposes of the determinism guarantee.
func extractBTETBlocks(pdf []byte) [][]byte {
	var blocks [][]byte
	rest := pdf
	for {
		start := bytes.Index(rest, []byte("BT\n"))
		if start < 0 {
			break
		}
		rest = rest[start:]
		end := bytes.Index(rest, []byte("\nET\n"))
		if end < 0 {
			break
		}
		blocks = append(blocks, append([]byte(nil), rest[:end+4]...))
		rest = rest[end+4:]
	}
	return blocks
}

func btBlocksEqual(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !bytes.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}

// TestDeterministicFullPDFSamePayload is the strongest determinism check:
// rendering the same documentModel 10 times must produce byte-for-byte identical
// PDFs. A failure here indicates non-determinism in layout, PDF serialisation,
// object ID assignment, or the C2PA fixed-point iteration.
func TestDeterministicFullPDFSamePayload(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{Heading: "1. Alpha", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "First clause."}}},
		}},
		{Heading: "2. Beta", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "Second clause."}}},
		}},
	})
	first, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}
	for i := 2; i <= 10; i++ {
		got, err := renderPDF(testSigningContext(), doc)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, got) {
			t.Fatalf("iteration %d: PDF bytes differ — full PDF determinism violated", i)
		}
	}
}

// TestDeterministicPageContentSamePayload checks that the human-visible portion
// (BT/ET text blocks) is stable across 10 compilations even if other parts of
// the PDF binary were to differ.
func TestDeterministicPageContentSamePayload(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{Heading: "Section One", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "Clause content here."}}},
		}},
	})
	_pdf0, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}
	first := extractBTETBlocks(_pdf0)
	if len(first) == 0 {
		t.Fatal("no BT/ET blocks found in PDF — test setup error")
	}
	for i := 2; i <= 10; i++ {
		_pdfI, err := renderPDF(testSigningContext(), doc)
		if err != nil {
			t.Fatal(err)
		}
		got := extractBTETBlocks(_pdfI)
		if !btBlocksEqual(first, got) {
			t.Fatalf("iteration %d: page content streams differ — rendering determinism violated", i)
		}
	}
}

// concatBTBlocks concatenates all BT/ET text-rendering blocks from pdf into a
// single byte slice. Searching within this slice avoids false matches on
// PostScript glyph names in the embedded font's binary data.
func concatBTBlocks(pdf []byte) []byte {
	var out []byte
	for _, b := range extractBTETBlocks(pdf) {
		out = append(out, b...)
	}
	return out
}

// TestDeterministicSectionOrder compiles a document with 5 sections in a specific
// order and asserts the headings appear in that exact order in the page content
// streams. This guards against any map-iteration-based reordering of sections.
// Searching is restricted to BT/ET blocks to avoid false matches in the embedded
// font's PostScript glyph name table.
func TestDeterministicSectionOrder(t *testing.T) {
	headings := []string{"Heading-One", "Heading-Two", "Heading-Three", "Heading-Four", "Heading-Five"}
	sections := make([]sectionData, len(headings))
	for i, h := range headings {
		sections[i] = sectionData{
			Heading: h,
			Clauses: []clauseData{{Segments: []clauseSegment{{Type: "prose", Text: "Content."}}}},
		}
	}
	_pdf, err := renderPDF(testSigningContext(), sectionDoc(sections))
	if err != nil {
		t.Fatal(err)
	}
	content := concatBTBlocks(_pdf)

	positions := make([]int, len(headings))
	for i, h := range headings {
		pos := bytes.Index(content, []byte(h))
		if pos < 0 {
			t.Fatalf("heading %q not found in page content streams", h)
		}
		positions[i] = pos
	}
	for i := 1; i < len(positions); i++ {
		if positions[i] <= positions[i-1] {
			t.Errorf("heading %q (offset %d) does not follow %q (offset %d) — section order violated",
				headings[i], positions[i], headings[i-1], positions[i-1])
		}
	}
}

// TestDeterministicClauseOrder compiles a single section with 6 distinct clauses
// whose labels are deliberately out of alphabetical order, then verifies they
// appear in submitted order in the page content streams.
func TestDeterministicClauseOrder(t *testing.T) {
	// Deliberately out of alphabetical order to catch any sorting.
	texts := []string{
		"ClauseZeta", "ClauseAlpha", "ClauseMu",
		"ClauseTheta", "ClauseOmega", "ClauseGamma",
	}
	clauses := make([]clauseData, len(texts))
	for i, text := range texts {
		clauses[i] = clauseData{Segments: []clauseSegment{{Type: "prose", Text: text}}}
	}
	_pdf, err := renderPDF(testSigningContext(), sectionDoc([]sectionData{{Heading: "Section", Clauses: clauses}}))
	if err != nil {
		t.Fatal(err)
	}
	content := concatBTBlocks(_pdf)

	positions := make([]int, len(texts))
	for i, text := range texts {
		pos := bytes.Index(content, []byte(text))
		if pos < 0 {
			t.Fatalf("clause text %q not found in page content streams", text)
		}
		positions[i] = pos
	}
	for i := 1; i < len(positions); i++ {
		if positions[i] <= positions[i-1] {
			t.Errorf("clause %q (offset %d) does not follow %q (offset %d) — clause order violated",
				texts[i], positions[i], texts[i-1], positions[i-1])
		}
	}
}

// TestDeterministicPayloadHashStable verifies that the same JSON-LD input always
// produces the same URDNA2015 N-Quads byte stream across 10 calls. The N-Quads
// hash is the payload's canonical identity and must be stable.
func TestDeterministicPayloadHashStable(t *testing.T) {
	payload := []byte(referencePayload)
	nquads0, err := NormalizePayload(payload)
	if err != nil {
		t.Fatalf("NormalizePayload: %v", err)
	}
	for i := 2; i <= 10; i++ {
		nquads, err := NormalizePayload(payload)
		if err != nil {
			t.Fatalf("iteration %d: NormalizePayload: %v", i, err)
		}
		if !bytes.Equal(nquads0, nquads) {
			t.Fatalf("iteration %d: URDNA2015 N-Quads differ — hash non-deterministic", i)
		}
	}
}

// TestDeterministicLineWrapStable compiles a document with a clause long enough
// to force word-wrap across multiple lines and verifies the wrapped output is
// identical across two compilations.
func TestDeterministicLineWrapStable(t *testing.T) {
	longText := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 10)
	doc := sectionDoc([]sectionData{{
		Heading: "Wrap Test Section",
		Clauses: []clauseData{{Segments: []clauseSegment{{Type: "prose", Text: longText}}}},
	}})
	pdf1, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}
	pdf2, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(pdf1, pdf2) {
		t.Fatal("PDF bytes differ across two compilations — line wrap is non-deterministic")
	}
	// Confirm that wrapping actually occurred (at least 3 BT/ET blocks: title + heading + ≥1 wrapped line pair).
	blocks := extractBTETBlocks(pdf1)
	if len(blocks) < 3 {
		t.Errorf("expected at least 3 BT/ET blocks for a wrapped clause, got %d", len(blocks))
	}
}

// TestReRenderingAfterRoundTrip is the re-rendering guarantee in unit-test form:
// compile payload → extract embedded JSON-LD → recompile → page content must be
// byte-for-byte identical to the original. This proves that the embedded
// attachment is sufficient to reproduce the exact human-readable output.
func TestReRenderingAfterRoundTrip(t *testing.T) {
	pdf1, err := CompilePDF(testSigningContext(), []byte(referencePayload), time.Now())
	if err != nil {
		t.Fatalf("first CompilePDF: %v", err)
	}

	extracted, err := ExtractEmbeddedJSONLD(pdf1)
	if err != nil {
		t.Fatalf("ExtractEmbeddedJSONLD: %v", err)
	}

	pdf2, err := CompilePDF(testSigningContext(), extracted, time.Now())
	if err != nil {
		t.Fatalf("second CompilePDF (from extracted JSON-LD): %v", err)
	}

	blocks1 := extractBTETBlocks(pdf1)
	blocks2 := extractBTETBlocks(pdf2)
	if len(blocks1) == 0 {
		t.Fatal("no BT/ET blocks in first PDF — test setup error")
	}
	if !btBlocksEqual(blocks1, blocks2) {
		t.Fatalf("re-rendered PDF has different page content (%d blocks vs %d blocks) — re-rendering guarantee violated",
			len(blocks1), len(blocks2))
	}
}

// TestCompactIRIDeterministicWithMultiplePrefixes verifies that compactIRI
// returns a consistent result when multiple namespace prefixes map to the same
// base URI. Without deterministic iteration (e.g. sorted keys), Go's random map
// traversal would make this non-deterministic across calls.
func TestCompactIRIDeterministicWithMultiplePrefixes(t *testing.T) {
	nsMap := map[string]string{
		"zzz": "http://example.org/ns/",
		"aaa": "http://example.org/ns/",
		"mmm": "http://example.org/ns/",
	}
	var results []string
	for i := 0; i < 200; i++ {
		results = append(results, compactIRI("http://example.org/ns/Thing", nsMap))
	}
	// All results must be identical.
	for i, r := range results {
		if r != results[0] {
			t.Errorf("compactIRI returned %q at iteration %d but %q at iteration 0 — non-deterministic",
				r, i, results[0])
			return
		}
	}
	// The result must actually be a compact form, not the raw IRI.
	if results[0] == "http://example.org/ns/Thing" {
		t.Error("compactIRI did not compact the IRI — prefix matching failed")
	}
}
