package compiler

import (
	"bytes"
	"testing"
)

// TestDcsOntologyIRIConstant verifies the fixed ontology IRI is set correctly.
func TestDcsOntologyIRIConstant(t *testing.T) {
	const want = "https://w3id.org/facis/dcs/ontology/v1#"
	if dcsOntologyIRI != want {
		t.Errorf("dcsOntologyIRI = %q, want %q", dcsOntologyIRI, want)
	}
}

// TestTitleExtractedFromMetadata verifies that the title is read from
// metadata.title under the dcs: namespace, which is the canonical form.
func TestTitleExtractedFromMetadata(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
		},
		"@id": "urn:doc:title-iri-test",
		"@type": "ContractTemplate",
		"metadata": {
			"@type": "TemplateMetadata",
			"title": "My Canonical Title"
		},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:title-iri-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:title-iri-test#s1", "children": ["urn:doc:title-iri-test#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:title-iri-test#s1", "title": "S1"},
				{"@type": "Clause", "@id": "urn:doc:title-iri-test#c1", "content": ["Text."]}
			]
		}
	}`)
	doc := mustExtractFromPayload(t, payload)
	if doc.Title != "My Canonical Title" {
		t.Errorf("title = %q, want %q", doc.Title, "My Canonical Title")
	}
	pdf, err := renderPDF(testSigningContext(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pdf, []byte("My Canonical Title")) {
		t.Error("compiled PDF does not contain the expected title text")
	}
}

// TestPayloadWithExplicitPrefixCompiles verifies that a payload using the
// explicit dcs: prefix (rather than @vocab shorthand) compiles successfully.
func TestPayloadWithExplicitPrefixCompiles(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
		},
		"@id": "urn:doc:prefix-iri",
		"@type": "dcs:ContractTemplate",
		"dcs:metadata": {
			"@type": "dcs:TemplateMetadata",
			"dcs:title": "Prefix IRI Document"
		},
		"dcs:documentStructure": {
			"@type": "dcs:DocumentStructure",
			"dcs:layout": {"@list": [
				{"@type": "dcs:LayoutNode", "dcs:isRoot": true, "dcs:children": {"@list": [{"@id": "urn:doc:prefix-iri#s1"}]}},
				{"@type": "dcs:LayoutNode", "@id": "urn:doc:prefix-iri#s1", "dcs:children": {"@list": [{"@id": "urn:doc:prefix-iri#c1"}]}}
			]},
			"dcs:blocks": {"@list": [
				{"@type": "dcs:Section", "@id": "urn:doc:prefix-iri#s1", "dcs:title": "Explicit Heading"},
				{"@type": "dcs:Clause", "@id": "urn:doc:prefix-iri#c1", "dcs:content": {"@list": ["Explicit clause."]}}
			]}
		}
	}`)
	doc := mustExtractFromPayload(t, payload)
	if doc.Title != "Prefix IRI Document" {
		t.Errorf("title = %q, want %q", doc.Title, "Prefix IRI Document")
	}
	if len(doc.Sections) == 0 {
		t.Error("no sections extracted with explicit dcs: prefix")
	}
	if doc.Sections[0].Heading != "Explicit Heading" {
		t.Errorf("section heading = %q, want %q", doc.Sections[0].Heading, "Explicit Heading")
	}
}
