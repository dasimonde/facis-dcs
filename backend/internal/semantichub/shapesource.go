package semantichub

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"digital-contracting-service/internal/base/validation"

	"github.com/jmoiron/sqlx"
	"github.com/tggo/goRDFlib/shacl"
)

// HubShapeSource is the Semantic Hub-backed enforcement source; it
// structurally satisfies validation.ShapeSource (main.go wires the two
// together).
type HubShapeSource struct {
	DB *sqlx.DB
}

// immutableContent caches (name, kind, version) → content. Hub versions are
// immutable rows, so entries never need invalidation; only the
// which-version-is-active lookups stay live queries.
var immutableContent sync.Map

// CanonicalShapesName is the hub entry the canonical DCS envelope shapes
// live under — resolved for every document, whatever it declares.
func (h HubShapeSource) CanonicalShapesName() string {
	return ShapesName
}

func (h HubShapeSource) ActiveProfile(ctx context.Context) (string, int, error) {
	return h.active(ctx, ProfileName, "profile")
}

func (h HubShapeSource) ActiveContext(ctx context.Context) (string, int, error) {
	return h.active(ctx, ContextName, "context")
}

// ActiveDomainOntology returns the SLA domain-field ontology — the
// dcs:DomainField/dcs:ValueConstraint catalog validation indexes by IRI.
func (h HubShapeSource) ActiveDomainOntology(ctx context.Context) (string, int, error) {
	return h.active(ctx, SLAOntologyName, "ontology")
}

// ShapesAt returns one shapes graph a document declares in sh:shapesGraph:
// the hub entry called name at the pinned version, or its active version
// when version is 0. Only the canonical entry carries the clause catalog
// with it (the catalog constrains the DCS envelope's own vocabulary — the
// odrl: rules in dcs:policies and the typed clauses in the document
// structure — so it is not an opt-in library); a registered library resolves
// to exactly its own content.
func (h HubShapeSource) ShapesAt(ctx context.Context, name string, version int) (string, int, error) {
	content, resolved, err := h.shapesEntry(ctx, name, version)
	if err != nil {
		return "", 0, err
	}
	if name != ShapesName {
		return content, resolved, nil
	}
	catalog, _, err := h.active(ctx, ClauseCatalogName, "shapes")
	if err != nil {
		return "", 0, fmt.Errorf("clause catalog: %w", err)
	}
	// Each document carries its own @prefix headers, so the concatenation
	// parses as one Turtle graph.
	return content + "\n\n" + catalog, resolved, nil
}

func (h HubShapeSource) shapesEntry(ctx context.Context, name string, version int) (string, int, error) {
	if version <= 0 {
		content, active, err := h.active(ctx, name, "shapes")
		if err != nil {
			return "", 0, fmt.Errorf("semantic hub: shapes %s: %w", name, err)
		}
		return content, active, nil
	}
	content, err := h.versionContent(ctx, name, "shapes", version)
	if err != nil {
		return "", 0, fmt.Errorf("semantic hub: shapes %s v%d: %w", name, version, err)
	}
	return content, version, nil
}

func (h HubShapeSource) ShapesBundleAt(ctx context.Context, refs []validation.VersionedShapeRef) (string, error) {
	if len(refs) == 0 {
		return "", fmt.Errorf("semantic hub: effective shapes bundle is empty")
	}
	parts := make([]string, 0, len(refs))
	for _, ref := range refs {
		content, err := h.versionContent(ctx, ref.Name, "shapes", ref.Version)
		if err != nil {
			return "", fmt.Errorf("semantic hub: effective shapes %s v%d: %w", ref.Name, ref.Version, err)
		}
		parts = append(parts, content)
	}
	return strings.Join(parts, "\n\n"), nil
}

func (h HubShapeSource) ProfileAt(ctx context.Context, version int) (string, error) {
	return h.versionContent(ctx, ProfileName, "profile", version)
}

// ContextByIRI returns the active version of a context registered under the
// given IRI as its name — externally anchored contexts a document
// references.
func (h HubShapeSource) ContextByIRI(ctx context.Context, iri string) (string, error) {
	content, _, err := h.active(ctx, iri, "context")
	if err != nil {
		return "", fmt.Errorf("context %q: %w", iri, err)
	}
	return content, nil
}

// ContextAt returns the JSON-LD context at a specific version.
func (h HubShapeSource) ContextAt(ctx context.Context, version int) (string, error) {
	content, err := h.versionContent(ctx, ContextName, "context", version)
	if err != nil {
		return "", fmt.Errorf("semantic hub: pinned context v%d: %w", version, err)
	}
	return content, nil
}

// ShapeLibrary identifies a registered SHACL library version: what a
// document declaring that library names in sh:shapesGraph.
type ShapeLibrary struct {
	Name    string
	Version int
}

// libraryTargetClasses caches (name, version) → the classes that library
// version targets. Hub versions are immutable rows, so entries never need
// invalidation — and a registered library can be megabytes of Turtle (an
// imported Gaia-X entry), which is parsed once here rather than on every
// activation refresh.
var libraryTargetClasses sync.Map

// ActiveShapeLibraryClasses indexes every class targeted by an ACTIVE
// registered shapes library — every kind="shapes" entry other than the
// canonical DCS envelope (the canonical shapes and the clause catalog) — to
// the library version that governs it. Document production reads this index
// to declare, in a document's own sh:shapesGraph, the libraries its data
// objects are modelled against: validation then honours the declaration, so
// no document is ever validated against a library it does not name.
func ActiveShapeLibraryClasses(ctx context.Context, db *sqlx.DB) (map[string]ShapeLibrary, error) {
	var libraries []Schema
	err := db.SelectContext(ctx, &libraries, `
        SELECT name, version, content FROM semantic_schemas
        WHERE kind = 'shapes' AND active AND name NOT IN ($1, $2)
        ORDER BY name`, ShapesName, ClauseCatalogName)
	if err != nil {
		return nil, fmt.Errorf("semantic hub: active shape libraries: %w", err)
	}
	index := map[string]ShapeLibrary{}
	for _, library := range libraries {
		classes, err := targetClassesOf(library)
		if err != nil {
			return nil, err
		}
		for _, class := range classes {
			// First library in name order wins a contested class, matching
			// the deterministic order the query imposes.
			if _, taken := index[class]; !taken {
				index[class] = ShapeLibrary{Name: library.Name, Version: library.Version}
			}
		}
	}
	return index, nil
}

func targetClassesOf(library Schema) ([]string, error) {
	key := fmt.Sprintf("%s\x00%d", library.Name, library.Version)
	if cached, ok := libraryTargetClasses.Load(key); ok {
		return cached.([]string), nil
	}
	graph, err := shacl.LoadTurtleString(library.Content, "urn:dcs:hub:shapes:"+library.Name)
	if err != nil {
		return nil, fmt.Errorf("semantic hub: parse shapes library %s v%d: %w", library.Name, library.Version, err)
	}
	targetClass := shacl.IRI(shacl.SH + "targetClass")
	var classes []string
	for _, triple := range graph.All(nil, &targetClass, nil) {
		if iri := triple.Object.Value(); iri != "" {
			classes = append(classes, iri)
		}
	}
	libraryTargetClasses.Store(key, classes)
	return classes, nil
}

// active resolves the entry's active version, then reads that version's
// content through the immutable cache.
func (h HubShapeSource) active(ctx context.Context, name, kind string) (string, int, error) {
	version, err := ActiveVersion(ctx, h.DB, name, kind)
	if err != nil {
		return "", 0, fmt.Errorf("semantic hub: active %s: %w", kind, err)
	}
	content, err := h.versionContent(ctx, name, kind, version)
	if err != nil {
		return "", 0, fmt.Errorf("semantic hub: active %s v%d: %w", kind, version, err)
	}
	return content, version, nil
}

// ActiveVersion returns the version number of the active (name, kind) entry.
func ActiveVersion(ctx context.Context, db *sqlx.DB, name, kind string) (int, error) {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	s, err := (Repo{}).Get(ctx, tx, name, kind, 0)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return s.Version, nil
}

func (h HubShapeSource) versionContent(ctx context.Context, name, kind string, version int) (string, error) {
	if version <= 0 {
		return "", fmt.Errorf("semantic hub: %s/%s: version must be positive, got %d", name, kind, version)
	}
	key := fmt.Sprintf("%s\x00%s\x00%d", name, kind, version)
	if cached, ok := immutableContent.Load(key); ok {
		return cached.(string), nil
	}
	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback() }()
	s, err := (Repo{}).Get(ctx, tx, name, kind, version)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	immutableContent.Store(key, s.Content)
	return s.Content, nil
}
