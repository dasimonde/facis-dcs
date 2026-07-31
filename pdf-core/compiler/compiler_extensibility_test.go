package compiler

import (
	"bytes"
	"testing"
	"time"
)

// odrlPayload is a JSON-LD document that includes an ODRL policy alongside the
// standard dcs ontology content. The odrl namespace is unknown to the ontology
// and must be silently passed through to the embedded JSON-LD attachment.
const odrlPayload = `{
	"@context": {
		"@vocab": "https://w3id.org/facis/dcs/ontology/v1#",
		"dcs": "https://w3id.org/facis/dcs/ontology/v1#",
		"odrl": "http://www.w3.org/ns/odrl/2/"
	},
	"@id": "urn:doc:odrl-test",
	"@type": "ContractTemplate",
	"metadata": {"@type": "TemplateMetadata", "title": "ODRL Pass-Through Test"},
	"documentStructure": {
		"@type": "DocumentStructure",
		"layout": [
			{"@type": "LayoutNode", "isRoot": true, "children": ["urn:doc:odrl-test#s1"]},
			{"@type": "LayoutNode", "@id": "urn:doc:odrl-test#s1", "children": ["urn:doc:odrl-test#c1"]}
		],
		"blocks": [
			{"@type": "Section", "@id": "urn:doc:odrl-test#s1", "title": "1. Usage Terms"},
			{"@type": "Clause", "@id": "urn:doc:odrl-test#c1", "content": ["This document is subject to usage constraints."]}
		]
	},
	"odrl:hasPolicy": {
		"@type": "odrl:Policy",
		"odrl:uid": "http://example.com/policy/1",
		"odrl:permission": [{
			"odrl:action": {"@id": "odrl:use"},
			"odrl:target": {"@id": "urn:doc:odrl-test"}
		}]
	}
}`

// TestExtraNamespaceCompilationSucceeds verifies that a payload containing an
// unknown namespace (odrl) compiles without error.
func TestExtraNamespaceCompilationSucceeds(t *testing.T) {
	_, err := CompilePDF(testSigningContext(), []byte(odrlPayload), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF with extra namespace: %v", err)
	}
}

// TestODRLDataPreservedInEmbeddedJSONLD verifies that ODRL properties survive
// the compile→extract round-trip in the embedded JSON-LD attachment.
func TestODRLDataPreservedInEmbeddedJSONLD(t *testing.T) {
	pdf, err := CompilePDF(testSigningContext(), []byte(odrlPayload), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	extracted, err := ExtractEmbeddedJSONLD(pdf)
	if err != nil {
		t.Fatalf("ExtractEmbeddedJSONLD: %v", err)
	}
	// The canonical form uses the odrl: prefix defined in the context.
	if !bytes.Contains(extracted, []byte("odrl:")) {
		t.Errorf("embedded JSON-LD must contain ODRL data; got:\n%s", extracted[:min(len(extracted), 500)])
	}
}

// TestExtraSemanticDataNotRenderedInPDF verifies that ODRL property names and
// type tokens do not appear in the PDF page content streams (BT/ET blocks).
func TestExtraSemanticDataNotRenderedInPDF(t *testing.T) {
	pdf, err := CompilePDF(testSigningContext(), []byte(odrlPayload), time.Now())
	if err != nil {
		t.Fatalf("CompilePDF: %v", err)
	}
	content := concatBTBlocks(pdf)
	for _, fragment := range []string{"odrl:", "odrl/2", "odrl:Policy", "hasPolicy", "permission"} {
		if bytes.Contains(content, []byte(fragment)) {
			t.Errorf("ODRL fragment %q must not appear in page content streams", fragment)
		}
	}
}
