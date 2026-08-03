package validation

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ShapeSource is the enforcement-time source for the SHACL shapes,
// validation profile, and JSON-LD context AuditContractContent checks
// produced documents against. HubShapeSource (internal/semantichub) is the
// production implementation.
type ShapeSource interface {
	// ActiveShapes returns the SHACL shapes document (hub kind="shapes")
	// currently active, and its version.
	ActiveShapes(ctx context.Context) (content string, version int, err error)
	// ActiveProfile returns the validation profile document (hub
	// kind="profile") currently active, and its version.
	ActiveProfile(ctx context.Context) (content string, version int, err error)
	// ActiveContext returns the JSON-LD context (hub kind="context")
	// currently active, and its version.
	ActiveContext(ctx context.Context) (content string, version int, err error)
	// ShapesAt returns the SHACL shapes document at a specific version —
	// the version a document's sh:shapesGraph anchor pins.
	ShapesAt(ctx context.Context, version int) (content string, err error)
	// ContextAt returns the JSON-LD context at a specific version — the
	// version a document's "@context" hub URL pins.
	ContextAt(ctx context.Context, version int) (content string, err error)
	// ContextByIRI returns the active version of a context registered
	// under the given IRI as its name — how externally anchored contexts
	// a document references are resolved without a network fetch.
	ContextByIRI(ctx context.Context, iri string) (content string, err error)
	// ActiveDomainOntology returns the SLA domain-field ontology (hub
	// name="facis-sla" kind="ontology") currently active, and its
	// version — the source of the dcs:DomainField index.
	ActiveDomainOntology(ctx context.Context) (content string, version int, err error)
}

type VersionedShapeRef struct {
	Name    string
	Version int
}

// EffectiveBundleShapeSource is implemented by the production Semantic Hub.
// It keeps independently-versioned shape libraries immutable for produced
// artifacts without widening ShapeSource for remote/fixture implementations.
type EffectiveBundleShapeSource interface {
	ShapesBundleAt(context.Context, []VersionedShapeRef) (string, error)
	ProfileAt(context.Context, int) (string, error)
}

func effectiveShapeRefs(contract map[string]any) ([]VersionedShapeRef, error) {
	raw, ok := contract["dcs:effectiveShapes"]
	if !ok {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok || len(items) == 0 {
		return nil, errors.New("dcs:effectiveShapes must be a non-empty array")
	}
	refs := make([]VersionedShapeRef, 0, len(items))
	for _, item := range items {
		iri := anchorIRI(item)
		version := anchorVersion(iri)
		const marker = "/semantic/shapes/"
		index := strings.Index(iri, marker)
		if index < 0 || version <= 0 {
			return nil, fmt.Errorf("invalid effective shapes reference %q", iri)
		}
		name := strings.SplitN(iri[index+len(marker):], "?", 2)[0]
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("invalid effective shapes reference %q", iri)
		}
		refs = append(refs, VersionedShapeRef{Name: name, Version: version})
	}
	return refs, nil
}

func pinnedHubProfileVersion(contract map[string]any) int {
	return anchorVersion(anchorIRI(contract["dcterms:conformsTo"]))
}

// activeShapeSource is the process-wide enforcement source, installed at
// startup (cmd/dcs/main.go); nil until SetShapeSource runs.
var activeShapeSource ShapeSource

// SetShapeSource installs the process-wide enforcement source and drops
// the domain-ontology cache so it reloads from the new source.
func SetShapeSource(s ShapeSource) {
	if s != nil {
		activeShapeSource = s
		ResetDomainOntologyCache()
	}
}

func requireShapeSource() (ShapeSource, error) {
	if activeShapeSource == nil {
		return nil, errors.New("semantic hub shape source is not configured (SetShapeSource was never called)")
	}
	return activeShapeSource, nil
}

// pinnedVersionPattern extracts the ?version=N (or &version=N) query
// parameter semantichub.AnchorURL encodes into a hub-served schema URL.
var pinnedVersionPattern = regexp.MustCompile(`[?&]version=(\d+)`)

// pinnedHubShapesVersion reads the hub SHACL shapes version pinned by the
// document's sh:shapesGraph anchor; 0 when there is no versioned anchor.
func pinnedHubShapesVersion(contract map[string]any) int {
	return anchorVersion(anchorIRI(contract["sh:shapesGraph"]))
}

// hubContextAnchorPath marks a hub-served context URL
// (semantichub.AnchorURL) among a document's @context entries.
const hubContextAnchorPath = "/semantic/context/"

func isHubContextAnchor(iri string) bool {
	return strings.Contains(iri, hubContextAnchorPath) || iri == SchemaJSONLDContextV1 || iri == schemaRefJSONLDContext
}

// pinnedHubContextVersion reads the hub context version pinned by the
// document's "@context" — the hub URL is either the whole @context or a
// string entry of its array form.
func pinnedHubContextVersion(contract map[string]any) int {
	switch context := contract["@context"].(type) {
	case string:
		if isHubContextAnchor(context) {
			return anchorVersion(context)
		}
	case []any:
		for _, entry := range context {
			if url, ok := entry.(string); ok && isHubContextAnchor(url) {
				if v := anchorVersion(url); v > 0 {
					return v
				}
			}
		}
	}
	return 0
}

// externalContextIRIs returns the non-hub string entries of a document's
// "@context".
func externalContextIRIs(data map[string]any) []string {
	var iris []string
	collect := func(entry any) {
		if iri, ok := entry.(string); ok && !isHubContextAnchor(iri) {
			iris = append(iris, iri)
		}
	}
	switch context := data["@context"].(type) {
	case string:
		collect(context)
	case []any:
		for _, entry := range context {
			collect(entry)
		}
	}
	return iris
}

// anchorIRI reads the IRI out of a JSON-LD object reference ({"@id": ...})
// or a plain string.
func anchorIRI(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		iri, _ := typed["@id"].(string)
		return iri
	}
	return ""
}

func anchorVersion(iri string) int {
	match := pinnedVersionPattern.FindStringSubmatch(iri)
	if match == nil {
		return 0
	}
	version := 0
	if _, err := fmt.Sscanf(match[1], "%d", &version); err != nil {
		return 0
	}
	return version
}
