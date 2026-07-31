package envelope

import (
	"fmt"

	"github.com/piprate/json-gold/ld"
)

func canonizeRDF(document any, loader ld.DocumentLoader) (string, error) {
	opts := ld.NewJsonLdOptions("")
	opts.DocumentLoader = loader
	opts.Algorithm = ld.AlgorithmURDNA2015
	opts.Format = "application/n-quads"
	opts.ProduceGeneralizedRdf = false

	processor := ld.NewJsonLdProcessor()
	normalized, err := processor.Normalize(document, opts)
	if err != nil {
		return "", fmt.Errorf("rdf canonicalization failed: %w", err)
	}
	nquads, ok := normalized.(string)
	if !ok {
		return "", fmt.Errorf("unexpected normalization result type %T", normalized)
	}
	return nquads, nil
}
