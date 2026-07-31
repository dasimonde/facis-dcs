package validation

import (
	"digital-contracting-service/internal/base/datatype"
)

// DeclaredShapeLibrary is one hub shapes graph a document declares in
// sh:shapesGraph: the hub entry name, the version its anchor pins (0 = that
// entry's active version), and the anchor IRI itself — which, for a document
// authored on another deployment, points at the hub that holds the graph.
type DeclaredShapeLibrary struct {
	Name    string
	Version int
	Anchor  string
}

// DeclaredShapeLibraries reports the shapes graphs a stored document declares,
// keeping the anchor IRI that declaredShapesGraphs discards: a consumer of a
// document authored elsewhere needs the originating hub's URL to say where a
// graph it does not hold came from.
func DeclaredShapeLibraries(raw *datatype.JSON) ([]DeclaredShapeLibrary, error) {
	data, err := decodeDocumentData(raw)
	if err != nil {
		return nil, err
	}
	var libraries []DeclaredShapeLibrary
	seen := map[shapesGraphAnchor]bool{}
	collect := func(value any) {
		iri := anchorIRI(value)
		name, ok := anchorShapesName(iri)
		if !ok {
			return
		}
		anchor := shapesGraphAnchor{Name: name, Version: anchorVersion(iri)}
		if seen[anchor] {
			return
		}
		seen[anchor] = true
		libraries = append(libraries, DeclaredShapeLibrary{Name: anchor.Name, Version: anchor.Version, Anchor: iri})
	}
	if declared, ok := data["sh:shapesGraph"].([]any); ok {
		for _, entry := range declared {
			collect(entry)
		}
		return libraries, nil
	}
	collect(data["sh:shapesGraph"])
	return libraries, nil
}
