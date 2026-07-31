package compiler

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

// loadSHACLForTest initialises the package-level SHACL shapes from the repo
// path so that ValidatePayloadSHACL can be used in unit tests.
func loadSHACLForTest(t *testing.T) {
	t.Helper()
	b, err := os.ReadFile("../docs/semantic-ontology/linkml/output/linkml.yaml.shacl.merged.ttl")
	if err != nil {
		t.Fatalf("loadSHACLForTest: %v", err)
	}
	SetSHACLBytes(b)
}

// TestCompilePDF_ExplicitAtListSyntax verifies that a payload using explicit
// {"@list": [...]} syntax for blocks and content compiles to a valid PDF with
// the correct content. This is the serialisation form produced by the DCS
// frontend and stored in the SHACL test corpus (other-template.jsonld, etc.).
func TestCompilePDF_ExplicitAtListSyntax(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
		},
		"@id": "urn:doc:explicit-list-test",
		"@type": "dcs:ContractTemplate",
		"dcs:metadata": {
			"@type": "dcs:TemplateMetadata",
			"dcs:title": "Explicit List Test"
		},
		"dcs:documentStructure": {
			"@type": "dcs:DocumentStructure",
			"dcs:layout": {"@list": [
				{
					"@type": "dcs:LayoutNode",
					"dcs:isRoot": true,
					"dcs:children": {"@list": ["urn:doc:explicit-list-test#s1"]}
				},
				{
					"@type": "dcs:LayoutNode",
					"@id": "urn:doc:explicit-list-test#s1",
					"dcs:children": {"@list": ["urn:doc:explicit-list-test#c1"]}
				}
			]},
			"dcs:blocks": {"@list": [
				{
					"@type": "dcs:Section",
					"@id": "urn:doc:explicit-list-test#s1",
					"dcs:title": "1. Obligations"
				},
				{
					"@type": "dcs:Clause",
					"@id": "urn:doc:explicit-list-test#c1",
					"dcs:content": {"@list": ["Each party shall perform its obligations."]}
				}
			]}
		}
	}`)

	pdf, err := CompilePDF(testSigningContext(), payload, time.Now())
	if err != nil {
		t.Fatalf("CompilePDF with explicit @list: %v", err)
	}
	if !bytes.Contains(pdf, []byte("(Explicit List Test) Tj")) {
		t.Error("document title not rendered; @list blocks must be traversed in order")
	}
	if !bytes.Contains(pdf, []byte("(1. Obligations) Tj")) {
		t.Error("section heading not rendered from explicit @list blocks")
	}
}

// TestCanonicalizePayload_ContextURLResolution verifies that a payload whose
// @context is a URL served by our in-process document loader is expanded and
// compacted correctly — i.e. SetContextDocument wires the loader so json-gold
// never attempts an HTTP fetch for the registered IRI.
func TestCanonicalizePayload_ContextURLResolution(t *testing.T) {
	const testCtxIRI = "http://test.example/ontology/dcs-pdf-core"
	ctxBytes, err := os.ReadFile("../docs/semantic-ontology/linkml/output/linkml.yaml.context.jsonld")
	if err != nil {
		t.Fatalf("read context: %v", err)
	}
	SetContextDocument(testCtxIRI, ctxBytes)
	t.Cleanup(func() {
		// restore the test-suite default registered by TestMain
		b, _ := os.ReadFile("../docs/semantic-ontology/linkml/output/linkml.yaml.context.jsonld")
		SetContextDocument("http://localhost:8080/ontology/dcs-pdf-core", b)
	})

	payload := []byte(`{
		"@context": "` + testCtxIRI + `",
		"@type": "ContractTemplate",
		"@id": "urn:test:ctx-url",
		"metadata": {"@type": "TemplateMetadata", "title": "URL Context Test"},
		"documentStructure": {
			"@type": "DocumentStructure",
			"layout": [
				{"@type": "LayoutNode", "isRoot": true, "children": ["urn:test:ctx-url#s1"]},
				{"@type": "LayoutNode", "@id": "urn:test:ctx-url#s1", "children": ["urn:test:ctx-url#c1"]}
			],
			"blocks": [
				{"@type": "Section", "@id": "urn:test:ctx-url#s1", "title": "Section"},
				{"@type": "Clause", "@id": "urn:test:ctx-url#c1", "content": ["Content."]}
			]
		}
	}`)

	canonical, err := CanonicalizePayload(payload)
	if err != nil {
		t.Fatalf("CanonicalizePayload with context URL: %v", err)
	}
	doc, err := extractDocumentModelFromCanonical(canonical, strings.Repeat("0", 64))
	if err != nil {
		t.Fatalf("extractDocumentModelFromCanonical: %v", err)
	}
	if doc.Title != "URL Context Test" {
		t.Errorf("title: got %q, want %q", doc.Title, "URL Context Test")
	}
}

// TestCanonicalizePayload_CanonicalFormPreservesListSHACLValidation verifies
// that the canonical form produced by CanonicalizePayload retains enough
// list structure for SHACL to reject invalid content members.
//
// A payload with a dcs:Section nested inside dcs:content (invalid) is
// canonicalized and then re-validated. The canonical form must still fail
// SHACL — if CanonicalizePayload strips the @container:@list hint, SHACL
// validates the content array as plain multi-valued triples and the member
// type constraint is never evaluated, silently accepting the invalid payload.
func TestCanonicalizePayload_CanonicalFormPreservesListSHACLValidation(t *testing.T) {
	loadSHACLForTest(t)

	// A payload whose content array contains an invalid member (Section nested
	// inside Clause content). SHACL must reject this.
	badPayload := []byte(`{
		"@context": {
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#"
		},
		"@id": "urn:doc:invalid-list-canonical",
		"@type": "dcs:ContractTemplate",
		"dcs:metadata": {
			"@type": "dcs:TemplateMetadata",
			"dcs:title": "Invalid List Canonical"
		},
		"dcs:documentStructure": {
			"@type": "dcs:DocumentStructure",
			"dcs:layout": {"@list": [
				{
					"@type": "dcs:LayoutNode",
					"dcs:isRoot": true,
					"dcs:children": {"@list": ["urn:doc:invalid-list-canonical#s1"]}
				},
				{
					"@type": "dcs:LayoutNode",
					"@id": "urn:doc:invalid-list-canonical#s1",
					"dcs:children": {"@list": ["urn:doc:invalid-list-canonical#c1"]}
				}
			]},
			"dcs:blocks": {"@list": [
				{
					"@type": "dcs:Section",
					"@id": "urn:doc:invalid-list-canonical#s1",
					"dcs:title": "1. Bad"
				},
				{
					"@type": "dcs:Clause",
					"@id": "urn:doc:invalid-list-canonical#c1",
					"dcs:content": {"@list": [
						"Valid text.",
						{"@type": "dcs:Section", "dcs:title": "nested section is invalid here"}
					]}
				}
			]}
		}
	}`)

	// Validate the raw payload — SHACL must reject it.
	if err := ValidatePayloadSHACL(badPayload); err == nil {
		t.Fatal("raw payload with invalid content member must be rejected by SHACL")
	}

	// Canonicalize, then re-validate. The canonical form must ALSO be rejected.
	canonical, err := CanonicalizePayload(badPayload)
	if err != nil {
		t.Fatalf("CanonicalizePayload: %v", err)
	}
	if err := ValidatePayloadSHACL(canonical); err == nil {
		t.Error("canonical form silently accepted invalid content member; " +
			"CanonicalizePayload must preserve @container:@list so SHACL list-member " +
			"constraints remain effective on round-tripped payloads")
	}
}
