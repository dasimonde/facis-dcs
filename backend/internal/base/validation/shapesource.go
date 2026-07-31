package validation

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ShapeSource is the source for the SHACL shapes, validation profile, and
// JSON-LD context AuditContractContent checks produced documents against.
// HubShapeSource (internal/semantichub) is the production implementation.
//
// Only the shapes half is enforcement-time: RequireHubConformance blocks at
// offer, submit and signing. The validation profile is NOT a gate on any
// path — AuditContractContent's only non-test caller is the read-only
// audit-trail query, so profile findings are reported, never blocking. Do not
// read "EnforceValidationProfile" as a blocking switch.
type ShapeSource interface {
	// CanonicalShapesName is the hub entry name of the canonical DCS envelope
	// shapes graph — the one graph every document is validated against
	// whether or not it declares it (declaredShapes).
	CanonicalShapesName() string
	// ActiveProfile returns the validation profile document (hub
	// kind="profile") currently active, and its version.
	ActiveProfile(ctx context.Context) (content string, version int, err error)
	// ActiveContext returns the JSON-LD context (hub kind="context")
	// currently active, and its version.
	ActiveContext(ctx context.Context) (content string, version int, err error)
	// ShapesAt returns one shapes graph a document's sh:shapesGraph
	// declares: the hub entry called name at the pinned version, or that
	// entry's active version when version is 0. Also returns the version it
	// resolved to.
	ShapesAt(ctx context.Context, name string, version int) (content string, resolved int, err error)
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

// EffectiveShapeRefs returns the immutable shapes bundle a document pins in
// dcs:effectiveShapes — the exact hub entries and versions it is validated
// against, whatever those entries are active at now. Absent (a document
// produced before the bundle was pinned) yields no refs and no error.
func EffectiveShapeRefs(contract map[string]any) ([]VersionedShapeRef, error) {
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

// pinnedHubShapesVersion reads the canonical shapes version a document pins.
// sh:shapesGraph is multi-valued — a document names the shape libraries its
// data is modelled against beside the canonical DCS shapes (ADR-23) — so the
// canonical anchor is located by name among every declared anchor rather than
// assumed to be the sole value.
func pinnedHubShapesVersion(contract map[string]any, canonicalName string) int {
	for _, anchor := range declaredShapesGraphs(contract) {
		if anchor.Name == canonicalName {
			return anchor.Version
		}
	}
	return 0
}

// hubShapesAnchorPath marks a hub-served shapes URL
// (semantichub.AnchorURL) among a document's sh:shapesGraph values.
const hubShapesAnchorPath = "/semantic/shapes/"

// shapesGraphAnchor is one shapes graph a document declares: a hub entry
// name plus the version it pins (0 = that entry's active version).
type shapesGraphAnchor struct {
	Name    string
	Version int
}

// shapesGraphDeclaration is one anchor as a document actually wrote it: the
// hub entry and version it resolves to, plus the IRI itself. A federated
// document names its libraries under the authoring instance's hostname, so the
// IRI cannot be reconstructed from name and version here.
type shapesGraphDeclaration struct {
	Name    string
	Version int
	IRI     string
}

// declaredShapesGraphDeclarations reads an sh:shapesGraph value — SHACL's own
// data-graph→shapes-graph link, multi-valued, so a document whose data is
// modelled against a registered hub library names that library beside the
// canonical DCS shapes (ADR-8 pin, ADR-23 libraries) — in declaration order.
// Anchors that are not hub-served shapes URLs (the compile-time w3id default an
// unanchored document carries) declare nothing and are skipped.
func declaredShapesGraphDeclarations(value any) []shapesGraphDeclaration {
	var declarations []shapesGraphDeclaration
	seen := map[shapesGraphAnchor]bool{}
	collect := func(entry any) {
		iri := anchorIRI(entry)
		name, ok := anchorShapesName(iri)
		if !ok {
			return
		}
		anchor := shapesGraphAnchor{Name: name, Version: anchorVersion(iri)}
		if seen[anchor] {
			return
		}
		seen[anchor] = true
		declarations = append(declarations, shapesGraphDeclaration{Name: anchor.Name, Version: anchor.Version, IRI: iri})
	}
	if list, ok := value.([]any); ok {
		for _, entry := range list {
			collect(entry)
		}
		return declarations
	}
	collect(value)
	return declarations
}

// declaredShapesGraphs reads the shapes graphs a document declares in
// sh:shapesGraph. A document is validated against exactly these graphs and
// nothing else.
func declaredShapesGraphs(data map[string]any) []shapesGraphAnchor {
	declarations := declaredShapesGraphDeclarations(data["sh:shapesGraph"])
	if len(declarations) == 0 {
		return nil
	}
	anchors := make([]shapesGraphAnchor, 0, len(declarations))
	for _, declaration := range declarations {
		anchors = append(anchors, shapesGraphAnchor{Name: declaration.Name, Version: declaration.Version})
	}
	return anchors
}

// DeclaredShapesGraphs returns the shapes graphs a document declares, in
// declaration order. The first is the canonical DCS envelope graph:
// PinSemanticBundle writes it there, ahead of the libraries the document's own
// data objects are modelled against.
func DeclaredShapesGraphs(data map[string]any) []VersionedShapeRef {
	anchors := declaredShapesGraphs(data)
	if len(anchors) == 0 {
		return nil
	}
	refs := make([]VersionedShapeRef, 0, len(anchors))
	for _, anchor := range anchors {
		refs = append(refs, VersionedShapeRef(anchor))
	}
	return refs
}

// anchorShapesName reads the hub entry name out of a shapes anchor URL
// (/semantic/shapes/{name}?version=N).
func anchorShapesName(iri string) (string, bool) {
	start := strings.Index(iri, hubShapesAnchorPath)
	if start < 0 {
		return "", false
	}
	name := iri[start+len(hubShapesAnchorPath):]
	if end := strings.IndexAny(name, "?#"); end >= 0 {
		name = name[:end]
	}
	decoded, err := url.PathUnescape(name)
	if err != nil || decoded == "" {
		return "", false
	}
	return decoded, true
}

// hubContextAnchorPath marks a hub-served context URL
// (semantichub.AnchorURL) among a document's @context entries.
const hubContextAnchorPath = "/semantic/context/"

func isHubContextAnchor(iri string) bool {
	return strings.Contains(iri, hubContextAnchorPath) || iri == SchemaJSONLDContextV1 || iri == currentJSONLDContextRef()
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
