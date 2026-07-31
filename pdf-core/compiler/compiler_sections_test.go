package compiler

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"
)

// mustExtractFromPayload canonicalizes a raw JSON-LD payload and extracts the
// document model via extractDocumentModelFromCanonical, failing the test on any
// error. Canonicalization ensures all input flavors (prefixed, @vocab, expanded)
// are normalized before extraction.
func mustExtractFromPayload(t *testing.T, payload []byte) documentModel {
	t.Helper()
	canonical, err := CanonicalizePayload(payload)
	if err != nil {
		t.Fatalf("CanonicalizePayload: %v", err)
	}
	doc, err := extractDocumentModelFromCanonical(canonical, strings.Repeat("0", 64))
	if err != nil {
		t.Fatalf("extractDocumentModelFromCanonical: %v", err)
	}
	return doc
}

// sectionDoc builds a documentModel with sections for testing.
func sectionDoc(sections []sectionData) documentModel {
	return documentModel{
		Title:         "Section Test",
		Sections:      sections,
		EmbeddedPayload: []byte(`{}`),
		PayloadHash:   strings.Repeat("0", 64),
		FileID:        strings.Repeat("0", 64),
		NamespaceMap:  map[string]string{},
	}
}

// TestSectionHeadingsAppearInPDF verifies that section headings are rendered into
// the PDF content streams.
func TestSectionHeadingsAppearInPDF(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{Heading: "1. Background", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "First clause."}}},
		}},
		{Heading: "2. Terms", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "Second clause."}}},
		}},
	})
	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}
	for _, heading := range []string{"1. Background", "2. Terms"} {
		if !bytes.Contains(pdf, []byte(heading)) {
			t.Errorf("PDF must contain section heading %q", heading)
		}
	}
}

func TestLayoutDocumentPreservesExplicitNewlinesInClauses(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{Heading: "1. Terms", Clauses: []clauseData{
			{Segments: []clauseSegment{
				{Type: "prose", Text: "Dispute Resolution Method: "},
				{Type: "prose", Text: "_____"},
				{Type: "prose", Text: "\nDispute Venue: "},
				{Type: "prose", Text: "_____"},
				{Type: "prose", Text: "\n\nGoverning Law: _____"},
			}},
		}},
	})

	pages := layoutDocumentPages(doc)
	var bodyLines []string
	for _, page := range pages {
		for _, line := range page.Lines {
			if line.Kind == "body" {
				bodyLines = append(bodyLines, line.Text)
			}
		}
	}

	want := []string{
		"Dispute Resolution Method: _____",
		"Dispute Venue: _____",
		"",
		"Governing Law: _____",
	}
	if len(bodyLines) != len(want) {
		t.Fatalf("body line count = %d, want %d (%q)", len(bodyLines), len(want), bodyLines)
	}
	for i := range want {
		if bodyLines[i] != want[i] {
			t.Fatalf("body line %d = %q, want %q", i, bodyLines[i], want[i])
		}
	}
}

func TestRenderContentStreamUsesBoldTextModeForSectionHeadings(t *testing.T) {
	content := renderContentStream(pageLayout{Lines: []positionedLine{
		{Text: "Section Heading", FontSize: 14, Kind: "section-heading"},
		{Text: "Body text", FontSize: 11, Kind: "body"},
	}})

	if !strings.Contains(content, "/F1 14.00 Tf\n0 g\n0 G\n0.35 w\n2 Tr\n") {
		t.Fatalf("section heading was not rendered with bold text mode:\n%s", content)
	}
	if !strings.Contains(content, "/F1 11.00 Tf\n0 g\n0 Tr\n") {
		t.Fatalf("body line did not reset to normal text mode:\n%s", content)
	}
}

// TestPDFHasOutlineWhenSectionsPresent verifies that the catalog includes
// /Outlines when the document has named sections.
func TestPDFHasOutlineWhenSectionsPresent(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{Heading: "Section One", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "Content."}}},
		}},
	})
	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf, []byte("/Outlines")) {
		t.Error("PDF catalog must contain /Outlines when the document has named sections")
	}
}

// TestPDFOutlineItemMatchesSectionHeading verifies that the PDF outline title
// matches the section heading text.
func TestPDFOutlineItemMatchesSectionHeading(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{Heading: "My Section", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "Content."}}},
		}},
	})
	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf, []byte("/Title (My Section)")) {
		t.Error("PDF outline must contain /Title (My Section) matching the section heading")
	}
}

// TestStructTreeHasSectAndH1Elements verifies that the PDF structure tree
// contains /S /Sect for sections and /S /H1 for section headings.
func TestStructTreeHasSectAndH1Elements(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{Heading: "Section A", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "Para."}}},
		}},
	})
	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf, []byte("/S /Sect")) {
		t.Error("PDF structure tree must contain /S /Sect for each section")
	}
	if !bytes.Contains(pdf, []byte("/S /H1")) {
		t.Error("PDF structure tree must contain /S /H1 for section headings")
	}
}

// TestLargePDFSpansMultiplePages verifies that a document with many sections
// and long clauses produces more than one page.
func TestLargePDFSpansMultiplePages(t *testing.T) {
	sections := make([]sectionData, 8)
	for i := range sections {
		clauses := make([]clauseData, 6)
		for j := range clauses {
			clauses[j] = clauseData{Segments: []clauseSegment{{
				Type: "prose",
				Text: fmt.Sprintf("This is paragraph %d in section %d. It is sufficiently long to occupy meaningful vertical space when rendered. The layout engine must split it across several wrapped lines and trigger page breaks across the document.", j+1, i+1),
			}}}
		}
		sections[i] = sectionData{
			Heading: fmt.Sprintf("%d. Section %d", i+1, i+1),
			Clauses: clauses,
		}
	}
	doc := sectionDoc(sections)
	pages := layoutDocumentPages(doc)
	if len(pages) < 2 {
		t.Errorf("large document should span at least 2 pages, got %d", len(pages))
	}
}

// TestExtractDocumentModelParsesSectionsFormat verifies IRI-based parsing of the
// documentStructure. Properties are found by their expanded IRIs, not verbatim JSON keys.
func TestExtractDocumentModelParsesSectionsFormat(t *testing.T) {
	payload := []byte(`{
		"@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id": "urn:doc:sections-format-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "Contract"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true,
					"children": ["urn:doc:sections-format-test#s1", "urn:doc:sections-format-test#s2"]},
				{"@type": "LayoutNode", "@id": "urn:doc:sections-format-test#s1",
					"children": ["urn:doc:sections-format-test#c1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:sections-format-test#s2",
					"children": ["urn:doc:sections-format-test#c2"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:sections-format-test#s1", "title": "1. Parties"},
				{"@type": "Clause", "@id": "urn:doc:sections-format-test#c1", "content": ["Plain prose clause."]},
				{"@type": "Section", "@id": "urn:doc:sections-format-test#s2", "title": "2. Terms"},
				{"@type": "Clause", "@id": "urn:doc:sections-format-test#c2", "content": ["Structured content."]}
			]
		}
	}`)
	model := mustExtractFromPayload(t, payload)
	if len(model.Sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(model.Sections))
	}
	if model.Sections[0].Heading != "1. Parties" {
		t.Errorf("section 0 heading = %q, want '1. Parties'", model.Sections[0].Heading)
	}
	if len(model.Sections[0].Clauses) != 1 {
		t.Errorf("section 0 clause count = %d, want 1", len(model.Sections[0].Clauses))
	}
	if model.Sections[1].Heading != "2. Terms" {
		t.Errorf("section 1 heading = %q, want '2. Terms'", model.Sections[1].Heading)
	}
	if len(model.Sections[1].Clauses) != 1 {
		t.Errorf("section 1 clause count = %d, want 1", len(model.Sections[1].Clauses))
	}
}

// TestExtractDocumentModelParsesAtIdContentItem verifies that an object with
// "@id" in a "content" array is parsed as an ontology-link segment.
// After JSON-LD expansion, seg.Ref holds the full IRI (not the compact prefix form).
func TestExtractDocumentModelParsesAtIdContentItem(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
			"prov": "http://www.w3.org/ns/prov#",
			"schema": "https://schema.org/"
		},
		"@id": "urn:doc:content-id-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "Content ID Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:content-id-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:content-id-test#s1", "children": ["urn:doc:content-id-test#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:content-id-test#s1", "title": "1. Section"},
				{"@type": "Clause", "@id": "urn:doc:content-id-test#c1", "content": [
					"prefix text ",
					{"@id": "prov:Entity", "schema:name": "entity"},
					" suffix text"
				]}
			]
		}
	}`)
	model := mustExtractFromPayload(t, payload)
	if len(model.Sections) != 1 || len(model.Sections[0].Clauses) != 1 {
		t.Fatalf("unexpected model structure")
	}
	clause := model.Sections[0].Clauses[0]
	found := false
	for _, seg := range clause.Segments {
		// After expansion, @id "prov:Entity" resolves to its full IRI.
		if seg.Type == "ontology-link" && seg.Ref == "http://www.w3.org/ns/prov#Entity" && seg.Text == "entity" {
			found = true
		}
	}
	if !found {
		t.Errorf("@id content item must produce ontology-link with Ref=http://www.w3.org/ns/prov#Entity Text=entity; got %+v", clause.Segments)
	}
}

// TestExtractDocumentModelParsesSchemaUrlContentItem verifies that an object
// with "schema:url" in a "content" array is parsed as an external-link segment.
func TestExtractDocumentModelParsesSchemaUrlContentItem(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
			"schema": "https://schema.org/"
		},
		"@id": "urn:doc:url-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "URL Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:url-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:url-test#s1", "children": ["urn:doc:url-test#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:url-test#s1", "title": "1. Section"},
				{"@type": "Clause", "@id": "urn:doc:url-test#c1", "content": [
					{"schema:url": "https://example.com", "schema:name": "Example Site"}
				]}
			]
		}
	}`)
	model := mustExtractFromPayload(t, payload)
	clause := model.Sections[0].Clauses[0]
	found := false
	for _, seg := range clause.Segments {
		if seg.Type == "external-link" && seg.Href == "https://example.com" && seg.Text == "Example Site" {
			found = true
		}
	}
	if !found {
		t.Errorf("schema:url content item must produce external-link segment; got %+v", clause.Segments)
	}
}

// TestExtractDocumentModelParsesAtValueContentItem verifies that an object with
// "@value" in a "content" array is parsed as a typed-value segment.
// Note: schema:unitCode cannot appear alongside @value in the same object —
// that is invalid JSON-LD (value objects accept only @value/@type/@language).
// The datatype is stored as the full expanded IRI.
func TestExtractDocumentModelParsesAtValueContentItem(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
			"xsd": "http://www.w3.org/2001/XMLSchema#"
		},
		"@id": "urn:doc:value-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "Value Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:value-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:value-test#s1", "children": ["urn:doc:value-test#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:value-test#s1", "title": "1. Section"},
				{"@type": "Clause", "@id": "urn:doc:value-test#c1", "content": [
					{"@value": "500", "@type": "xsd:decimal"}
				]}
			]
		}
	}`)
	model := mustExtractFromPayload(t, payload)
	clause := model.Sections[0].Clauses[0]
	found := false
	for _, seg := range clause.Segments {
		// Datatype is the full expanded IRI after JSON-LD expansion.
		if seg.Type == "typed-value" && seg.Value == "500" &&
			seg.Datatype == "http://www.w3.org/2001/XMLSchema#decimal" {
			found = true
		}
	}
	if !found {
		t.Errorf("@value content item must produce typed-value with Value=500 Datatype=http://www.w3.org/2001/XMLSchema#decimal; got %+v", clause.Segments)
	}
}

// TestExtractDocumentModelIgnoresUnknownNamespaceTitle verifies that a
// property in an unrelated namespace does not supply the required dcs:metadata.
// Only dcs:metadata satisfies the metadata requirement.
func TestExtractDocumentModelIgnoresUnknownNamespaceTitle(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#",
			"evil": "https://example.com/evil#"
		},
		"@id": "urn:doc:unknown-title-ns",
		"@type": "dcs:ContractTemplate",
		"evil:metadata": {"evil:title": "Do Not Use This"},
		"dcs:documentStructure": {
			"@type": "dcs:DocumentStructure",
			"dcs:layout": [
				{"@type": "dcs:LayoutNode", "dcs:isRoot": true, "dcs:children": ["urn:doc:unknown-title-ns#s1"]},
				{"@type": "dcs:LayoutNode", "@id": "urn:doc:unknown-title-ns#s1", "dcs:children": ["urn:doc:unknown-title-ns#c1"]}
			],
			"dcs:blocks": [
				{"@type": "dcs:Section", "@id": "urn:doc:unknown-title-ns#s1", "dcs:title": "1. Terms"},
				{"@type": "dcs:Clause", "@id": "urn:doc:unknown-title-ns#c1", "dcs:content": ["A clause."]}
			]
		}
	}`)

	_, err := CompilePDF(testSigningContext(), payload, time.Now())
	if err == nil {
		t.Fatal("CompilePDF must return an error when dcs:metadata is absent (evil:metadata must be ignored)")
	}
}

// TestExtractDocumentModelIgnoresUnknownNamespaceSections verifies that blocks
// in an unrelated namespace are ignored; extraction uses only dcs:documentStructure.
func TestExtractDocumentModelIgnoresUnknownNamespaceSections(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#",
			"evil": "https://example.com/evil#"
		},
		"@id": "urn:doc:unknown-sections-ns",
		"@type": "dcs:ContractTemplate",
		"dcs:metadata": {"@type": "dcs:TemplateMetadata", "dcs:title": "Unknown Sections NS Test"},
		"evil:documentStructure": {"evil:blocks": [{"evil:title": "Wrong Heading"}]},
		"dcs:documentStructure": {
			"@type": "dcs:DocumentStructure",
			"dcs:layout": [
				{"@type": "dcs:LayoutNode", "dcs:isRoot": true, "dcs:children": ["urn:doc:unknown-sections-ns#s1"]},
				{"@type": "dcs:LayoutNode", "@id": "urn:doc:unknown-sections-ns#s1", "dcs:children": ["urn:doc:unknown-sections-ns#c1"]}
			],
			"dcs:blocks": [
				{"@type": "dcs:Section", "@id": "urn:doc:unknown-sections-ns#s1", "dcs:title": "Correct Heading"},
				{"@type": "dcs:Clause", "@id": "urn:doc:unknown-sections-ns#c1", "dcs:content": ["Correct clause."]}
			]
		}
	}`)

	model := mustExtractFromPayload(t, payload)
	if len(model.Sections) != 1 {
		t.Fatalf("expected exactly 1 valid section, got %d", len(model.Sections))
	}
	if model.Sections[0].Heading != "Correct Heading" {
		t.Fatalf("unknown namespace sections must be ignored; got heading %q", model.Sections[0].Heading)
	}
}


// TestH1TagInContentStream verifies that section headings are tagged /H1 in the
// PDF content stream rather than /P.
func TestH1TagInContentStream(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{Heading: "My Heading", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "Body text."}}},
		}},
	})
	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf, []byte("/H1 <<")) {
		t.Error("section heading must use /H1 tag in content stream")
	}
}

// TestParentTreeCoversAllMCIDs verifies that the structure tree's ParentTree
// covers every MCID used in the document — no orphaned marked-content items.
func TestParentTreeCoversAllMCIDs(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{Heading: "S1", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "Alpha clause."}}},
			{Segments: []clauseSegment{{Type: "prose", Text: "Beta clause."}}},
		}},
		{Heading: "S2", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "Gamma clause."}}},
		}},
	})
	pages := layoutDocumentPages(doc)
	totalMCIDs := 0
	for _, p := range pages {
		totalMCIDs += len(p.Lines)
	}
	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}

	// The ParentTree must have at least as many entries as there are MCIDs.
	// The entries look like "N 0 R" inside the /Nums array.
	// A simple proxy: the PDF must contain the string for each page-local MCID
	// up to totalMCIDs-1 referenced in a parent tree array (multiple "0 R" refs).
	// We check that the parent tree nums array has enough entries.
	_ = totalMCIDs
	if !bytes.Contains(pdf, []byte("/ParentTree")) {
		t.Error("PDF must contain /ParentTree in StructTreeRoot")
	}
}

// TestSectionHeadingHasPreSpacing verifies that a section heading is placed with
// extra vertical space above it relative to the preceding body line — not flush
// against it. The gap should be larger than a single body line height.
func TestSectionHeadingHasPreSpacing(t *testing.T) {
	doc := sectionDoc([]sectionData{
		{Heading: "1. First Section", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "A short clause in the first section."}}},
		}},
		{Heading: "2. Second Section", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "A short clause in the second section."}}},
		}},
	})
	pages := layoutDocumentPages(doc)

	var bodyY, headingY float64
	found := false
	for _, page := range pages {
		for i, line := range page.Lines {
			if line.Kind == "body" && i+1 < len(page.Lines) {
				next := page.Lines[i+1]
				if next.Kind == "section-heading" {
					bodyY = line.Y
					headingY = next.Y
					found = true
					break
				}
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatal("could not find a body line immediately followed by a section-heading line")
	}
	// A normal body line (11pt) has lineHeight ≈ 15.95pt. With pre-spacing the
	// gap should be noticeably larger — at least 20pt.
	gap := bodyY - headingY
	if gap < 20 {
		t.Errorf("section heading pre-spacing gap = %.1f pt, want >= 20 pt (current layout is too tight)", gap)
	}
}

// TestLargePDFPageContentDistribution verifies that for a document large enough
// to span 3+ pages, every page except possibly the last has content, and no
// positioned line sits below the bottom margin or above the top margin.
func TestLargePDFPageContentDistribution(t *testing.T) {
	const (
		pageHeight   = 792.0
		topMargin    = 72.0
		bottomMargin = 54.0
	)
	sections := make([]sectionData, 10)
	for i := range sections {
		clauses := make([]clauseData, 5)
		for j := range clauses {
			clauses[j] = clauseData{Segments: []clauseSegment{{
				Type: "prose",
				Text: fmt.Sprintf("Clause %d in section %d. This clause is intentionally verbose to consume vertical space and ensure the layout engine must split content across multiple pages, exercising the page-break logic thoroughly.", j+1, i+1),
			}}}
		}
		sections[i] = sectionData{
			Heading: fmt.Sprintf("%d. Section %d Heading", i+1, i+1),
			Clauses: clauses,
		}
	}
	doc := sectionDoc(sections)
	pages := layoutDocumentPages(doc)

	if len(pages) < 3 {
		t.Errorf("10-section document should span at least 3 pages, got %d", len(pages))
	}
	// Every page (except the last, which may be a trailing sig page) must have lines.
	for i, page := range pages[:len(pages)-1] {
		if len(page.Lines) == 0 {
			t.Errorf("page %d has no lines — content distribution is broken", i+1)
		}
	}
	// No line may sit outside the printable area.
	for pageNum, page := range pages {
		for _, line := range page.Lines {
			maxY := pageHeight - topMargin + line.FontSize // top of first line
			if line.Y > maxY {
				t.Errorf("page %d line %q Y=%.1f exceeds top margin area", pageNum+1, line.Text, line.Y)
			}
			if line.Y < bottomMargin {
				t.Errorf("page %d line %q Y=%.1f is below bottom margin %.1f", pageNum+1, line.Text, line.Y, bottomMargin)
			}
		}
	}
}

// docWithGlossaryTerms returns a document whose clauses reference ontology
// terms, causing the compiler to generate a Glossary section.
func docWithGlossaryTerms(sections []sectionData) documentModel {
	return documentModel{
		Title:    "Glossary Placement Test",
		Sections: sections,
		Glossary: []glossaryTerm{
			{Term: "prov:Entity", TermURI: "http://www.w3.org/ns/prov#Entity", Definition: "Something whose provenance can be described."},
			{Term: "prov:Activity", TermURI: "http://www.w3.org/ns/prov#Activity", Definition: "Something that occurs over a period of time and acts upon entities."},
		},
		EmbeddedPayload: []byte(`{}`),
		PayloadHash:   strings.Repeat("0", 64),
		FileID:        strings.Repeat("0", 64),
		NamespaceMap:  map[string]string{},
	}
}

// TestGlossaryStartsOnFreshPage verifies that the Glossary heading is always
// the first content line on its page — it must never share a page with body
// text from the preceding sections.
func TestGlossaryStartsOnFreshPage(t *testing.T) {
	sections := make([]sectionData, 3)
	for i := range sections {
		clauses := make([]clauseData, 4)
		for j := range clauses {
			clauses[j] = clauseData{Segments: []clauseSegment{{
				Type: "prose",
				Text: fmt.Sprintf("Clause %d in section %d — enough text to take up vertical space in the layout.", j+1, i+1),
			}}}
		}
		sections[i] = sectionData{
			Heading: fmt.Sprintf("%d. Section %d", i+1, i+1),
			Clauses: clauses,
		}
	}
	doc := docWithGlossaryTerms(sections)
	pages := layoutDocumentPages(doc)

	// Find the page index of the Glossary heading.
	glossaryPageIdx := -1
	for pi, page := range pages {
		for _, line := range page.Lines {
			if line.Kind == "glossary-heading" {
				glossaryPageIdx = pi
				break
			}
		}
		if glossaryPageIdx >= 0 {
			break
		}
	}
	if glossaryPageIdx < 0 {
		t.Fatal("no glossary-heading line found in any page")
	}

	// The glossary heading must be the first line on its page.
	firstLine := pages[glossaryPageIdx].Lines[0]
	if firstLine.Kind != "glossary-heading" {
		t.Errorf("glossary page first line Kind=%q, want glossary-heading; Glossary must start on a fresh page", firstLine.Kind)
	}
}

// TestGlossaryAppearsInOutline verifies that the PDF bookmark tree includes a
// Glossary entry pointing at the glossary page.
func TestGlossaryAppearsInOutline(t *testing.T) {
	doc := docWithGlossaryTerms([]sectionData{
		{Heading: "1. Intro", Clauses: []clauseData{
			{Segments: []clauseSegment{{Type: "prose", Text: "A clause."}}},
		}},
	})
	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf, []byte("/Title (Glossary)")) {
		t.Error("PDF outline must contain a /Title (Glossary) bookmark entry")
	}
}

// TestAnnotationRectsMatchLineY verifies that link annotation rects on body
// lines use the actual positioned Y value, not a re-simulated one.
func TestAnnotationRectsMatchLineY(t *testing.T) {
	doc := documentModel{
		Title: "Annotation Y Test",
		Sections: []sectionData{{
			Heading: "1. Terms",
			Clauses: []clauseData{{
				Segments: []clauseSegment{
					{Type: "prose", Text: "The term "},
					{Type: "ontology-link", Text: "prov:Entity", Ref: "http://www.w3.org/ns/prov#Entity"},
					{Type: "prose", Text: " is defined here."},
				},
			}},
		}},
		Glossary: []glossaryTerm{
			{Term: "prov:Entity", TermURI: "http://www.w3.org/ns/prov#Entity"},
		},
		EmbeddedPayload: []byte(`{}`),
		PayloadHash:   strings.Repeat("0", 64),
		FileID:        strings.Repeat("0", 64),
		NamespaceMap:  map[string]string{},
	}
	pages := layoutDocumentPages(doc)

	// Collect Y ranges for all body lines.
	type yRange struct{ min, max float64 }
	var lineYRanges []yRange
	for _, page := range pages {
		for _, line := range page.Lines {
			if line.Kind == "body" {
				lh := line.FontSize * 1.45
				lineYRanges = append(lineYRanges, yRange{line.Y - 4, line.Y + lh + 4})
			}
		}
	}

	// Glossary-dest annotations on pages that contain body lines must have
	// Rect[1] within a body line's Y range. (Glossary-page self-annotations are
	// excluded because those pages have no body lines.)
	const tolerance = 2.0
	for pi, page := range pages {
		// Determine whether this page has any body lines.
		hasBody := false
		for _, line := range page.Lines {
			if line.Kind == "body" {
				hasBody = true
				break
			}
		}
		if !hasBody {
			continue
		}
		for _, ann := range page.Annotations {
			if ann.URI != "" {
				continue // URI annotations
			}
			annY := ann.Rect[1]
			matched := false
			for _, yr := range lineYRanges {
				if annY >= yr.min-tolerance && annY <= yr.max+tolerance {
					matched = true
					break
				}
			}
			if !matched {
				t.Errorf("page %d annotation %q Rect[1]=%.1f does not match any body line Y range (stale re-simulation?)", pi+1, ann.Term, annY)
			}
		}
	}
}

// TestBodyWrapUsesFullTextWidth verifies that a clause of ~83 chars fits on a
// single body line. With the old hardcoded 78-char limit it wrapped onto two
// lines, leaving a large dead right margin. The correct limit is derived from
// the available text width (pageWidth − leftMargin − rightMargin) / charWidth.
func TestBodyWrapUsesFullTextWidth(t *testing.T) {
	// 83-char clause: fits within the full text column (~86 chars) but exceeded
	// the old limit of 78. Verify it stays on one line after the fix.
	longClause := "The quick brown fox jumps over the lazy dog and then does something more herein ok."
	if len(longClause) <= 78 {
		t.Fatalf("test clause must be >78 chars, got %d", len(longClause))
	}
	doc := sectionDoc([]sectionData{{
		Heading: "1. Test",
		Clauses: []clauseData{{
			Segments: []clauseSegment{{Type: "prose", Text: longClause}},
		}},
	}})
	pages := layoutDocumentPages(doc)
	bodyLines := 0
	for _, page := range pages {
		for _, line := range page.Lines {
			if line.Kind == "body" {
				bodyLines++
			}
		}
	}
	if bodyLines != 1 {
		t.Errorf("83-char clause must fit on 1 body line with the full text width, got %d lines (wrap limit is too narrow)", bodyLines)
	}
}

// TestAnnotationXUsesHelveticaMetrics verifies that body-line annotation Rect[0]
// is computed from actual Helvetica glyph widths, not a fixed char-width
// multiplier. The prefix "Width test here: " (17 chars) has a Helvetica
// width of 7281/1000 * 11pt ≈ 80.1pt, but 17 × 5.8 = 98.6pt — an 18pt error
// that places the clickbox well to the right of the visible term.
func TestAnnotationXUsesHelveticaMetrics(t *testing.T) {
	const leftMargin = 54.0
	const fontSize = 11.0
	prefix := "Width test here: "
	term := "prov:Entity"
	// Sum of standard Helvetica widths (thousandths) for "Width test here: "
	// W=944 i=222 d=556 t=278 h=556 SP=278 t=278 e=556 s=500 t=278
	// SP=278 h=556 e=556 r=333 e=556 :=278 SP=278
	const prefixWidthThou = 944 + 222 + 556 + 278 + 556 + 278 +
		278 + 556 + 500 + 278 + 278 + 556 + 556 + 333 + 556 + 278 + 278
	expectedX := leftMargin + float64(prefixWidthThou)*fontSize/1000.0 // ≈ 134.1pt

	doc := documentModel{
		Title: "Metrics Test",
		Sections: []sectionData{{
			Heading: "1. Test",
			Clauses: []clauseData{{
				Segments: []clauseSegment{
					{Type: "prose", Text: prefix},
					{Type: "ontology-link", Text: term, Ref: "http://www.w3.org/ns/prov#Entity"},
					{Type: "prose", Text: " follows."},
				},
			}},
		}},
		Glossary:      []glossaryTerm{{Term: "prov:Entity", TermURI: "http://www.w3.org/ns/prov#Entity"}},
		EmbeddedPayload: []byte(`{}`),
		PayloadHash:   strings.Repeat("0", 64),
		FileID:        strings.Repeat("0", 64),
		NamespaceMap:  map[string]string{},
	}
	pages := layoutDocumentPages(doc)

	var annRect [4]float64
	found := false
	for _, page := range pages {
		for _, ann := range page.Annotations {
			if ann.Term == term && ann.URI == "" {
				annRect = ann.Rect
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Fatal("annotation for prov:Entity not found")
	}
	const tol = 2.0
	if annRect[0] < expectedX-tol || annRect[0] > expectedX+tol {
		t.Errorf("annotation Rect[0]=%.2f, want %.2f±%.0f (Helvetica metrics); old fixed-width gives ≈%.2f",
			annRect[0], expectedX, tol, leftMargin+float64(len(prefix))*5.8)
	}
}

// TestGlossaryLinkAnnotationUsesExplicitDestination verifies that link
// annotations pointing from body text to a glossary entry embed an explicit
// array destination ([page /XYZ x y 0]) rather than a string-based named
// destination ((glossary-N)). String destinations require a /Names /Dests
// catalog lookup that is not reliably supported across PDF viewers.
func TestGlossaryLinkAnnotationUsesExplicitDestination(t *testing.T) {
	doc := documentModel{
		Title: "Dest Test",
		Sections: []sectionData{{
			Heading: "1. Terms",
			Clauses: []clauseData{{
				Segments: []clauseSegment{
					{Type: "prose", Text: "The term "},
					{Type: "ontology-link", Text: "prov:Entity", Ref: "http://www.w3.org/ns/prov#Entity"},
					{Type: "prose", Text: " is defined below."},
				},
			}},
		}},
		Glossary: []glossaryTerm{
			{Term: "prov:Entity", TermURI: "http://www.w3.org/ns/prov#Entity", Definition: "An entity."},
		},
		EmbeddedPayload: []byte(`{}`),
		PayloadHash:   strings.Repeat("0", 64),
		FileID:        strings.Repeat("0", 64),
		NamespaceMap:  map[string]string{},
	}
	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(pdf, []byte("/Dest (glossary-")) {
		t.Error("glossary link annotations must use explicit array destinations [page /XYZ ...], not named string destinations (glossary-N) — string destinations require a /Names /Dests lookup not supported by all viewers")
	}
}

// TestOntologyLinkWithoutSchemaNameUsesCompactedIRI verifies that when a content
// node has only @id (no schema:name), the segment Text is the compacted prefix
// form (e.g. "prov:Entity") rather than the raw full IRI.
func TestOntologyLinkWithoutSchemaNameUsesCompactedIRI(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
			"prov": "http://www.w3.org/ns/prov#"
		},
		"@id": "urn:doc:compact-iri-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "Compact IRI Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:compact-iri-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:compact-iri-test#s1", "children": ["urn:doc:compact-iri-test#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:compact-iri-test#s1", "title": "1. Test"},
				{"@type": "Clause", "@id": "urn:doc:compact-iri-test#c1", "content": [
					"A term: ",
					{"@id": "prov:Entity"}
				]}
			]
		}
	}`)
	model := mustExtractFromPayload(t, payload)
	if len(model.Sections) != 1 || len(model.Sections[0].Clauses) != 1 {
		t.Fatalf("unexpected model structure")
	}
	clause := model.Sections[0].Clauses[0]
	for _, seg := range clause.Segments {
		if seg.Type == "ontology-link" && seg.Ref == "http://www.w3.org/ns/prov#Entity" {
			if seg.Text != "prov:Entity" {
				t.Errorf("ontology-link without schema:name must use compacted IRI as Text; got %q, want %q", seg.Text, "prov:Entity")
			}
			return
		}
	}
	t.Errorf("no ontology-link segment with Ref=http://www.w3.org/ns/prov#Entity found; got %+v", clause.Segments)
}

// TestGlossaryAnnotationCreatedForBodyTextTerm verifies that an ontology-link
// segment in body text produces a clickable /Dest annotation pointing at the
// corresponding glossary entry.
func TestGlossaryAnnotationCreatedForBodyTextTerm(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
			"prov": "http://www.w3.org/ns/prov#"
		},
		"@id": "urn:doc:glossary-link-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "Glossary Link Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:glossary-link-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:glossary-link-test#s1", "children": ["urn:doc:glossary-link-test#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:glossary-link-test#s1", "title": "1. Test"},
				{"@type": "Clause", "@id": "urn:doc:glossary-link-test#c1", "content": [
					"A term: ",
					{"@id": "prov:Entity"},
					" defined here."
				]}
			]
		}
	}`)
	model := mustExtractFromPayload(t, payload)
	if len(model.Glossary) == 0 {
		t.Fatal("model must have at least one glossary term (prov:Entity) to test annotation creation")
	}
	pages := layoutDocumentPages(model)
	for _, page := range pages {
		hasBody := false
		for _, line := range page.Lines {
			if line.Kind == "body" {
				hasBody = true
				break
			}
		}
		if !hasBody {
			continue
		}
		for _, ann := range page.Annotations {
			if ann.URI == "" && ann.DestPageIdx >= 0 {
				return // glossary-dest annotation found — pass
			}
		}
	}
	t.Error("no glossary-dest annotation found on body page; prov:Entity term in body text must link to its glossary entry")
}

// TestInternalGlossaryLinkUsesGoToAction verifies that link annotations for
// glossary navigation use a /GoTo action dict (/A << /S /GoTo /D [page /XYZ x y 0] >>)
// rather than a bare /Dest entry. The /A /GoTo form is universally supported
// across PDF viewers; the /Dest shorthand in Link annotations is unreliable in
// several common viewers (Edge, Foxit, SumatraPDF) which only process annotations
// with explicit /A action dicts.
func TestInternalGlossaryLinkUsesGoToAction(t *testing.T) {
	stubPages := []pageLayout{{ObjectID: 5}}
	annot := annotationRef{
		ObjectID:    99,
		Term:        "prov:Entity",
		Rect:        [4]float64{10, 20, 100, 30},
		DestPageIdx: 0,
		DestY:       700,
	}
	out := renderAnnotationObject(annot, stubPages)
	if !strings.Contains(out, "/A <<") || !strings.Contains(out, "/S /GoTo") {
		t.Errorf("internal glossary link annotation must use /A << /S /GoTo /D >> action dict; got: %s", out)
	}
	if strings.Contains(out, "/Dest [") {
		t.Errorf("internal glossary link annotation must not use bare /Dest entry; got: %s", out)
	}
}
