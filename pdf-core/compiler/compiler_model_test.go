package compiler

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestCompilePDF_NoOntologyHTTPFetch verifies that compilation makes no outbound
// HTTP calls to ontology endpoints.
func TestCompilePDF_NoOntologyHTTPFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected HTTP request to ontology server: %s %s", r.Method, r.URL)
		http.Error(w, "not expected", http.StatusInternalServerError)
	}))
	defer srv.Close()

	payload := []byte(`{
		"@context": {
			"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
			"dcs": "` + srv.URL + `/ontology/dcs#"
		},
		"@id": "urn:doc:no-fetch-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "No Fetch Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:no-fetch-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:no-fetch-test#s1", "children": ["urn:doc:no-fetch-test#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:no-fetch-test#s1", "title": "1. Terms"},
				{"@type": "Clause", "@id": "urn:doc:no-fetch-test#c1", "content": ["No ontology fetch should occur."]}
			]
		}
	}`)

	_, err := CompilePDF(testSigningContext(), payload, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
}

// TestCompilePDF_DcsCoreIRITitleExtracted verifies that a document whose title
// is in metadata.title is correctly extracted and rendered as the document heading.
func TestCompilePDF_DcsCoreIRITitleExtracted(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
		},
		"@id": "urn:doc:title-iri-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "IRI Title Test Document"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:title-iri-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:title-iri-test#s1", "children": ["urn:doc:title-iri-test#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:title-iri-test#s1", "title": "1. Terms"},
				{"@type": "Clause", "@id": "urn:doc:title-iri-test#c1", "content": ["A clause."]}
			]
		}
	}`)

	pdf, err := CompilePDF(testSigningContext(), payload, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	if !bytes.Contains(pdf, []byte("(IRI Title Test Document) Tj")) {
		t.Errorf("metadata.title must be RENDERED in the page content stream; " +
			"model extraction must use IRI-based lookup, not verbatim JSON key matching.")
	}
}

// TestCompilePDF_PrefixedTermsExtracted verifies that section properties declared
// with the explicit dcs: prefix are correctly recognised and rendered.
func TestCompilePDF_PrefixedTermsExtracted(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
		},
		"@id": "urn:doc:prefix-test",
		"@type": "dcs:ContractTemplate",
		"dcs:metadata": {"@type": "dcs:TemplateMetadata", "dcs:title": "Prefix Test Document"},
		"dcs:documentStructure": {
			"@type": "dcs:DocumentStructure",
			"dcs:layout": [
				{"@type": "dcs:LayoutNode", "dcs:isRoot": true, "dcs:children": ["urn:doc:prefix-test#s1"]},
				{"@type": "dcs:LayoutNode", "@id": "urn:doc:prefix-test#s1", "dcs:children": ["urn:doc:prefix-test#c1"]}
			],
			"dcs:blocks": [
				{"@type": "dcs:Section", "@id": "urn:doc:prefix-test#s1", "dcs:title": "1. Obligations"},
				{"@type": "dcs:Clause", "@id": "urn:doc:prefix-test#c1", "dcs:content": ["All terms shall use prefixed IRIs."]}
			]
		}
	}`)

	pdf, err := CompilePDF(testSigningContext(), payload, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	if !bytes.Contains(pdf, []byte("(Prefix Test Document) Tj")) {
		t.Errorf("metadata.title must be rendered when using explicit dcs: prefix")
	}
	if !bytes.Contains(pdf, []byte("(1. Obligations) Tj")) {
		t.Errorf("section title must be rendered; sections must be found by IRI, not verbatim key")
	}
}

// TestCompilePDF_TitleFieldRendered verifies that metadata.title is rendered as the PDF heading.
func TestCompilePDF_TitleFieldRendered(t *testing.T) {
	payload := []byte(`{
		"@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id": "urn:doc:title-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "My Contract Template"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:title-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:title-test#s1", "children": ["urn:doc:title-test#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:title-test#s1", "title": "1. Terms"},
				{"@type": "Clause", "@id": "urn:doc:title-test#c1", "content": ["A clause."]}
			]
		}
	}`)

	pdf, err := CompilePDF(testSigningContext(), payload, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	if !bytes.Contains(pdf, []byte("(My Contract Template) Tj")) {
		t.Errorf("metadata.title must be rendered as the document heading")
	}
}

// TestCompilePDF_MissingTitleReturnsError verifies that CompilePDF returns an
// error when the payload contains no metadata or no title in metadata.
func TestCompilePDF_MissingTitleReturnsError(t *testing.T) {
	payload := []byte(`{
		"@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id": "urn:doc:no-title-test",
		"@type": "ContractTemplate",
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:no-title-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:no-title-test#s1", "children": ["urn:doc:no-title-test#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:no-title-test#s1", "title": "1. Terms"},
				{"@type": "Clause", "@id": "urn:doc:no-title-test#c1", "content": ["A clause."]}
			]
		}
	}`)

	_, err := CompilePDF(testSigningContext(), payload, time.Now())
	if err == nil {
		t.Error("CompilePDF must return an error when metadata/title is absent")
	}
}

// TestCompilePDF_NonStablePrefixRenderedCompact verifies that ontology-link terms
// from a namespace not in the stable context (e.g. odrl) are rendered compact
// when the CanonicalizePayload→CompilePDF path is used.
func TestCompilePDF_NonStablePrefixRenderedCompact(t *testing.T) {
	raw := []byte(`{
		"@context": {
			"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#",
			"odrl": "http://www.w3.org/ns/odrl/2/"
		},
		"@id": "urn:doc:odrl-compact-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "ODRL Compact Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:odrl-compact-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:odrl-compact-test#s1", "children": ["urn:doc:odrl-compact-test#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:odrl-compact-test#s1", "title": "1. Test"},
				{"@type": "Clause", "@id": "urn:doc:odrl-compact-test#c1", "content": ["Subject to the applicable ", "odrl:Policy", "."]}
			]
		}
	}`)
	canonical, err := CanonicalizePayload(raw)
	if err != nil {
		t.Fatalf("CanonicalizePayload: %v", err)
	}
	pdf, err := CompilePDF(testSigningContext(), canonical, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	btContent := concatBTBlocks(pdf)
	if !bytes.Contains(btContent, []byte("odrl:Policy")) {
		t.Errorf("odrl:Policy must be rendered as compact name after CanonicalizePayload+CompilePDF; not found in PDF content streams.\ncanonical context: %s", canonical[:min(len(canonical), 300)])
	}
	if bytes.Contains(btContent, []byte("http://www.w3.org/ns/odrl/2/Policy")) {
		t.Errorf("odrl:Policy must not be expanded to full IRI in PDF content streams; CanonicalizePayload must preserve compact names")
	}
}

func TestCompilePDF_RendersTextBlocks(t *testing.T) {
	payload := []byte(`{
		"@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id": "urn:doc:text-block-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "Text Block Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:text-block-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:text-block-test#s1", "children": ["urn:doc:text-block-test#t1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:text-block-test#s1", "title": "1. Terms"},
				{"@type": "TextBlock", "@id": "urn:doc:text-block-test#t1", "text": "Rendered text block."}
			]
		}
	}`)

	pdf, err := CompilePDF(testSigningContext(), payload, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	if !bytes.Contains(pdf, []byte("(Rendered text block.) Tj")) {
		t.Fatalf("text block content was not rendered into the PDF content stream")
	}
}

func TestCompilePDF_RendersRootLevelClauseAndTextBlock(t *testing.T) {
	payload := []byte(`{
		"@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id": "urn:doc:root-body-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "Root Body Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:root-body-test#s1", "urn:doc:root-body-test#c1", "urn:doc:root-body-test#t1", "urn:doc:root-body-test#s2"]},
				{"@type": "LayoutNode", "@id": "urn:doc:root-body-test#s1", "children": []},
				{"@type": "LayoutNode", "@id": "urn:doc:root-body-test#s2", "children": []}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:root-body-test#s1", "title": "1. First"},
				{"@type": "Clause", "@id": "urn:doc:root-body-test#c1", "content": ["Root-level clause body."]},
				{"@type": "TextBlock", "@id": "urn:doc:root-body-test#t1", "text": "Root-level text block."},
				{"@type": "Section", "@id": "urn:doc:root-body-test#s2", "title": "2. Second"}
			]
		}
	}`)

	pdf, err := CompilePDF(testSigningContext(), payload, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	if !bytes.Contains(pdf, []byte("(Root-level clause body.) Tj")) {
		t.Fatalf("root-level clause content was not rendered")
	}
	if !bytes.Contains(pdf, []byte("(Root-level text block.) Tj")) {
		t.Fatalf("root-level text block content was not rendered")
	}
	first := bytes.Index(pdf, []byte("(1. First) Tj"))
	clause := bytes.Index(pdf, []byte("(Root-level clause body.) Tj"))
	textBlock := bytes.Index(pdf, []byte("(Root-level text block.) Tj"))
	second := bytes.Index(pdf, []byte("(2. Second) Tj"))
	if !(first >= 0 && clause > first && textBlock > clause && second > textBlock) {
		t.Fatalf("root-level body nodes were not preserved in layout order")
	}
}
