package compiler

import (
	"bytes"
	"testing"
)

// TestNormalizePayloadWithBareUUIDIdentifier reproduces a real-world DCS
// backend payload shape: a top-level @id (and nested @ids) that are bare
// UUIDs/fragments with no URI scheme at all (e.g. "15d84717-71c3-...",
// "15d84717-71c3-...#metadata") — unlike pdf-core's own feature-test fixtures,
// which always use a properly scheme-prefixed absolute @id ("urn:doc:...").
// Without an @base in the normalization context, a relative @id cannot
// resolve to an absolute IRI, so URDNA2015 RDF conversion silently drops the
// entire node (per the JSON-LD/RDF spec — this is not an error, just an empty
// result), yielding zero N-Quads and a payload hash of sha256("") for every
// document the real DCS backend ever compiles.
func TestNormalizePayloadWithBareUUIDIdentifier(t *testing.T) {
	payload := []byte(`{
		"@context": {
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#",
			"xsd": "http://www.w3.org/2001/XMLSchema#"
		},
		"@id": "15d84717-71c3-41c3-8fca-36ef9fa091a0",
		"@type": "dcs:ContractTemplate",
		"dcs:metadata": {
			"@id": "15d84717-71c3-41c3-8fca-36ef9fa091a0#metadata",
			"@type": "dcs:TemplateMetadata",
			"dcs:title": "Bare UUID identifier test"
		}
	}`)

	canonical, err := CanonicalizePayload(payload)
	if err != nil {
		t.Fatalf("CanonicalizePayload: %v", err)
	}

	nquads, err := NormalizePayload(canonical)
	if err != nil {
		t.Fatalf("NormalizePayload: %v", err)
	}
	if len(nquads) == 0 {
		t.Fatalf("NormalizePayload produced zero N-Quads for a bare-UUID-identified document; canonical was: %s", canonical)
	}
	if bytes.Contains(nquads, []byte("_:")) {
		t.Errorf("expected the bare UUID @id to resolve to a real IRI, not a blank node; got: %s", nquads)
	}
}
