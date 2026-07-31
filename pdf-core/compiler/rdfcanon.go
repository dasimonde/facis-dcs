package compiler

import (
	"encoding/json"
	"fmt"

	"github.com/piprate/json-gold/ld"
)

// NormalizePayload applies URDNA2015 (Universal RDF Dataset Normalization
// Algorithm 2015) to the JSON-LD payload, producing the deterministic N-Quads
// byte stream used for hashing and as the FileID seed.
//
// Returns:
//
//	nquads – URDNA2015-canonical N-Quads (used for SHA-256 FileID hashing).
//	err
func NormalizePayload(raw []byte) (nquads []byte, err error) {
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("invalid JSON-LD payload: %w", err)
	}
	if _, ok := doc.(map[string]any); !ok {
		return nil, fmt.Errorf("JSON-LD payload must be a JSON object at the root")
	}

	proc := ld.NewJsonLdProcessor()

	// baseIRI lets Expand (invoked internally by Normalize) resolve every
	// document's relative @id (bare UUIDs, e.g. "<did>#metadata") to an
	// absolute IRI — see baseIRIFromContextIRI's doc comment for why this is
	// required (not optional/cosmetic): without it, URDNA2015 silently drops
	// every node in the graph, producing zero N-Quads.
	baseIRI := ""
	var loader ld.DocumentLoader
	if ctxIRI, l, loaderErr := canonicalContextArgs(); loaderErr == nil {
		baseIRI = baseIRIFromContextIRI(ctxIRI)
		loader = l
	}

	normalizeOpts := ld.NewJsonLdOptions(baseIRI)
	normalizeOpts.Algorithm = "URDNA2015"
	normalizeOpts.Format = "application/n-quads"
	if loader != nil {
		normalizeOpts.DocumentLoader = loader
	}
	normResult, err := proc.Normalize(doc, normalizeOpts)
	if err != nil {
		return nil, fmt.Errorf("URDNA2015 normalization failed: %w", err)
	}
	normalized, ok := normResult.(string)
	if !ok {
		return nil, fmt.Errorf("unexpected normalization result type %T", normResult)
	}
	return []byte(normalized), nil
}
