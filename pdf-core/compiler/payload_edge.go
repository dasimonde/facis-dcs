package compiler

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"

	"github.com/piprate/json-gold/ld"
	"github.com/tggo/goRDFlib/shacl"
)

// PayloadSHACLValidationError represents a non-conformant SHACL report.
type PayloadSHACLValidationError struct {
	Report string
}

func (e *PayloadSHACLValidationError) Error() string {
	report := strings.TrimSpace(e.Report)
	if report == "" {
		return "payload failed SHACL validation"
	}
	return "payload failed SHACL validation: " + report
}

var shaclShapes []byte

// SetSHACLBytes stores the SHACL shapes graph used by ValidatePayloadSHACL.
// Must be called before any validation is attempted.
func SetSHACLBytes(b []byte) {
	shaclShapes = b
}

var (
	canonicalCtxMu  sync.RWMutex
	canonicalCtxIRI string
	canonicalCtxDoc []byte
)

// SetContextDocument registers the JSON-LD context document served at contextIRI
// for in-process resolution. CanonicalizePayload uses this IRI as the compaction
// context and installs a document loader so json-gold never attempts an HTTP fetch
// for it. Must be called before any CanonicalizePayload invocation (alongside
// SetSHACLBytes at server startup / TestMain).
func SetContextDocument(contextIRI string, contextDoc []byte) {
	canonicalCtxMu.Lock()
	defer canonicalCtxMu.Unlock()
	canonicalCtxIRI = contextIRI
	canonicalCtxDoc = contextDoc
}

// canonicalContextArgs returns the configured context IRI and a document loader
// that serves the registered context bytes for that IRI (delegating all other
// URLs to json-gold's default HTTP loader). Returns an error if SetContextDocument
// has not been called.
func canonicalContextArgs() (iri string, loader ld.DocumentLoader, err error) {
	canonicalCtxMu.RLock()
	iri = canonicalCtxIRI
	doc := canonicalCtxDoc
	canonicalCtxMu.RUnlock()
	if iri == "" || len(doc) == 0 {
		return "", nil, fmt.Errorf("JSON-LD context not configured; call compiler.SetContextDocument at startup")
	}
	return iri, &inProcessLoader{iri: iri, doc: doc}, nil
}

// baseIRIFromContextIRI derives a JSON-LD API "base IRI" (scheme://host/) from
// the registered context IRI, for use as the base argument to
// ld.NewJsonLdOptions. Every DCS document's @id (and nested @ids, e.g.
// "<did>#metadata") is a bare identifier with no URI scheme at all — unlike
// pdf-core's own feature-test fixtures, which always use a properly
// scheme-prefixed absolute @id ("urn:doc:..."). Without an API-level base IRI,
// json-gold's Expand leaves such relative @ids unresolved, and a node whose
// @id isn't an absolute IRI or blank node is silently dropped during RDF
// conversion (not an error — URDNA2015 normalization just omits it), producing
// zero N-Quads and a payload hash of sha256("") for every document the real
// backend compiles. This must be passed as ld.NewJsonLdOptions's base
// argument specifically — embedding "@base" in the context document's own
// JSON-LD @context is NOT equivalent and does not fix this (verified: Expand
// still left @ids unresolved with only the context-level @base set).
func baseIRIFromContextIRI(contextIRI string) string {
	u, err := url.Parse(contextIRI)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/"
}

// inProcessLoader serves the registered context document for its IRI without
// making any network request, and falls back to json-gold's default HTTP loader
// for all other URLs.
type inProcessLoader struct {
	iri string
	doc []byte
}

func (l *inProcessLoader) LoadDocument(u string) (*ld.RemoteDocument, error) {
	if u == l.iri {
		var parsed any
		if err := json.Unmarshal(l.doc, &parsed); err != nil {
			return nil, fmt.Errorf("parse context document: %w", err)
		}
		return &ld.RemoteDocument{DocumentURL: u, Document: parsed}, nil
	}
	return ld.NewDefaultDocumentLoader(nil).LoadDocument(u)
}

// dcsListProperties are DCS properties whose values form ordered RDF lists
// (@container:@list in the context). json-gold does not apply @container:@list
// from the context when the property is written with a compact-IRI prefix;
// normalizeExpandedProps enforces the {"@list":[...]} wrapper.
var dcsListProperties = map[string]bool{
	dcsOntologyIRI + "blocks":  true,
	dcsOntologyIRI + "content": true,
	dcsOntologyIRI + "layout":  true,
}

// dcsListIRIProperties are DCS properties that are both ordered RDF lists
// (@container:@list) and carry IRI references as values (@type:@id). Both
// constraints are enforced by normalizeExpandedProps: the list wrapper is added
// or preserved, and plain string value objects {"@value":"iri"} are coerced to
// IRI node objects {"@id":"iri"} because json-gold does not apply @type:@id from
// the context when the input uses compact-IRI prefix notation.
var dcsListIRIProperties = map[string]bool{
	dcsOntologyIRI + "children": true,
}

// CanonicalizePayload disambiguates JSON-LD at the application edge by running
// an expand+compact pass with the hosted context so semantically equivalent
// payload flavors serialize to the same canonical JSON representation.
// SetContextDocument must be called before invoking this function.
func CanonicalizePayload(raw []byte) ([]byte, error) {
	var doc any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("invalid JSON-LD payload: %w", err)
	}
	if _, ok := doc.(map[string]any); !ok {
		return nil, fmt.Errorf("JSON-LD payload must be a JSON object at the root")
	}

	ctxIRI, loader, err := canonicalContextArgs()
	if err != nil {
		return nil, err
	}
	baseIRI := baseIRIFromContextIRI(ctxIRI)

	proc := ld.NewJsonLdProcessor()
	expandOpts := ld.NewJsonLdOptions(baseIRI)
	expandOpts.DocumentLoader = loader
	expanded, err := proc.Expand(doc, expandOpts)
	if err != nil {
		return nil, fmt.Errorf("JSON-LD expansion failed: %w", err)
	}

	// Normalise the expanded graph: enforce @list for list properties and coerce
	// string literals to IRI nodes for @type:@id properties. Both are required
	// because json-gold does not apply term annotations from the context when
	// the input uses compact-IRI prefix notation (e.g. dcs:blocks, dcs:children).
	normalizeExpandedProps(expanded)

	compactOpts := ld.NewJsonLdOptions(baseIRI)
	compactOpts.DocumentLoader = loader
	compacted, err := proc.Compact(expanded, ctxIRI, compactOpts)
	if err != nil {
		return nil, fmt.Errorf("JSON-LD compaction failed: %w", err)
	}

	b, err := json.Marshal(compacted)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical JSON-LD: %w", err)
	}
	return b, nil
}

// substituteInlineContext replaces a URL @context value in a JSON-LD document
// with the registered inline context object. This is necessary when handing
// documents to the SHACL library, which has no access to our in-process
// document loader and would otherwise make an HTTP fetch for the context URL.
// Returns the payload unchanged if it does not reference the canonical context.
func substituteInlineContext(payload []byte) ([]byte, error) {
	canonicalCtxMu.RLock()
	iri := canonicalCtxIRI
	doc := canonicalCtxDoc
	canonicalCtxMu.RUnlock()
	if iri == "" || len(doc) == 0 {
		return payload, nil
	}
	var obj map[string]any
	if err := json.Unmarshal(payload, &obj); err != nil {
		return payload, nil
	}
	ctxVal, ok := obj["@context"].(string)
	if !ok || ctxVal != iri {
		return payload, nil // payload uses its own context, nothing to substitute
	}
	var ctxDocParsed map[string]any
	if err := json.Unmarshal(doc, &ctxDocParsed); err != nil {
		return nil, fmt.Errorf("parse registered context document: %w", err)
	}
	ctxObj, ok := ctxDocParsed["@context"]
	if !ok {
		return nil, fmt.Errorf("registered context document has no @context key")
	}
	obj["@context"] = ctxObj
	return json.Marshal(obj)
}

// ValidatePayloadSHACL validates JSON-LD against LinkML-generated SHACL using
// a backend Go SHACL library.
func ValidatePayloadSHACL(raw []byte) error {
	if len(shaclShapes) == 0 {
		return fmt.Errorf("SHACL shapes not initialised; call compiler.SetSHACLBytes before validating")
	}

	// Replace the URL @context with the inline context object so the SHACL
	// library can expand the document without making HTTP fetches.
	inlined, err := substituteInlineContext(raw)
	if err != nil {
		return fmt.Errorf("substitute inline context: %w", err)
	}

	shapesGraph, err := shacl.LoadTurtleString(string(shaclShapes), "")
	if err != nil {
		return fmt.Errorf("load SHACL shapes: %w", err)
	}
	dataGraph, err := shacl.LoadJsonLDString(string(inlined), "")
	if err != nil {
		return fmt.Errorf("load JSON-LD data graph: %w", err)
	}
	report := shacl.Validate(dataGraph, shapesGraph)
	if report.Conforms {
		return nil
	}
	return &PayloadSHACLValidationError{Report: formatSHACLReport(report) + payloadTypeDiagnostic(raw)}
}

// payloadTypeDiagnostic names the document's and its metadata node's @type in
// a SHACL failure so a class-constraint report identifies the offending
// payload shape without access to the payload itself.
func payloadTypeDiagnostic(raw []byte) string {
	var doc map[string]any
	if json.Unmarshal(raw, &doc) != nil {
		return ""
	}
	metaType := ""
	for _, key := range []string{"dcs:metadata", "metadata"} {
		if node, ok := doc[key].(map[string]any); ok {
			metaType, _ = node["@type"].(string)
			break
		}
	}
	return fmt.Sprintf(" (document @type=%v, metadata @type=%q)", doc["@type"], metaType)
}

func formatSHACLReport(report shacl.ValidationReport) string {
	if len(report.Results) == 0 {
		return "payload does not conform to SHACL shapes"
	}
	results := flattenValidationResults(report.Results)
	sort.Slice(results, func(i, j int) bool {
		return results[i].ResultPath.String() < results[j].ResultPath.String()
	})
	parts := make([]string, 0, len(results))
	for _, r := range results {
		focus := strings.TrimSpace(r.FocusNode.String())
		path := strings.TrimSpace(r.ResultPath.String())
		component := strings.TrimSpace(r.SourceConstraintComponent.String())
		message := ""
		if len(r.ResultMessages) > 0 {
			message = strings.TrimSpace(r.ResultMessages[0].Value())
		}
		if message == "" {
			message = strings.TrimSpace(r.Value.String())
		}
		if message == "" {
			message = "constraint violation"
		}
		parts = append(parts, fmt.Sprintf("focus=%s path=%s component=%s message=%s", focus, path, component, message))
	}
	return strings.Join(parts, "; ")
}

func flattenValidationResults(results []shacl.ValidationResult) []shacl.ValidationResult {
	flat := make([]shacl.ValidationResult, 0, len(results))
	var walk func(shacl.ValidationResult)
	walk = func(r shacl.ValidationResult) {
		flat = append(flat, r)
		for _, d := range r.Details {
			walk(d)
		}
	}
	for _, r := range results {
		walk(r)
	}
	return flat
}

// normalizeExpandedProps brings the expanded graph into a consistent shape before
// compaction in a single pass:
//
//   - dcsListProperties (blocks, content): ensure values are wrapped as
//     {"@list":[...]} so @container:@list compaction matches and SHACL list-path
//     constraints (rdf:rest*/rdf:first) can fire.
//   - dcsListIRIProperties (children): same as list properties, but also coerces
//     plain string value objects {"@value":"iri"} to IRI node objects {"@id":"iri"}
//     so @type:@id compaction succeeds. Required because json-gold does not apply
//     @type:@id from the context when the input uses compact-IRI prefix notation.
//   - All other array properties: strip any explicit {"@list":[...]} wrappers
//     to plain arrays so @container:@set compaction produces clean JSON arrays.
func normalizeExpandedProps(expanded []any) {
	for _, node := range expanded {
		if m, ok := node.(map[string]any); ok {
			normalizeNodeProps(m)
		}
	}
}

func normalizeNodeProps(node map[string]any) {
	for k, v := range node {
		arr, ok := v.([]any)
		if !ok {
			continue
		}
		switch {
		case dcsListProperties[k], dcsListIRIProperties[k]:
			iriCoerce := dcsListIRIProperties[k]
			// Already wrapped as an RDF list — handle items, keep wrapper.
			if len(arr) == 1 {
				if listObj, ok := arr[0].(map[string]any); ok {
					if inner, has := listObj["@list"]; has {
						if innerArr, ok := inner.([]any); ok {
							if iriCoerce {
								coerceIRIItems(innerArr)
							}
							for _, item := range innerArr {
								if m, ok := item.(map[string]any); ok {
									normalizeNodeProps(m)
								}
							}
						}
						continue
					}
				}
			}
			// Plain array — coerce IRIs if needed, recurse into items, then wrap.
			if iriCoerce {
				coerceIRIItems(arr)
			}
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok {
					normalizeNodeProps(m)
				}
			}
			node[k] = []any{map[string]any{"@list": arr}}
		default:
			// Strip any @list wrapper to a plain array, then recurse.
			if len(arr) == 1 {
				if listObj, ok := arr[0].(map[string]any); ok {
					if inner, has := listObj["@list"]; has {
						if innerArr, ok := inner.([]any); ok {
							arr = innerArr
							node[k] = arr
						}
					}
				}
			}
			for _, item := range arr {
				if m, ok := item.(map[string]any); ok {
					normalizeNodeProps(m)
				}
			}
		}
	}
}

// coerceIRIItems replaces plain string value objects {"@value":"iri"} in arr with
// IRI node objects {"@id":"iri"} for items that look like IRIs. Items that are
// already IRI nodes or non-IRI values are left unchanged.
func coerceIRIItems(arr []any) {
	for i, item := range arr {
		if vm, ok := item.(map[string]any); ok {
			if sv, hasVal := vm["@value"].(string); hasVal && len(vm) == 1 && looksLikeIRI(sv) {
				arr[i] = map[string]any{"@id": sv}
			}
		}
	}
}

func looksLikeIRI(s string) bool {
	return strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "urn:") ||
		strings.HasPrefix(s, "did:")
}
