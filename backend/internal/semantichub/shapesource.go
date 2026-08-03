package semantichub

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"digital-contracting-service/internal/base/validation"

	"github.com/jmoiron/sqlx"
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

// ActiveShapes returns the canonical contract shapes concatenated with the
// clause catalog's active version into one shapes graph.
func (h HubShapeSource) ActiveShapes(ctx context.Context) (string, int, error) {
	content, version, err := h.active(ctx, ShapesName, "shapes")
	if err != nil {
		return "", 0, err
	}
	merged, err := h.withClauseCatalog(ctx, content)
	if err != nil {
		return "", 0, err
	}
	return merged, version, nil
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

// ShapesAt returns exactly the canonical SHACL shapes at a specific version.
// Mixing an immutable canonical version with today's active libraries would
// silently change old artifacts after an activation or rollback.
func (h HubShapeSource) ShapesAt(ctx context.Context, version int) (string, error) {
	content, err := h.versionContent(ctx, ShapesName, "shapes", version)
	if err != nil {
		return "", fmt.Errorf("semantic hub: pinned shapes v%d: %w", version, err)
	}
	return content, nil
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

// withClauseCatalog appends the clause catalog's active shapes and every
// other ACTIVE registered shape library to a canonical shapes document.
// Each document declares its own @prefix headers, so the concatenation
// parses as one Turtle graph. Registered libraries are arbitrary SHACL
// uploaded through the hub — they target their own classes, and the
// validation pass dereferences contract-field references so the libraries
// see plain instance data.
func (h HubShapeSource) withClauseCatalog(ctx context.Context, canonicalShapesTTL string) (string, error) {
	catalog, _, err := h.active(ctx, ClauseCatalogName, "shapes")
	if err != nil {
		return "", fmt.Errorf("clause catalog: %w", err)
	}
	libraries, err := h.activeShapeLibraries(ctx)
	if err != nil {
		return "", err
	}
	merged := canonicalShapesTTL + "\n\n" + catalog
	for _, library := range libraries {
		merged += "\n\n" + library
	}
	return merged, nil
}

// activeShapeLibraries returns the content of every ACTIVE kind="shapes"
// schema other than the canonical shapes and the clause catalog, in stable
// name order.
func (h HubShapeSource) activeShapeLibraries(ctx context.Context) ([]string, error) {
	var libraries []string
	err := h.DB.SelectContext(ctx, &libraries, `
        SELECT content FROM semantic_schemas
        WHERE kind = 'shapes' AND active AND name NOT IN ($1, $2)
        ORDER BY name`, ShapesName, ClauseCatalogName)
	if err != nil {
		return nil, fmt.Errorf("semantic hub: active shape libraries: %w", err)
	}
	return libraries, nil
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
