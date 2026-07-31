package validation

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/piprate/json-gold/ld"
	"github.com/tggo/goRDFlib/jsonld"
	"github.com/tggo/goRDFlib/shacl"
)

// Parsed shapes graphs, keyed by content hash. Validation runs on every
// submit/offer/approve/sign, and a registered shapes library can be
// megabytes of Turtle (an imported Gaia-X hub entry) — re-parsing it per
// call dominates request latency. Validation only reads the graph, and its
// lazy read indexes are warmed at cache-fill time, so cached graphs are
// safe to share across concurrent validations.
var (
	shapesGraphCacheMu sync.Mutex
	shapesGraphCache   = map[[sha256.Size]byte]*shacl.Graph{}
)

func parsedShapesGraph(shapesTTL string, shapesVersion int) (*shacl.Graph, error) {
	key := sha256.Sum256([]byte(shapesTTL))
	shapesGraphCacheMu.Lock()
	defer shapesGraphCacheMu.Unlock()
	if graph, ok := shapesGraphCache[key]; ok {
		return graph, nil
	}
	graph, err := shacl.LoadTurtleString(shapesTTL, "urn:dcs:hub:shapes")
	if err != nil {
		return nil, fmt.Errorf("parse SHACL shapes (hub version %d): %w", shapesVersion, err)
	}
	// goRDFlib builds its read indexes lazily on the first pattern lookup and
	// does not guard that build; one lookup here, under the cache mutex,
	// keeps concurrent validations off an unsynchronized map write.
	rdfType := shacl.IRI(shacl.RDFType)
	graph.All(nil, &rdfType, nil)
	// Version churn is rare; a handful of entries covers active + pinned
	// shapes without growing unbounded.
	if len(shapesGraphCache) >= 8 {
		shapesGraphCache = map[[sha256.Size]byte]*shacl.Graph{}
	}
	shapesGraphCache[key] = graph
	return graph, nil
}

// validateAgainstHubShapes checks a decoded JSON-LD document against the
// Semantic Hub shapes graphs the document itself declares in sh:shapesGraph,
// at the versions it pins. Returns the findings and the shapes version they
// were produced against.
func validateAgainstHubShapes(ctx context.Context, contract map[string]any) ([]PolicyFinding, int, error) {
	source, err := requireShapeSource()
	if err != nil {
		return nil, 0, err
	}
	return validateAgainstShapeSource(ctx, contract, source)
}

// validateAgainstShapeSource is validateAgainstHubShapes generalized over an
// explicit ShapeSource, so a caller can validate against a source other than
// the process-wide activeShapeSource without mutating shared process state
// under concurrent request handling.
func validateAgainstShapeSource(ctx context.Context, contract map[string]any, source ShapeSource) ([]PolicyFinding, int, error) {
	var shapesTTL string
	var shapesVersion int
	var err error
	refs, refsErr := EffectiveShapeRefs(contract)
	if refsErr != nil {
		return nil, 0, refsErr
	}
	if len(refs) > 0 {
		bundleSource, ok := source.(EffectiveBundleShapeSource)
		if !ok {
			return nil, 0, fmt.Errorf("shape source cannot resolve immutable effective bundle")
		}
		pinned := pinnedHubShapesVersion(contract, source.CanonicalShapesName())
		if pinned <= 0 || refs[0].Name != source.CanonicalShapesName() || refs[0].Version != pinned {
			return nil, 0, fmt.Errorf("effective shapes bundle does not match sh:shapesGraph")
		}
		shapesTTL, err = bundleSource.ShapesBundleAt(ctx, refs)
		shapesVersion = pinned
	} else {
		shapesTTL, shapesVersion, err = declaredShapes(ctx, contract, source)
	}
	if err != nil {
		return nil, 0, err
	}

	var contextContent string
	if pinnedContext := pinnedHubContextVersion(contract); pinnedContext > 0 {
		contextContent, err = source.ContextAt(ctx, pinnedContext)
		if err != nil {
			return nil, 0, fmt.Errorf("load pinned JSON-LD context v%d: %w", pinnedContext, err)
		}
	} else {
		contextContent, _, err = source.ActiveContext(ctx)
		if err != nil {
			return nil, 0, fmt.Errorf("load active JSON-LD context: %w", err)
		}
	}
	loader, err := hermeticContextLoader(ctx, contextContent, source)
	if err != nil {
		return nil, 0, err
	}

	// Shape libraries are written against plain instance data; dereference
	// filled field references so a library's literal constraints see the
	// value where vanilla SHACL expects it.
	contractJSON, err := json.Marshal(materializeContractDataFields(contract))
	if err != nil {
		return nil, 0, fmt.Errorf("encode contract document: %w", err)
	}

	dataGraph, err := shacl.LoadJsonLDString(string(contractJSON), "urn:dcs:contract", jsonld.WithDocumentLoader(loader))
	if err != nil {
		return nil, 0, fmt.Errorf("parse contract document as JSON-LD: %w", err)
	}
	shapesGraph, err := parsedShapesGraph(shapesTTL, shapesVersion)
	if err != nil {
		return nil, 0, err
	}

	report := shacl.Validate(dataGraph, shapesGraph)
	return mapShaclReport(report, shapesVersion), shapesVersion, nil
}

// declaredShapes resolves the shapes graphs a document is validated against
// into one Turtle document, and the version of the canonical DCS envelope
// graph — the version findings and SHACL evidence are reported against.
//
// The canonical graph, and the clause catalog the source carries with it, is
// ALWAYS resolved. sh:shapesGraph is an ordinary top-level key of
// client-submitted contract JSON-LD, so a document naming only a registered
// library (or only the catalog) would otherwise be checked by that graph
// alone and escape dcs:CanonicalContractShape, dcs:ContractFieldShape and the
// ODRL prose shapes entirely — the gate is not the document's to choose.
//
// What the document declares is opt-IN only: it adds registered libraries and
// it pins the canonical graph's version. So an undeclared library registered
// in the hub still cannot change the verdict — which is what makes the same
// document validate identically on every deployment and years later — and a
// graph the source cannot resolve is still a hard failure.
func declaredShapes(ctx context.Context, contract map[string]any, source ShapeSource) (string, int, error) {
	canonicalName := source.CanonicalShapesName()
	canonical := shapesGraphAnchor{Name: canonicalName}
	pinned := false
	var libraries []shapesGraphAnchor
	for _, anchor := range declaredShapesGraphs(contract) {
		if anchor.Name != canonicalName {
			libraries = append(libraries, anchor)
			continue
		}
		if pinned && anchor.Version != canonical.Version {
			return "", 0, fmt.Errorf(
				"document pins the canonical shapes graph %q at two versions (%d and %d)",
				canonicalName, canonical.Version, anchor.Version)
		}
		canonical.Version = anchor.Version
		pinned = true
	}
	graphs := append([]shapesGraphAnchor{canonical}, libraries...)

	// Each resolved document carries its own @prefix headers, so the
	// concatenation parses as one Turtle graph.
	parts := make([]string, 0, len(graphs))
	version := 0
	for i, anchor := range graphs {
		content, resolved, err := source.ShapesAt(ctx, anchor.Name, anchor.Version)
		if err != nil {
			return "", 0, fmt.Errorf("load declared shapes graph %q (version %d): %w", anchor.Name, anchor.Version, err)
		}
		if i == 0 {
			version = resolved
		}
		parts = append(parts, content)
	}
	return strings.Join(parts, "\n\n"), version, nil
}

// mapShaclReport translates a goRDFlib sh:ValidationReport into the
// PolicyFinding shape every other audit source in this package produces.
// SHACL reports only non-conformant results — a conformant document yields
// no findings.
func mapShaclReport(report shacl.ValidationReport, shapesVersion int) []PolicyFinding {
	findings := make([]PolicyFinding, 0, len(report.Results))
	for _, result := range report.Results {
		findings = append(findings, shaclResultFinding(result, shapesVersion))
	}
	return findings
}

func shaclResultFinding(result shacl.ValidationResult, shapesVersion int) PolicyFinding {
	// SourceShape is frequently a blank node (every inline sh:property [...]
	// shape is anonymous) — not a stable identifier across parses/versions.
	// ResultPath (a real predicate IRI whenever the violation is a property
	// constraint) is: prefer it for the rule ID, falling back to the shape
	// IRI only for node-level violations (sh:targetClass/sh:nodeKind
	// mismatches), which name a real, non-blank NodeShape.
	shapeName := shaclLocalName(termValue(result.SourceShape))
	componentName := shaclLocalName(termValue(result.SourceConstraintComponent))
	pathName := shaclLocalName(termValue(result.ResultPath))
	focusNode := termValue(result.FocusNode)

	ruleID := pathName
	if ruleID == "" {
		ruleID = shapeName
	}
	if componentName != "" {
		ruleID += "-" + componentName
	}

	message := joinResultMessages(result.ResultMessages)
	if strings.TrimSpace(message) == "" {
		message = fmt.Sprintf("%s: constraint %s violated at %s", shapeName, componentName, focusNode)
		if pathName != "" {
			message = fmt.Sprintf("%s: %s must satisfy %s (focus node %s)", shapeName, pathName, componentName, focusNode)
		}
	} else if focusNode != "" {
		message = fmt.Sprintf("%s (focus node %s)", message, focusNode)
	}

	finding := contractFinding(ruleID, shapeName, shaclResultSeverity(result), message, pathName, termValue(result.SourceShape))
	finding.ActualValue = shaclFindingValue(result.Value)
	finding.Operator = componentName
	finding.ShapesVersion = shapesVersion
	return finding
}

func shaclResultSeverity(result shacl.ValidationResult) string {
	switch termValue(result.ResultSeverity) {
	case shacl.SHWarning.Value():
		return "warning"
	case shacl.SHInfo.Value():
		return "info"
	case "":
		return "error"
	default:
		// sh:Violation and any custom/debug/trace severity goRDFlib passes
		// through (e.g. SHACL 1.2's sh:Debug/sh:Trace) — treat anything not
		// explicitly Warning/Info as blocking, matching Validate's own
		// sh:conforms computation.
		return "error"
	}
}

func shaclFindingValue(t shacl.Term) any {
	v := termValue(t)
	if v == "" {
		return nil
	}
	return v
}

func joinResultMessages(messages []shacl.Term) string {
	parts := make([]string, 0, len(messages))
	for _, m := range messages {
		if v := termValue(m); v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, "; ")
}

// termValue safely reads a goRDFlib Term's string value — result terms
// (FocusNode, SourceShape, ResultPath, ...) are nil-valued zero Terms when
// the constraint evaluator had nothing to report for that field.
func termValue(t shacl.Term) string {
	return strings.TrimSpace(t.Value())
}

// shaclLocalName extracts the fragment/last-segment local name from a full
// IRI (e.g. "https://w3id.org/facis/dcs/ontology/v1#ContractShape" ->
// "ContractShape", "http://www.w3.org/ns/shacl#MinCountConstraintComponent"
// -> "MinCountConstraintComponent") for compact, readable rule IDs/titles.
func shaclLocalName(iri string) string {
	if iri == "" {
		return ""
	}
	if i := strings.LastIndexAny(iri, "#/"); i >= 0 && i < len(iri)-1 {
		return iri[i+1:]
	}
	return iri
}

// hermeticContextLoader returns a JSON-LD document loader that serves the
// given hub context content for hub anchor URLs and resolves any other
// context IRI through the ShapeSource's registered contexts — never a
// network fetch during validation.
func hermeticContextLoader(ctx context.Context, hubContextJSON string, source ShapeSource) (ld.DocumentLoader, error) {
	doc, err := ld.DocumentFromReader(strings.NewReader(hubContextJSON))
	if err != nil {
		return nil, fmt.Errorf("parse hub JSON-LD context: %w", err)
	}
	return hubContextLoader{ctx: ctx, hubDocument: doc, source: source}, nil
}

type hubContextLoader struct {
	ctx         context.Context
	hubDocument any
	source      ShapeSource
}

func (l hubContextLoader) LoadDocument(u string) (*ld.RemoteDocument, error) {
	if isHubContextAnchor(u) {
		return &ld.RemoteDocument{DocumentURL: u, Document: l.hubDocument}, nil
	}
	content, err := l.source.ContextByIRI(l.ctx, u)
	if err != nil {
		return nil, fmt.Errorf("SHACL validation: JSON-LD context %q is not registered in the Semantic Hub and network fetches during validation are disallowed: %w", u, err)
	}
	doc, err := ld.DocumentFromReader(strings.NewReader(content))
	if err != nil {
		return nil, fmt.Errorf("registered context %q is not valid JSON: %w", u, err)
	}
	return &ld.RemoteDocument{DocumentURL: u, Document: doc}, nil
}
