package compiler

import (
	"strings"
	"testing"
)

// TestExtractDocumentModelFromCanonical_Basic verifies the struct-based canonical
// model extractor. The function must not exist yet — this test fails to compile
// until extractDocumentModelFromCanonical is implemented.
func TestExtractDocumentModelFromCanonical_Basic(t *testing.T) {
	raw := []byte(`{
		"@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id": "urn:doc:canonical-model-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "Canonical Model Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:canonical-model-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:canonical-model-test#s1",
				 "children": ["urn:doc:canonical-model-test#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:canonical-model-test#s1", "title": "1. Terms"},
				{"@type": "Clause", "@id": "urn:doc:canonical-model-test#c1",
				 "content": ["A plain prose clause."]}
			]
		},
		"signatureFields": [
			{"@id": "urn:doc:canonical-model-test#sig1", "@type": "SignatureField",
			 "signatoryName": "Party A", "title": "Party A Signature"}
		]
	}`)

	canonical, err := CanonicalizePayload(raw)
	if err != nil {
		t.Fatalf("CanonicalizePayload: %v", err)
	}

	hashHex := strings.Repeat("0", 64)
	model, err := extractDocumentModelFromCanonical(canonical, hashHex)
	if err != nil {
		t.Fatalf("extractDocumentModelFromCanonical: %v", err)
	}

	if model.Title != "Canonical Model Test" {
		t.Errorf("Title = %q, want %q", model.Title, "Canonical Model Test")
	}
	if model.ContractID != "urn:doc:canonical-model-test" {
		t.Errorf("ContractID = %q, want %q", model.ContractID, "urn:doc:canonical-model-test")
	}
	if len(model.Sections) != 1 {
		t.Fatalf("Sections len = %d, want 1", len(model.Sections))
	}
	if model.Sections[0].Heading != "1. Terms" {
		t.Errorf("Sections[0].Heading = %q, want %q", model.Sections[0].Heading, "1. Terms")
	}
	if len(model.Sections[0].Clauses) != 1 {
		t.Fatalf("Clauses len = %d, want 1", len(model.Sections[0].Clauses))
	}
	segs := model.Sections[0].Clauses[0].Segments
	if len(segs) != 1 || segs[0].Type != "prose" || segs[0].Text != "A plain prose clause." {
		t.Errorf("Segments = %+v, want [{prose 'A plain prose clause.'}]", segs)
	}
	if len(model.SignatureFields) != 1 {
		t.Fatalf("SignatureFields len = %d, want 1", len(model.SignatureFields))
	}
	if model.SignatureFields[0].Name != "Party A" {
		t.Errorf("SignatureFields[0].Name = %q, want %q", model.SignatureFields[0].Name, "Party A")
	}
	if model.SignatureFields[0].Label != "Party A Signature" {
		t.Errorf("SignatureFields[0].Label = %q, want %q", model.SignatureFields[0].Label, "Party A Signature")
	}
}

// TestExtractDocumentModelFromCanonical_MissingTitle verifies that a payload with
// no metadata.title returns an error rather than an empty title.
func TestExtractDocumentModelFromCanonical_MissingTitle(t *testing.T) {
	raw := []byte(`{
		"@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id": "urn:doc:notitle",
		"@type": "ContractTemplate"
	}`)
	canonical, err := CanonicalizePayload(raw)
	if err != nil {
		t.Fatalf("CanonicalizePayload: %v", err)
	}
	_, err = extractDocumentModelFromCanonical(canonical, strings.Repeat("0", 64))
	if err == nil {
		t.Fatal("expected error for missing metadata.title")
	}
}
