package compiler

import (
	"bytes"
	"strings"
	"testing"
)

// TestNormalizePayload_SectionsAndClausesInNQuads verifies that clause text and
// section titles appear in the URDNA2015 N-Quads when the payload uses the
// dcs ontology context. Without this, UpdatePDF cannot detect changes confined
// to human-readable content.
func TestNormalizePayload_SectionsAndClausesInNQuads(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
		},
		"@id": "urn:doc:nquads-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "NQuads Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:nquads-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:nquads-test#s1", "children": ["urn:doc:nquads-test#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:nquads-test#s1", "title": "Section Alpha"},
				{"@type": "Clause", "@id": "urn:doc:nquads-test#c1", "content": ["unique clause sentinel"]}
			]
		}
	}`)
	nquads, err := NormalizePayload(payload)
	if err != nil {
		t.Fatalf("NormalizePayload: %v", err)
	}
	if !bytes.Contains(nquads, []byte("unique clause sentinel")) {
		t.Errorf("N-Quads do not contain clause text; content must be mapped to IRIs via @vocab\nProduced N-Quads:\n%s", nquads)
	}
	if !bytes.Contains(nquads, []byte("Section Alpha")) {
		t.Errorf("N-Quads do not contain section title; title must be mapped to an IRI via @vocab\nProduced N-Quads:\n%s", nquads)
	}
}

// TestNormalizePayload_ClauseChangeProducesDifferentHash verifies that two
// payloads that differ only in clause content produce different N-Quads hashes.
func TestNormalizePayload_ClauseChangeProducesDifferentHash(t *testing.T) {
	base := []byte(`{
		"@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#", "dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id": "urn:doc:diff-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "Diff Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:diff-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:diff-test#s1", "children": ["urn:doc:diff-test#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:diff-test#s1", "title": "1. Test"},
				{"@type": "Clause", "@id": "urn:doc:diff-test#c1", "content": ["original clause"]}
			]
		}
	}`)
	amended := []byte(`{
		"@context": {"@vocab": "https://w3id.org/facis/dcs/ontology/v1#", "dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id": "urn:doc:diff-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "Diff Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:diff-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:diff-test#s1", "children": ["urn:doc:diff-test#c1", "urn:doc:diff-test#c2"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:diff-test#s1", "title": "1. Test"},
				{"@type": "Clause", "@id": "urn:doc:diff-test#c1", "content": ["original clause"]},
				{"@type": "Clause", "@id": "urn:doc:diff-test#c2", "content": ["added clause"]}
			]
		}
	}`)

	nquadsBase, err := NormalizePayload(base)
	if err != nil {
		t.Fatalf("NormalizePayload(base): %v", err)
	}
	nquadsAmended, err := NormalizePayload(amended)
	if err != nil {
		t.Fatalf("NormalizePayload(amended): %v", err)
	}
	if bytes.Equal(nquadsBase, nquadsAmended) {
		t.Error("adding a clause must produce different N-Quads")
	}
}

// TestNormalizePayload_ExamplePayloadFullyRDFied verifies that a complete example
// payload produces N-Quads that contain every piece of semantic content.
func TestNormalizePayload_ExamplePayloadFullyRDFied(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
		},
		"@id": "urn:doc:rdf-completeness-test",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "RDF Completeness Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:rdf-completeness-test#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:rdf-completeness-test#s1", "children": ["urn:doc:rdf-completeness-test#c1", "urn:doc:rdf-completeness-test#c2"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:rdf-completeness-test#s1", "title": "1. Obligations"},
				{"@type": "Clause", "@id": "urn:doc:rdf-completeness-test#c1", "content": ["The service shall produce deterministic output."]},
				{"@type": "Clause", "@id": "urn:doc:rdf-completeness-test#c2", "content": ["All content must appear in the machine-readable RDF graph."]}
			]
		}
	}`)

	nquads, err := NormalizePayload(payload)
	if err != nil {
		t.Fatalf("NormalizePayload: %v", err)
	}
	nq := string(nquads)

	checks := []struct {
		desc    string
		present string
	}{
		{"document IRI", "urn:doc:rdf-completeness-test"},
		{"metadata title IRI", "ontology/v1#title"},
		{"title text", "RDF Completeness Test"},
		{"documentStructure IRI", "ontology/v1#documentStructure"},
		{"blocks IRI", "ontology/v1#blocks"},
		{"section title IRI", "ontology/v1#title"},
		{"section title text", "1. Obligations"},
		{"first clause text", "The service shall produce deterministic output."},
		{"second clause text", "All content must appear in the machine-readable RDF graph."},
	}
	for _, c := range checks {
		if !strings.Contains(nq, c.present) {
			t.Errorf("N-Quads missing %s (%q):\n%s", c.desc, c.present, nq)
		}
	}
}

// TestNormalizePayload_LargeDocumentWithVocab verifies that a complex multi-section
// payload containing typed values can be normalized by URDNA2015 without error.
func TestNormalizePayload_LargeDocumentWithVocab(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#",
			"prov": "http://www.w3.org/ns/prov#",
			"odrl": "http://www.w3.org/ns/odrl/2/",
			"schema": "https://schema.org/",
			"xsd": "http://www.w3.org/2001/XMLSchema#"
		},
		"@id": "urn:doc:large-document",
		"@type": "ContractTemplate",
		"metadata": {"@type": "TemplateMetadata", "title": "Master Services Agreement"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:large-document#s1"]},
				{"@type": "LayoutNode", "@id": "urn:doc:large-document#s1", "children": ["urn:doc:large-document#c1", "urn:doc:large-document#c2"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:doc:large-document#s1", "title": "1. Scope"},
				{"@type": "Clause", "@id": "urn:doc:large-document#c1", "content": ["The provider acts as a ", "prov:Activity", " under this agreement."]},
				{"@type": "Clause", "@id": "urn:doc:large-document#c2", "content": ["The cap is 100 GBP unless agreed in writing."]}
			]
		}
	}`)
	nquads, err := NormalizePayload(payload)
	if err != nil {
		t.Fatalf("NormalizePayload failed for large-document payload with @vocab: %v", err)
	}
	if len(nquads) == 0 {
		t.Fatal("NormalizePayload returned empty N-Quads for large-document payload")
	}
	if !strings.Contains(string(nquads), "1. Scope") {
		t.Error("N-Quads must contain section title for large-document payload")
	}
}
