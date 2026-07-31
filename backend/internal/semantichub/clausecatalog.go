package semantichub

import (
	"sort"
	"strings"

	"github.com/tggo/goRDFlib/shacl"
)

// ClauseCatalogEntry is one typed clause NodeShape: the palette listing for
// GET /semantic/clauses. Form generation itself happens client-side from
// the raw shapes Turtle (shacl-form); the server only enumerates which
// shapes exist and validates submitted instances against the same graph.
type ClauseCatalogEntry struct {
	Type  string
	Label string
	Shape string
}

// ParseClauseCatalog lists every sh:NodeShape with a sh:targetClass in a
// clause-catalog SHACL Turtle. prefixes (the active hub context's
// prefix → IRI map) drives type compaction; terms outside every declared
// namespace keep their full IRI.
func ParseClauseCatalog(shapesTTL string, prefixes map[string]string) ([]ClauseCatalogEntry, error) {
	g, err := shacl.LoadTurtleString(shapesTTL, "urn:dcs:hub:clause-catalog")
	if err != nil {
		return nil, err
	}
	compact := newTermCompactor(prefixes)

	nodeShapeType := shacl.IRI(shacl.SH + "NodeShape")
	rdfTypePred := shacl.IRI(shacl.RDFType)
	targetClassPred := shacl.IRI(shacl.SH + "targetClass")
	labelPred := shacl.IRI(shacl.RDFS + "label")

	shapeSubjects := g.Subjects(rdfTypePred, nodeShapeType)
	entries := make([]ClauseCatalogEntry, 0, len(shapeSubjects))
	for _, shape := range shapeSubjects {
		targetClasses := g.Objects(shape, targetClassPred)
		if len(targetClasses) == 0 {
			continue
		}
		targetClass := targetClasses[0]

		label := localName(targetClass.Value())
		if labels := g.Objects(shape, labelPred); len(labels) > 0 {
			label = labels[0].Value()
		}

		entries = append(entries, ClauseCatalogEntry{
			Type:  compact(targetClass.Value()),
			Label: label,
			Shape: shape.Value(),
		})
	}
	return entries, nil
}

// localName extracts the fragment/last-segment local name from a full IRI.
func localName(iri string) string {
	if iri == "" {
		return ""
	}
	if i := strings.LastIndexAny(iri, "#/"); i >= 0 && i < len(iri)-1 {
		return iri[i+1:]
	}
	return iri
}

// newTermCompactor renders full IRIs as prefix:LocalName using the given
// prefix → namespace map; IRIs outside every declared namespace stay whole
// (still valid JSON-LD keys/types). Longest namespace wins.
func newTermCompactor(prefixes map[string]string) func(string) string {
	type binding struct{ prefix, namespace string }
	bindings := make([]binding, 0, len(prefixes))
	for prefix, namespace := range prefixes {
		if namespace != "" {
			bindings = append(bindings, binding{prefix, namespace})
		}
	}
	sort.Slice(bindings, func(i, j int) bool { return len(bindings[i].namespace) > len(bindings[j].namespace) })
	return func(iri string) string {
		for _, b := range bindings {
			if strings.HasPrefix(iri, b.namespace) && len(iri) > len(b.namespace) {
				return b.prefix + ":" + strings.TrimPrefix(iri, b.namespace)
			}
		}
		return iri
	}
}
