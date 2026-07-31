package validation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"digital-contracting-service/internal/base"
	"digital-contracting-service/internal/base/datatype"
)

// Anchors stamped into newly produced documents: the hub-served versioned
// URLs for the JSON-LD context ("@context") and the SHACL shapes
// ("sh:shapesGraph"). Re-pointed at startup and on every hub activation
// (SetSchemaAnchorRefs).
//
// All four are written together by one hub-activation request handler while
// normalization and validation read them on other request goroutines, so they
// share anchorsMu. The maps are only ever replaced wholesale, never mutated in
// place, which lets a reader take a snapshot under the read lock and walk it
// after releasing.
//
// No validation profile is stamped. The anchor named one hardcoded entry
// whose rules are about the DCS envelope — a contract root and its party
// roles — so claiming conformance to it said nothing about the vocabulary a
// contract's own data is modelled against, which may be any registered SHACL
// library (ADR-23). The profile's rules still run at validation time; only
// the claim in the document is gone.
var (
	schemaRefJSONLDContext = SchemaJSONLDContextV1
	schemaRefSHACLShapes   = SchemaSHACLShapesV1
	// canonicalOntologyIRIs is the active hub context's prefix -> IRI map;
	// documents redefining one of these prefixes are rejected.
	canonicalOntologyIRIs map[string]string
	// shapeLibraryAnchors maps a class targeted by an ACTIVE registered hub
	// shapes library to that library's anchor (SetShapeLibraryAnchors).
	shapeLibraryAnchors map[string]ShapeLibraryAnchor
	anchorsMu           sync.RWMutex
)

// ShapeLibraryAnchor is a registered hub SHACL library's name and the
// versioned anchor URL a document declaring it carries.
type ShapeLibraryAnchor struct {
	Name string
	URL  string
}

// SetShapeLibraryAnchors installs the class -> registered-library index used
// to declare, in a produced document's sh:shapesGraph, the shape libraries
// its own data objects are governed by (ADR-23). Re-installed on every hub
// activation alongside SetSchemaAnchorRefs.
func SetShapeLibraryAnchors(byTargetClass map[string]ShapeLibraryAnchor) {
	anchorsMu.Lock()
	defer anchorsMu.Unlock()
	shapeLibraryAnchors = byTargetClass
}

func currentShapeLibraryAnchors() map[string]ShapeLibraryAnchor {
	anchorsMu.RLock()
	defer anchorsMu.RUnlock()
	return shapeLibraryAnchors
}

// SetSchemaAnchorRefs re-points the anchors of newly produced documents at
// the Semantic Hub's served URLs.
func SetSchemaAnchorRefs(contextRef, shapesRef string) {
	anchorsMu.Lock()
	defer anchorsMu.Unlock()
	if contextRef != "" {
		schemaRefJSONLDContext = contextRef
	}
	if shapesRef != "" {
		schemaRefSHACLShapes = shapesRef
	}
}

func currentJSONLDContextRef() string {
	anchorsMu.RLock()
	defer anchorsMu.RUnlock()
	return schemaRefJSONLDContext
}

func currentSHACLShapesRef() string {
	anchorsMu.RLock()
	defer anchorsMu.RUnlock()
	return schemaRefSHACLShapes
}

// PinSemanticBundle stamps the immutable Semantic Hub bundle selected while a
// new artifact is created: the hub context, the shapes graphs the artifact is
// validated against, and the validation profile, each at an exact version that
// no later activation or rollback moves.
//
// The canonical DCS envelope graph is this deployment's own, so the new
// artifact gets the version active at its creation — the same rule the hub
// context anchor above follows, and what makes an activation change what newly
// created contracts are checked against. The shape LIBRARIES the document
// declares are the opposite case: they are the authoring choice a federated
// template carries, naming the graphs its author modelled its data against, and
// they are what must validate it on a peer. Those are kept exactly as declared,
// at the version declared (ADR-8 pin, ADR-23 libraries).
//
// dcs:effectiveShapes is derived from the result, so the two can never
// disagree: it holds the selected bundle plus every library the document
// declares beyond it, which is what makes a declared library resolve even when
// this hub does not have it active.
func PinSemanticBundle(raw *datatype.JSON, contextRef, canonicalShapesRef string, effectiveShapeRefs []string, profileRef string) (*datatype.JSON, error) {
	if raw == nil || strings.TrimSpace(contextRef) == "" || strings.TrimSpace(canonicalShapesRef) == "" ||
		len(effectiveShapeRefs) == 0 || strings.TrimSpace(profileRef) == "" {
		return nil, errors.New("complete semantic bundle references are required")
	}
	var data documentData
	if err := json.Unmarshal(*raw, &data); err != nil {
		return nil, fmt.Errorf("decode semantic bundle document: %w", err)
	}
	contextEntries := []any{contextRef}
	switch current := data["@context"].(type) {
	case []any:
		for _, entry := range current {
			if iri, ok := entry.(string); ok && isHubContextAnchor(iri) {
				continue
			}
			contextEntries = append(contextEntries, entry)
		}
	case map[string]any:
		contextEntries = append(contextEntries, current)
	}
	data["@context"] = contextEntries
	libraries := declaredShapeLibraryAnchors(data["sh:shapesGraph"], canonicalShapesRef, effectiveShapeRefs)
	if len(libraries) == 0 {
		data["sh:shapesGraph"] = map[string]any{"@id": canonicalShapesRef}
	} else {
		graphs := make([]any, 0, len(libraries)+1)
		graphs = append(graphs, map[string]any{"@id": canonicalShapesRef})
		for _, library := range libraries {
			graphs = append(graphs, map[string]any{"@id": library})
		}
		data["sh:shapesGraph"] = graphs
	}
	data["dcs:effectiveShapes"] = effectiveShapesBundle(effectiveShapeRefs, libraries)
	data["dcterms:conformsTo"] = map[string]any{"@id": profileRef}
	return encodeDocumentData(data)
}

// declaredShapeLibraryAnchors returns the anchors of the shape libraries a
// document declares beside the canonical graph, in declaration order and as the
// document wrote them. An anchor that names no version pins nothing, so it is
// resolved to the bundle's entry of the same name; one this hub has no entry for
// names a graph that resolves nowhere and is dropped rather than pinned.
func declaredShapeLibraryAnchors(declared any, canonicalShapesRef string, bundleRefs []string) []string {
	canonicalName, ok := anchorShapesName(canonicalShapesRef)
	if !ok {
		return nil
	}
	bundleByName := map[string]string{}
	for _, ref := range bundleRefs {
		if name, ok := anchorShapesName(ref); ok {
			bundleByName[name] = ref
		}
	}
	var libraries []string
	for _, declaration := range declaredShapesGraphDeclarations(declared) {
		if declaration.Name == canonicalName {
			continue
		}
		anchor := declaration.IRI
		if declaration.Version <= 0 {
			active, registered := bundleByName[declaration.Name]
			if !registered {
				continue
			}
			anchor = active
		}
		libraries = append(libraries, anchor)
	}
	return libraries
}

// effectiveShapesBundle lists the selected bundle first — its canonical entry
// leads, which is where the gate and validateAgainstShapeSource read the
// canonical graph — followed by every declared library the bundle does not
// already carry at that exact version.
func effectiveShapesBundle(bundleRefs, libraries []string) []any {
	refs := make([]any, 0, len(bundleRefs)+len(libraries))
	pinned := map[shapesGraphAnchor]bool{}
	for _, ref := range bundleRefs {
		refs = append(refs, map[string]any{"@id": ref})
		if name, ok := anchorShapesName(ref); ok {
			pinned[shapesGraphAnchor{Name: name, Version: anchorVersion(ref)}] = true
		}
	}
	for _, library := range libraries {
		name, ok := anchorShapesName(library)
		if !ok {
			continue
		}
		anchor := shapesGraphAnchor{Name: name, Version: anchorVersion(library)}
		if pinned[anchor] {
			continue
		}
		pinned[anchor] = true
		refs = append(refs, map[string]any{"@id": library})
	}
	return refs
}

// semanticBundleProperties are the document properties PinSemanticBundle owns.
// They record which hub assets an artifact was produced against, so they are
// written once at production time and are never the client's to restate.
var semanticBundleProperties = []string{"sh:shapesGraph", "dcs:effectiveShapes", "dcterms:conformsTo"}

// CarrySemanticBundle restores onto a replacement document the Semantic Hub
// bundle the stored document is pinned to. Every command that lets a client
// replace the document wholesale (a save, a submitted draft, a negotiated
// redline) receives one the client assembled from its editor state, and that
// document models none of these properties — so without this the contract loses
// the bundle it was created under, and with it the ability to prove what it was
// validated against.
func CarrySemanticBundle(stored, replacement *datatype.JSON) (*datatype.JSON, error) {
	pinned, err := decodeDocumentData(stored)
	if err != nil {
		return nil, err
	}
	data, err := decodeDocumentData(replacement)
	if err != nil {
		return nil, err
	}
	carried := false
	for _, property := range semanticBundleProperties {
		value, exists := pinned[property]
		if !exists {
			continue
		}
		data[property] = value
		carried = true
	}
	if !carried {
		return replacement, nil
	}
	return encodeDocumentData(data)
}

// SetCanonicalOntologyIRIs installs the ACTIVE hub context's prefix -> IRI
// map for enforcement during normalization.
func SetCanonicalOntologyIRIs(iris map[string]string) {
	anchorsMu.Lock()
	defer anchorsMu.Unlock()
	canonicalOntologyIRIs = iris
}

func currentCanonicalOntologyIRIs() map[string]string {
	anchorsMu.RLock()
	defer anchorsMu.RUnlock()
	return canonicalOntologyIRIs
}

// enforceCanonicalOntologyIRIs rejects documents whose @context redefines a
// hub-declared prefix to a different IRI (DCS-FR-TR-03: templating and
// contracting validate against the Semantic Hub's active schema).
func enforceCanonicalOntologyIRIs(data documentData) error {
	canonical := currentCanonicalOntologyIRIs()
	if len(canonical) == 0 {
		return nil
	}
	switch context := data["@context"].(type) {
	case map[string]any:
		return enforceCanonicalOntologyIRIMap(context, canonical)
	case []any:
		for _, entry := range context {
			if inline, ok := entry.(map[string]any); ok {
				if err := enforceCanonicalOntologyIRIMap(inline, canonical); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func enforceCanonicalOntologyIRIMap(context map[string]any, canonicalIRIs map[string]string) error {
	for prefix, iri := range context {
		supplied, ok := iri.(string)
		if !ok {
			continue
		}
		canonical, known := canonicalIRIs[prefix]
		if known && supplied != canonical {
			return fmt.Errorf(
				"%w: document @context redefines prefix %q to %q, but the Semantic Hub's active context declares %q",
				ErrDocumentSchemaConflict, prefix, supplied, canonical)
		}
	}
	return nil
}

// domainField is one dcs:DomainField from the hub's SLA ontology; its IRI
// is its identity — documents reference it via dcs:domainField {"@id": …}.
type domainField struct {
	IRI        string
	Constraint *valueConstraint
}

type valueConstraint struct {
	Format           string
	Pattern          string
	ValueType        string
	AllowedValues    []string
	ValueOptions     []valueOption
	AllowedValuesRef string
	Min              *float64
	Max              *float64
	Description      string
}

type valueOption struct {
	Value  string
	Label  string
	Symbol string
	IRI    string
}

// clone returns a deep copy so per-field constraint metadata can be
// mutated without touching the shared parsed ontology.
func (constraint *valueConstraint) clone() *valueConstraint {
	copied := *constraint
	copied.AllowedValues = append([]string(nil), constraint.AllowedValues...)
	copied.ValueOptions = append([]valueOption(nil), constraint.ValueOptions...)
	return &copied
}

type documentData map[string]any

// ErrContractHierarchyInvalid is the sentinel wrapped by every hierarchy
// invariant violation (FR-TR-02/FR-CWE-02). The HTTP layer
// (service.mapContractCommandError) maps it to a 4xx client error rather than
// a 500, since it is caused by malformed client-supplied contract data.
var ErrContractHierarchyInvalid = errors.New("contract hierarchy invariant violated")

// ErrDocumentSchemaConflict is a client-input error (map to 400): the
// submitted document's @context redefines a Semantic Hub-declared ontology
// prefix to a different IRI (DCS-FR-TR-03).
var ErrDocumentSchemaConflict = errors.New("document schema conflict")

// childEnumeratingProperties are top-level document properties that would
// enumerate child contracts from a parent document. The hierarchy is
// child→parent only: a document may never list its children (that would leak
// siblings to every receiver of the parent and force a document rewrite on
// every new child). Note dcs:children is deliberately NOT listed here — it is
// a legitimate documentStructure layout term, checked only at the top level.
var childEnumeratingProperties = []string{
	"dcs:childContracts",
	"dcs:subContracts",
	"dcs:hasPart",
}

// validateContractHierarchyInvariants enforces the structural hierarchy rules
// on a decoded contract document (no DB access — the cycle check that needs
// the parent chain lives in the command handler):
//
//   - at most one dcs:parentContract reference;
//   - no child-enumerating top-level property.
func validateContractHierarchyInvariants(data documentData) error {
	if list, isList := data["dcs:parentContract"].([]any); isList && len(list) > 1 {
		return fmt.Errorf("%w: a contract may reference at most one dcs:parentContract, got %d",
			ErrContractHierarchyInvalid, len(list))
	}
	for _, key := range childEnumeratingProperties {
		if _, ok := data[key]; ok {
			return fmt.Errorf("%w: contract documents must not enumerate children (found %q); the hierarchy is child→parent only",
				ErrContractHierarchyInvalid, key)
		}
	}
	return nil
}

// NormalizeTemplateData validates and normalizes template JSON-LD data.
func NormalizeTemplateData(raw *datatype.JSON) (*datatype.JSON, error) {
	data, err := decodeDocumentData(raw)
	if err != nil {
		return nil, err
	}
	if !isCanonicalEnvelope(data) {
		return nil, errors.New("template data must use the canonical dcs:documentStructure envelope")
	}
	if err := enforceCanonicalOntologyIRIs(data); err != nil {
		return nil, err
	}
	normalizeCanonicalEnvelope(data, "dcs:ContractTemplate")
	if err := validateExternalContextsResolvable(data); err != nil {
		return nil, err
	}
	if err := validateCanonicalEnvelope(data, expectedPolicyTypes("dcs:ContractTemplate")); err != nil {
		return nil, err
	}
	return encodeDocumentData(data)
}

// NormalizeTemplateDataForPersistence keeps stored template JSON-LD
// self-identifying when it is read outside the relational row envelope:
// the document @id is the template's dereferenceable resource IRI, minted
// from the system key.
func NormalizeTemplateDataForPersistence(raw *datatype.JSON, did string) (*datatype.JSON, error) {
	normalized, err := NormalizeTemplateData(raw)
	if err != nil {
		return nil, err
	}
	return addDocumentIdentity(normalized, base.ResourceIRI("template", did), did)
}

// NormalizeContractData anchors and validates a canonical contract JSON-LD
// envelope. Semantic values are enforced separately, by
// ValidateContractPolicySatisfaction at the approve/apply gates.
func NormalizeContractData(raw *datatype.JSON, _ bool) (*datatype.JSON, error) {
	data, err := decodeDocumentData(raw)
	if err != nil {
		return nil, err
	}
	if err := validateContractHierarchyInvariants(data); err != nil {
		return nil, err
	}
	if !isCanonicalEnvelope(data) {
		return nil, errors.New("contract data must use the canonical dcs:documentStructure envelope")
	}
	if err := enforceCanonicalOntologyIRIs(data); err != nil {
		return nil, err
	}
	normalizeCanonicalEnvelope(data, "dcs:Contract")
	if err := validateContractMetadataType(data); err != nil {
		return nil, err
	}
	if err := validateExternalContextsResolvable(data); err != nil {
		return nil, err
	}
	if err := validateCanonicalEnvelope(data, expectedPolicyTypes("dcs:Contract")); err != nil {
		return nil, err
	}
	return encodeDocumentData(data)
}

// NormalizeContractDataForPersistence keeps stored contract JSON-LD
// self-identifying when it is read outside the relational row envelope:
// the document @id is the contract's dereferenceable resource IRI, minted
// from the system key, and cross-contract references
// (dcs:parentContract, dcs:renewsContract) are canonicalized to the same
// IRI scheme.
func NormalizeContractDataForPersistence(raw *datatype.JSON, did string, requireSemanticValues bool) (*datatype.JSON, error) {
	normalized, err := NormalizeContractData(raw, requireSemanticValues)
	if err != nil {
		return nil, err
	}
	anchored, err := addDocumentIdentity(normalized, base.ResourceIRI("contract", did), did)
	if err != nil {
		return nil, err
	}
	return canonicalizeContractReferences(anchored)
}

// canonicalizeContractReferences rewrites references to other contracts to
// their resource IRIs, so a document never points at a bare system key.
func canonicalizeContractReferences(raw *datatype.JSON) (*datatype.JSON, error) {
	data, err := decodeDocumentData(raw)
	if err != nil {
		return nil, err
	}
	for _, key := range []string{"dcs:parentContract", "dcs:renewsContract"} {
		node, ok := data[key].(map[string]any)
		if !ok {
			continue
		}
		if id, _ := node["@id"].(string); id != "" {
			node["@id"] = base.ResourceIRI("contract", base.ResourceKey(id))
		}
	}
	return encodeDocumentData(data)
}

// ValidateContractSemantics validates the canonical contract envelope.
func ValidateContractSemantics(raw *datatype.JSON) error {
	data, err := decodeDocumentData(raw)
	if err != nil {
		return err
	}
	if !isCanonicalEnvelope(data) {
		return errors.New("contract data must use the canonical dcs:documentStructure envelope")
	}
	return validateCanonicalEnvelope(data, expectedPolicyTypes("dcs:Contract"))
}

func addDocumentIdentity(raw *datatype.JSON, did string, aliases ...string) (*datatype.JSON, error) {
	data, err := decodeDocumentData(raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(did) != "" {
		previousID, _ := data["@id"].(string)
		// derivedFromTemplate is a cross-document provenance reference, not
		// an identifier owned by the document being anchored. A freshly
		// converted contract still carries the template as its temporary
		// document @id, so the generic rebase would otherwise rewrite this
		// reference to the new contract DID as collateral damage.
		derivedFromTemplateID := ""
		if provenance, ok := data["derivedFromTemplate"].(map[string]any); ok {
			derivedFromTemplateID, _ = provenance["@id"].(string)
		}
		rebaseDocumentIDs(map[string]any(data), previousID, did, aliases)
		if derivedFromTemplateID != "" {
			data["derivedFromTemplate"].(map[string]any)["@id"] = derivedFromTemplateID
		}
		data["@id"] = did
		if metadata, ok := topLevelValue(data, "metadata").(map[string]any); ok {
			metadata["@id"] = did + "#metadata"
		}
		if structure, ok := topLevelValue(data, "documentStructure").(map[string]any); ok {
			structure["@id"] = did + "#document-structure"
		}
	}
	return encodeDocumentData(data)
}

func rebaseDocumentIDs(value any, previousID string, did string, aliases []string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if text, ok := nested.(string); ok {
				if rebased, ok := rebaseIDText(text, previousID, did, aliases); ok {
					typed[key] = rebased
					continue
				}
			}
			rebaseDocumentIDs(nested, previousID, did, aliases)
		}
	case []any:
		for _, nested := range typed {
			rebaseDocumentIDs(nested, previousID, did, aliases)
		}
	}
}

func rebaseIDText(text, previousID, did string, aliases []string) (string, bool) {
	switch {
	case strings.HasPrefix(text, "urn:uuid:"):
		return did + "#" + strings.TrimPrefix(text, "urn:uuid:"), true
	case previousID != "" && previousID != did && strings.HasPrefix(text, previousID+"#"):
		return did + strings.TrimPrefix(text, previousID), true
	case previousID != "" && previousID != did && text == previousID:
		return did, true
	}
	for _, alias := range aliases {
		if alias == "" || alias == did {
			continue
		}
		if text == alias {
			return did, true
		}
		if strings.HasPrefix(text, alias+"#") {
			return did + strings.TrimPrefix(text, alias), true
		}
	}
	return "", false
}

func decodeDocumentData(raw *datatype.JSON) (documentData, error) {
	if raw == nil || !raw.IsNotNullValue() {
		return documentData{}, nil
	}
	var data map[string]any
	if err := json.Unmarshal(*raw, &data); err != nil {
		return nil, fmt.Errorf("document data is not valid JSON: %w", err)
	}
	if data == nil {
		data = map[string]any{}
	}
	return data, nil
}

func encodeDocumentData(data documentData) (*datatype.JSON, error) {
	normalized, err := datatype.NewJSON(map[string]any(data))
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

func isCanonicalEnvelope(data documentData) bool {
	_, hasPrefixedDocumentStructure := data["dcs:documentStructure"]
	_, hasDocumentStructure := data["documentStructure"]
	return hasPrefixedDocumentStructure || hasDocumentStructure
}

func normalizeCanonicalEnvelope(data documentData, documentType string) {
	normalizeCanonicalContext(data)
	if rawType, _ := data["@type"].(string); strings.TrimSpace(rawType) == "" {
		data["@type"] = documentType
	}
	// Anchors are set once, at production time: a document keeps the hub
	// versions it was authored under.
	if _, exists := data["sh:shapesGraph"]; !exists {
		data["sh:shapesGraph"] = map[string]any{"@id": currentSHACLShapesRef()}
	}
	declareShapeLibraries(data)
	if _, ok := topLevelValue(data, "contractData").([]any); !ok {
		if _, exists := topLevelValueExists(data, "contractData"); !exists {
			setTopLevelValue(data, "dcs:contractData", []any{})
		}
	}
	if _, ok := topLevelValue(data, "contractFields").([]any); !ok {
		if _, exists := topLevelValueExists(data, "contractFields"); !exists {
			setTopLevelValue(data, "dcs:contractFields", []any{})
		}
	}
	if _, ok := topLevelValue(data, "policies").([]any); !ok {
		if _, exists := topLevelValueExists(data, "policies"); !exists {
			setTopLevelValue(data, "dcs:policies", []any{})
		}
	}
	typeLayoutNodes(data)
}

// declareShapeLibraries adds to the document's own sh:shapesGraph the
// versioned anchor of every registered hub shapes library that governs a
// class the document's data uses. Validation loads exactly the declared
// graphs, so this declaration is what keeps a data object modelled against a
// registered library under that library's constraints (ADR-23) — and what
// keeps every other registered library out of the verdict.
//
// A library the document already declares keeps the version it was authored
// under: anchors are added, never rewritten (ADR-8). "Already declared" is
// name AND version, so a document that arrives pre-declaring an OLD version of
// a library still gets the active anchor added: a submitter cannot pick which
// version of a library governs its data by naming a laxer one first, and the
// document ends up validated against both.
func declareShapeLibraries(data documentData) {
	anchors := currentShapeLibraryAnchors()
	if len(anchors) == 0 {
		return
	}
	declared := map[shapesGraphAnchor]bool{}
	for _, anchor := range declaredShapesGraphs(data) {
		declared[anchor] = true
	}
	var added []any
	for _, class := range assertedTypeIRIs(data) {
		anchor, governed := anchors[class]
		if !governed {
			continue
		}
		declaration := shapesGraphAnchor{Name: anchor.Name, Version: anchorVersion(anchor.URL)}
		if declared[declaration] {
			continue
		}
		declared[declaration] = true
		added = append(added, map[string]any{"@id": anchor.URL})
	}
	if len(added) == 0 {
		return
	}
	existing, isList := data["sh:shapesGraph"].([]any)
	if !isList {
		existing = []any{data["sh:shapesGraph"]}
	}
	data["sh:shapesGraph"] = append(existing, added...)
}

// assertedTypeIRIs collects every @type the document asserts anywhere in its
// graph, sorted — the anchor list a document ends up with must not depend on
// map iteration order.
func assertedTypeIRIs(data documentData) []string {
	seen := map[string]bool{}
	var types []string
	collect := func(value any) {
		iri, ok := value.(string)
		if !ok || seen[iri] {
			return
		}
		seen[iri] = true
		types = append(types, iri)
	}
	var walk func(node any)
	walk = func(node any) {
		switch typed := node.(type) {
		case map[string]any:
			switch asserted := typed["@type"].(type) {
			case string:
				collect(asserted)
			case []any:
				for _, entry := range asserted {
					collect(entry)
				}
			}
			for key, value := range typed {
				if key != "@type" {
					walk(value)
				}
			}
		case []any:
			for _, entry := range typed {
				walk(entry)
			}
		}
	}
	walk(map[string]any(data))
	slices.Sort(types)
	return types
}

// typeLayoutNodes asserts rdf:type on the document's layout nodes — the
// hub shapes constrain dcs:layout values with sh:class dcs:LayoutNode, and
// SHACL class targeting needs the explicit type assertion.
func typeLayoutNodes(data documentData) {
	structure, ok := topLevelValue(data, "documentStructure").(map[string]any)
	if !ok {
		return
	}
	rawLayout := topLevelValue(documentData(structure), "layout")
	nodes, ok := jsonLDList(rawLayout)
	if !ok {
		nodes, ok = rawLayout.([]any)
	}
	if !ok {
		return
	}
	for _, rawNode := range nodes {
		node, ok := rawNode.(map[string]any)
		if !ok {
			continue
		}
		if existing, _ := node["@type"].(string); existing == "" {
			node["@type"] = "dcs:LayoutNode"
		}
	}
}

// normalizeCanonicalContext anchors "@context" to the Semantic Hub's
// versioned context URL, keeping a submitted inline prefix map alongside
// it in JSON-LD array form. A document whose @context already carries a
// URL entry keeps it.
func normalizeCanonicalContext(data documentData) {
	anchor := currentJSONLDContextRef()
	switch context := data["@context"].(type) {
	case string:
		if isHubContextAnchor(context) {
			return
		}
		data["@context"] = []any{anchor, context}
	case []any:
		for _, entry := range context {
			if url, ok := entry.(string); ok && isHubContextAnchor(url) {
				return
			}
		}
		data["@context"] = append([]any{anchor}, context...)
	case map[string]any:
		data["@context"] = []any{anchor, context}
	default:
		data["@context"] = anchor
	}
}

// validateExternalContextsResolvable rejects documents whose "@context"
// references an external context IRI that is not registered in the Semantic
// Hub — validation resolves contexts hermetically, so an unregistered IRI
// would make every later audit of the document fail.
func validateExternalContextsResolvable(data documentData) error {
	iris := externalContextIRIs(data)
	if len(iris) == 0 || activeShapeSource == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, iri := range iris {
		if _, err := activeShapeSource.ContextByIRI(ctx, iri); err != nil {
			return fmt.Errorf("%w: @context references %q, which is not registered in the Semantic Hub", ErrDocumentSchemaConflict, iri)
		}
	}
	return nil
}

// expectedPolicyTypes reflects the policy lifecycle: a template's policy
// set is an odrl:Offer, a contract instance remains an odrl:Offer through
// negotiation and becomes an odrl:Agreement when signing completes.
func expectedPolicyTypes(documentType string) []string {
	if documentType == "dcs:Contract" {
		return []string{"Offer", "Agreement"}
	}
	return []string{"Offer"}
}

func validateCanonicalEnvelope(data documentData, policyTypes []string) error {
	documentStructure, ok := topLevelValue(data, "documentStructure").(map[string]any)
	if !ok {
		return errors.New("documentStructure must be an object")
	}
	if containsODRLTerms(documentStructure) {
		return errors.New("documentStructure must not contain ODRL policy terms")
	}
	if metadata, exists := topLevelValueExists(data, "metadata"); exists {
		if _, ok := metadata.(map[string]any); !ok {
			return errors.New("metadata must be an object")
		}
	}
	if contractData, exists := topLevelValueExists(data, "contractData"); exists {
		if _, ok := contractData.([]any); !ok {
			return errors.New("contractData must be an array")
		}
	}
	if contractFields, exists := topLevelValueExists(data, "contractFields"); exists {
		if _, ok := contractFields.([]any); !ok {
			return errors.New("contractFields must be an array")
		}
	}
	if policies, exists := topLevelValueExists(data, "policies"); exists {
		if err := validateODRLPoliciesShape(policies, policyTypes); err != nil {
			return err
		}
	}
	return validateCanonicalReferences(data, documentStructure)
}

func validateCanonicalReferences(data documentData, documentStructure map[string]any) error {
	blocks, ok := jsonLDList(documentStructure["dcs:blocks"])
	if !ok {
		blocks, ok = documentStructure["dcs:blocks"].([]any)
	}
	if !ok {
		return errors.New("documentStructure.dcs:blocks must be an array")
	}
	layout, ok := jsonLDList(documentStructure["dcs:layout"])
	if !ok {
		layout, ok = documentStructure["dcs:layout"].([]any)
	}
	if !ok {
		return errors.New("documentStructure.dcs:layout must be an array")
	}

	blockIDs := map[string]bool{}
	blockTypes := map[string]string{}
	for index, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			return fmt.Errorf("documentStructure.dcs:blocks.%d must be an object", index)
		}
		id, _ := block["@id"].(string)
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("documentStructure.dcs:blocks.%d.@id is required", index)
		}
		if blockIDs[id] {
			return fmt.Errorf("duplicate document block @id %q", id)
		}
		blockIDs[id] = true
		blockTypes[id], _ = block["@type"].(string)
	}

	referencedBlocks := map[string]bool{}
	rootCount := 0
	for index, rawNode := range layout {
		node, ok := rawNode.(map[string]any)
		if !ok {
			return fmt.Errorf("documentStructure.dcs:layout.%d must be an object", index)
		}
		nodeID, _ := node["@id"].(string)
		if isTrueValue(node["dcs:isRoot"]) {
			rootCount++
		} else if !blockIDs[nodeID] {
			return fmt.Errorf("layout references nonexistent block %q", nodeID)
		}
		children, ok := jsonLDList(node["dcs:children"])
		if !ok {
			return fmt.Errorf("documentStructure.dcs:layout.%d.dcs:children must be an @list", index)
		}
		for _, rawChild := range children {
			child, ok := rawChild.(map[string]any)
			if !ok {
				return fmt.Errorf("layout child in %q must be an @id reference", nodeID)
			}
			childID, _ := child["@id"].(string)
			if !blockIDs[childID] {
				return fmt.Errorf("layout references nonexistent block %q", childID)
			}
			referencedBlocks[childID] = true
		}
	}
	if rootCount != 1 {
		return fmt.Errorf("documentStructure must contain exactly one root layout node, got %d", rootCount)
	}
	for blockID := range blockIDs {
		if !referencedBlocks[blockID] {
			if blockTypes[blockID] == "dcs:Clause" {
				continue
			}
			return fmt.Errorf("document block %q is not referenced by layout", blockID)
		}
	}

	fieldIDs, err := canonicalFieldIDs(data)
	if err != nil {
		return err
	}
	for _, rawBlock := range blocks {
		block := rawBlock.(map[string]any)
		if err := validateBlockFieldReferences(block, fieldIDs); err != nil {
			return err
		}
	}
	if err := validateContractDataGraph(data, fieldIDs); err != nil {
		return err
	}
	return validatePolicyOperands(data, fieldIDs)
}

// canonicalFieldIDs indexes the document's top-level dcs:ContractField
// declarations. Contract data objects, clause content and ODRL operands refer
// to these declarations by bare {"@id": ...} references.
func canonicalFieldIDs(data documentData) (map[string]bool, error) {
	contractFields, _ := topLevelValue(data, "contractFields").([]any)
	fieldIDs := map[string]bool{}
	for index, rawField := range contractFields {
		field, ok := rawField.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("contractFields.%d must be a ContractField object", index)
		}
		id, _ := field["@id"].(string)
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("contractFields.%d.@id is required", index)
		}
		if fieldIDs[id] {
			return nil, fmt.Errorf("duplicate contract field @id %q", id)
		}
		if compactTerm(fmt.Sprint(field["@type"])) != "ContractField" {
			return nil, fmt.Errorf("contract field %q must have @type dcs:ContractField", id)
		}
		if strings.TrimSpace(stringMapValue(field, "dcs:label")) == "" {
			return nil, fmt.Errorf("contract field %q has no dcs:label", id)
		}
		if strings.TrimSpace(stringMapValue(field, "dcs:datatype")) == "" {
			return nil, fmt.Errorf("contract field %q has no dcs:datatype", id)
		}
		if _, exists := field["dcs:required"]; !exists {
			return nil, fmt.Errorf("contract field %q has no dcs:required", id)
		}
		if _, ok := field["dcs:required"].(bool); !ok {
			return nil, fmt.Errorf("contract field %q dcs:required must be boolean", id)
		}
		fieldIDs[id] = true
	}
	return fieldIDs, nil
}

// validateBlockFieldReferences checks that every field reference in clause
// content resolves to a top-level ContractField declaration.
func validateBlockFieldReferences(block map[string]any, fieldIDs map[string]bool) error {
	content, ok := jsonLDList(block["dcs:content"])
	if !ok {
		return nil
	}
	for _, rawSegment := range content {
		segment, ok := rawSegment.(map[string]any)
		if !ok {
			continue // a plain text run
		}
		id, _ := segment["@id"].(string)
		if strings.TrimSpace(id) == "" {
			continue
		}
		if !fieldIDs[id] {
			return fmt.Errorf("clause content references nonexistent contract field %q", id)
		}
	}
	return nil
}

// validateContractDataGraph enforces the contract-data object graph: every
// domain object is a typed node named by @id; a property value is a literal
// (fixed data), a reference to a declared ContractField (a negotiable leaf),
// or a reference to another domain object (structure, arbitrary depth). Every
// reference must resolve in-document — the graph stays self-contained, so
// SHACL traversal and rendering never consult anything outside the document.
func validateContractDataGraph(data documentData, fieldIDs map[string]bool) error {
	documentID, _ := data["@id"].(string)
	contractData, _ := topLevelValue(data, "contractData").([]any)
	objectIDs := map[string]bool{}
	for index, rawObject := range contractData {
		object, ok := rawObject.(map[string]any)
		if !ok {
			return fmt.Errorf("contractData.%d must be a domain object", index)
		}
		id, _ := object["@id"].(string)
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("contractData.%d needs an @id: a domain object is an addressable graph node", index)
		}
		if objectType, _ := object["@type"].(string); strings.TrimSpace(objectType) == "" {
			return fmt.Errorf("contractData.%d.@type is required", index)
		}
		objectIDs[id] = true
	}
	for index, rawObject := range contractData {
		object := rawObject.(map[string]any)
		for property, rawValue := range object {
			if strings.HasPrefix(property, "@") {
				continue
			}
			values, _ := asArray(rawValue)
			if values == nil {
				values = []any{rawValue}
			}
			for _, value := range values {
				if err := validateContractDataValue(value, documentID, fieldIDs, objectIDs); err != nil {
					return fmt.Errorf("contractData.%d.%s %w", index, property, err)
				}
			}
		}
	}
	return nil
}

// validateContractDataValue admits the value kinds of the contract-data
// graph — a JSON literal, a typed {"@value"} literal, a single-key {"@id"}
// reference into the document, or a single-key {"@id"} absolute IRI naming an
// external resource (an sh:nodeKind sh:IRI leaf) — and nothing else. A
// document-scoped @id must resolve within the document: it is a dangling
// internal reference, not an external resource.
func validateContractDataValue(value any, documentID string, fieldIDs, objectIDs map[string]bool) error {
	object, ok := value.(map[string]any)
	if !ok {
		return nil // a bare JSON literal: fixed data
	}
	if id, isRef := object["@id"].(string); isRef && len(object) == 1 {
		if fieldIDs[id] || objectIDs[id] || isExternalResourceIRI(id, documentID) {
			return nil
		}
		return fmt.Errorf("references %q, which is neither a declared contract field nor a domain object in this document", id)
	}
	if _, isLiteral := object["@value"]; isLiteral {
		return nil // a typed literal
	}
	return errors.New("must be a literal, a contract-field reference, or a {\"@id\"} reference to another domain object — an embedded blank node is not addressable")
}

// isExternalResourceIRI reports whether an unresolved @id names a resource
// outside the document: an absolute IRI that is neither draft-scoped
// (urn:uuid:, the editor's pre-registration node scheme) nor scoped to the
// document's own IRI.
func isExternalResourceIRI(id, documentID string) bool {
	if strings.HasPrefix(id, "urn:uuid:") {
		return false
	}
	if documentID != "" && (id == documentID || strings.HasPrefix(id, documentID+"#")) {
		return false
	}
	return strings.Contains(id, "://") || strings.HasPrefix(id, "did:") || strings.HasPrefix(id, "urn:")
}

func validatePolicyOperands(data documentData, fieldIDs map[string]bool) error {
	policies := topLevelValue(data, "policies")
	rules := collectODRLPolicyRules(policies)
	for index, policy := range rules {
		switch compactTerm(fmt.Sprint(policy["@type"])) {
		case "Duty", "Permission", "Prohibition":
		default:
			return fmt.Errorf("policies.%d has unsupported @type %q", index, policy["@type"])
		}
		// A rule's constraints are a conjunction (ODRL IM §2.5); each may itself
		// be a logical constraint (and/or/xone/andSequence) nesting more
		// constraints. Its duties (and their consequences) carry constraints too.
		// Flatten to leaf operands: each is a document data field or an ODRL
		// context operand (spatial, dateTime, …) evaluated at use-time — never a
		// field.
		constraints := policyConstraints(policy["odrl:constraint"])
		constraints = append(constraints, dutyConstraints(policy["odrl:duty"])...)
		for _, constraint := range constraints {
			for _, leaf := range compactConstraintLeaves(constraint) {
				leftOperand, _ := leaf["odrl:leftOperand"].(map[string]any)
				operandID, _ := leftOperand["@id"].(string)
				if operandID == "" || isODRLContextOperandTerm(operandID) {
					continue
				}
				if !fieldIDs[operandID] {
					return fmt.Errorf("policy references nonexistent contract data field %q", operandID)
				}
			}
		}
	}
	return nil
}

// compactConstraintLeaves flattens a (compact-form) constraint tree to its
// atomic leaves, descending through logical constraints (odrl:and/or/xone/
// andSequence, prefixed or bare).
func compactConstraintLeaves(constraint map[string]any) []map[string]any {
	for _, key := range []string{
		"odrl:and", "and", "odrl:or", "or", "odrl:xone", "xone", "odrl:andSequence", "andSequence",
	} {
		raw, ok := constraint[key]
		if !ok {
			continue
		}
		leaves := []map[string]any{}
		children, _ := asArray(raw)
		if children == nil {
			if child, ok := raw.(map[string]any); ok {
				children = []any{child}
			}
		}
		for _, child := range children {
			if node, ok := child.(map[string]any); ok {
				leaves = append(leaves, compactConstraintLeaves(node)...)
			}
		}
		return leaves
	}
	return []map[string]any{constraint}
}

// dutyConstraints collects the constraint nodes carried by a rule's duties and,
// recursively, their consequence duties (ODRL IM §2.6.6) — so a duty constraint
// referencing a nonexistent data field is caught wherever it nests.
func dutyConstraints(raw any) []map[string]any {
	out := []map[string]any{}
	for _, duty := range policyConstraints(raw) {
		out = append(out, policyConstraints(duty["odrl:constraint"])...)
		out = append(out, dutyConstraints(duty["odrl:consequence"])...)
	}
	return out
}

// policyConstraints normalizes a rule's odrl:constraint to a list — a JSON-LD
// property with one value may be a bare object or a one-element array; a
// conjunction is an array.
func policyConstraints(raw any) []map[string]any {
	if items, ok := asArray(raw); ok {
		constraints := make([]map[string]any, 0, len(items))
		for _, item := range items {
			if constraint, ok := item.(map[string]any); ok {
				constraints = append(constraints, constraint)
			}
		}
		return constraints
	}
	if constraint, ok := raw.(map[string]any); ok {
		return []map[string]any{constraint}
	}
	return nil
}

// isODRLContextOperandTerm reports whether a left-operand @id (compact
// "odrl:spatial" or a full IRI) is an ODRL context operand.
func isODRLContextOperandTerm(operandID string) bool {
	return odrlContextOperandIRIs[odrlIRI+compactTerm(operandID)]
}

// odrlRuleBucketKeys are the ODRL 2.2 rule-bucket properties an enclosing
// odrl:Set may carry.
var odrlRuleBucketKeys = []string{"odrl:permission", "odrl:prohibition", "odrl:obligation"}

// collectODRLPolicyRules flattens dcs:policies into a plain list of rule
// nodes. Only the canonical shape yields rules: a single enclosing odrl:Set
// whose rules live in the odrl:duty/odrl:permission/odrl:prohibition/
// odrl:obligation bucket properties. An array (the empty "no policies yet"
// default; non-empty bare rule arrays are rejected by
// validateODRLPoliciesShape before they can be persisted) yields none.
func collectODRLPolicyRules(policies any) []map[string]any {
	set, ok := policies.(map[string]any)
	if !ok {
		return nil
	}
	return collectODRLSetRules(set)
}

func collectODRLSetRules(set map[string]any) []map[string]any {
	rules := []map[string]any{}
	for _, key := range odrlRuleBucketKeys {
		bucket, ok := set[key]
		if !ok {
			continue
		}
		if items, ok := asArray(bucket); ok {
			for _, item := range items {
				if rule, ok := item.(map[string]any); ok {
					rules = append(rules, rule)
				}
			}
			continue
		}
		if rule, ok := bucket.(map[string]any); ok {
			rules = append(rules, rule)
		}
	}
	return rules
}

// validateODRLPoliciesShape enforces the structural contract for
// dcs:policies:
//
//   - An empty array is accepted (no policies declared yet — the default
//     normalizeCanonicalEnvelope produces for a brand-new document).
//   - A non-empty bare rule array (Duty/Permission/Prohibition nodes with no
//     odrl:action, no enclosing odrl:Set, no parties/target) is explicitly
//     REJECTED — such a shape is not consumable by a standard ODRL processor.
//   - A single enclosing odrl:Set object is validated structurally: it must
//     declare odrl:profile and a uid, and every contained rule must declare
//     exactly one odrl:action plus odrl:assigner/odrl:assignee/odrl:target.
func validateODRLPoliciesShape(policies any, policyTypes []string) error {
	switch typed := policies.(type) {
	case []any:
		if len(typed) == 0 {
			return nil
		}
		return errors.New("dcs:policies is a bare rule array (no enclosing policy node, " +
			"no odrl:action, and no odrl:assigner/odrl:assignee/odrl:target), which is not accepted; " +
			"policies must form a single enclosing odrl:" + policyTypeLabel(policyTypes) + " declaring odrl:profile, whose rules each carry " +
			"exactly one odrl:action plus odrl:assigner, odrl:assignee, and odrl:target")
	case map[string]any:
		return validateODRLPolicySet(typed, policyTypes)
	default:
		return fmt.Errorf("dcs:policies must be an odrl:%s object (or an empty array), got %T", policyTypeLabel(policyTypes), policies)
	}
}

func policyTypeLabel(policyTypes []string) string {
	return strings.Join(policyTypes, " or odrl:")
}

func validateODRLPolicySet(set map[string]any, policyTypes []string) error {
	if !slices.Contains(policyTypes, compactTerm(fmt.Sprint(set["@type"]))) {
		return fmt.Errorf("dcs:policies enclosing node @type must be odrl:%s, got %v", policyTypeLabel(policyTypes), set["@type"])
	}
	if _, hasDutyBucket := set["odrl:duty"]; hasDutyBucket {
		return errors.New("odrl:duty is not a policy-level property in ODRL 2.2 (a Duty hangs under a Permission); policy-level duties belong under odrl:obligation")
	}
	if _, hasUID := set["uid"]; hasUID {
		return errors.New("the policy's identity is its @id (the ODRL JSON-LD context maps uid to @id); a separate uid key is not accepted")
	}
	if id, _ := set["@id"].(string); strings.TrimSpace(id) == "" {
		return fmt.Errorf("dcs:policies odrl:%s requires an @id (its odrl:uid)", policyTypeLabel(policyTypes))
	}
	if _, hasProfile := set["odrl:profile"]; !hasProfile {
		return fmt.Errorf("dcs:policies odrl:%s must declare odrl:profile", policyTypeLabel(policyTypes))
	}
	rules := collectODRLSetRules(set)
	for index, rule := range rules {
		if err := validateODRLRuleShape(rule); err != nil {
			return fmt.Errorf("dcs:policies rule %d: %w", index, err)
		}
	}
	return nil
}

func validateODRLRuleShape(rule map[string]any) error {
	switch compactTerm(fmt.Sprint(rule["@type"])) {
	case "Duty", "Permission", "Prohibition":
	default:
		return fmt.Errorf("unsupported rule @type %v", rule["@type"])
	}
	action, hasAction := rule["odrl:action"]
	if !hasAction {
		return errors.New("rule is missing odrl:action")
	}
	// A rule may declare several actions (ODRL Policy Rule Composition §2.7).
	if items, ok := action.([]any); ok && len(items) == 0 {
		return errors.New("rule must declare at least one odrl:action")
	}
	for _, key := range []string{"odrl:assigner", "odrl:assignee", "odrl:target"} {
		if _, ok := rule[key]; !ok {
			return fmt.Errorf("rule is missing %s", key)
		}
	}
	prose, _ := rule["dcs:prose"].(map[string]any)
	proseID, _ := prose["@id"].(string)
	if strings.TrimSpace(proseID) == "" {
		return errors.New("rule is missing dcs:prose — every machine-readable rule must reference the human-readable clause it is backed by")
	}
	// A Permission may carry duties (ODRL IM §2.6.5): obligations the assignee
	// must fulfil to exercise it. Each is a fragment — its own odrl:action (and
	// optional constraints/consequence), inheriting the enclosing rule's
	// parties, so it declares no assigner/assignee/target of its own.
	if rawDuty, hasDuty := rule["odrl:duty"]; hasDuty {
		if compactTerm(fmt.Sprint(rule["@type"])) != "Permission" {
			return errors.New("odrl:duty may only be attached to a Permission (a duty nests under the rule it obliges); a policy-level Duty belongs under odrl:obligation")
		}
		for index, duty := range policyConstraints(rawDuty) {
			if err := validateODRLDutyFragment(duty); err != nil {
				return fmt.Errorf("odrl:duty %d: %w", index, err)
			}
		}
	}
	return nil
}

// validateODRLDutyFragment validates a Duty nested under a Permission (ODRL IM
// §2.6.5). A duty fragment inherits its parties from the enclosing rule, so it
// needs an odrl:action and the prose backing every odrl:Duty owes (the hub's
// dcs:OdrlDutyProseShape refuses a document at submit otherwise); its
// consequence is itself a duty, validated the same way (recursively).
func validateODRLDutyFragment(duty map[string]any) error {
	if t := compactTerm(fmt.Sprint(duty["@type"])); t != "" && t != "Duty" {
		return fmt.Errorf("a duty must be an odrl:Duty, got %v", duty["@type"])
	}
	action, hasAction := duty["odrl:action"]
	if !hasAction {
		return errors.New("duty is missing odrl:action")
	}
	if items, ok := action.([]any); ok && len(items) == 0 {
		return errors.New("duty must declare at least one odrl:action")
	}
	prose, _ := duty["dcs:prose"].(map[string]any)
	if proseID, _ := prose["@id"].(string); strings.TrimSpace(proseID) == "" {
		return errors.New("duty is missing dcs:prose — a nested duty is a machine-readable rule too, so it must reference the human-readable clause it is backed by")
	}
	if rawConsequence, ok := duty["odrl:consequence"]; ok {
		for index, consequence := range policyConstraints(rawConsequence) {
			if err := validateODRLDutyFragment(consequence); err != nil {
				return fmt.Errorf("odrl:consequence %d: %w", index, err)
			}
		}
	}
	return nil
}

func jsonLDList(value any) ([]any, bool) {
	container, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	items, ok := container["@list"].([]any)
	return items, ok
}

func isTrueValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func topLevelValue(data documentData, localName string) any {
	value, _ := topLevelValueExists(data, localName)
	return value
}

func topLevelValueExists(data documentData, localName string) (any, bool) {
	if value, ok := data["dcs:"+localName]; ok {
		return value, true
	}
	value, ok := data[localName]
	return value, ok
}

func setTopLevelValue(data documentData, key string, value any) {
	data[key] = value
}

func containsODRLTerms(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if strings.HasPrefix(key, "odrl:") {
				return true
			}
			if raw, ok := nested.(string); ok && strings.HasPrefix(raw, "odrl:") {
				return true
			}
			if containsODRLTerms(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if containsODRLTerms(nested) {
				return true
			}
		}
	}
	return false
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	default:
		return 0, false
	}
}

func asArray(value any) ([]any, bool) {
	switch items := value.(type) {
	case []any:
		return items, true
	case []map[string]any:
		result := make([]any, len(items))
		for i, item := range items {
			result[i] = item
		}
		return result, true
	case []string:
		result := make([]any, len(items))
		for i, item := range items {
			result[i] = item
		}
		return result, true
	default:
		return nil, false
	}
}

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}
