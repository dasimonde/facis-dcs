package validation

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/tggo/goRDFlib/shacl"
)

const skosNamespace = "http://www.w3.org/2004/02/skos/core#"

// domainOntology is the parsed active SLA domain ontology (Semantic Hub
// name="facis-sla" kind="ontology"): the dcs:DomainField index keyed by
// field IRI — a domain field's IRI is its identity — plus the taxonomy
// prefix that compacts controlled-vocabulary contract-party role IRIs.
type domainOntology struct {
	fields           map[string]domainField
	entityRolePrefix string
}

var (
	domainOntologyMu     sync.Mutex
	cachedDomainOntology *domainOntology
)

// requireDomainOntology loads the domain-field index from the installed
// ShapeSource on first use and caches it. Hub versions are immutable, so
// the cache lives until a new source is installed (SetShapeSource) or the
// hub activates a new version (ResetDomainOntologyCache).
func requireDomainOntology(ctx context.Context) (*domainOntology, error) {
	domainOntologyMu.Lock()
	defer domainOntologyMu.Unlock()
	if cachedDomainOntology != nil {
		return cachedDomainOntology, nil
	}
	source, err := requireShapeSource()
	if err != nil {
		return nil, err
	}
	content, version, err := source.ActiveDomainOntology(ctx)
	if err != nil {
		return nil, fmt.Errorf("load domain ontology: %w", err)
	}
	ontology, err := parseDomainOntology(content)
	if err != nil {
		return nil, fmt.Errorf("parse domain ontology (hub version %d): %w", version, err)
	}
	cachedDomainOntology = ontology
	return ontology, nil
}

// ResetDomainOntologyCache drops the cached domain-field index so the next
// audit re-reads the hub's active ontology version — called on hub
// activation (service.RefreshValidationAnchors).
func ResetDomainOntologyCache() {
	domainOntologyMu.Lock()
	defer domainOntologyMu.Unlock()
	cachedDomainOntology = nil
}

func parseDomainOntology(content string) (*domainOntology, error) {
	graph, err := shacl.LoadTurtleString(content, "urn:dcs:hub:domain-ontology")
	if err != nil {
		return nil, err
	}
	dcs := dcsNamespace()

	valueOptions := parseOntologyValueOptions(graph)
	constraints := map[string]*valueConstraint{}
	for _, subject := range graph.Subjects(shacl.IRI(shacl.RDFType), shacl.IRI(dcs+"ValueConstraint")) {
		constraints[subject.Value()] = parseOntologyValueConstraint(graph, subject, dcs, valueOptions)
	}
	resolveAllowedValuesRefs(constraints)

	fields := map[string]domainField{}
	for _, subject := range graph.Subjects(shacl.IRI(shacl.RDFType), shacl.IRI(dcs+"DomainField")) {
		iri := subject.Value()
		if firstObjectValue(graph, subject, shacl.RDFS+"label") == "" {
			return nil, fmt.Errorf("domain field %s requires rdfs:label", iri)
		}
		if firstObjectValue(graph, subject, shacl.RDFS+"range") == "" {
			return nil, fmt.Errorf("domain field %s requires rdfs:range", iri)
		}
		field := domainField{IRI: iri}
		if constraintIRI := firstObjectValue(graph, subject, dcs+"hasValueConstraint"); constraintIRI != "" {
			constraint, ok := constraints[constraintIRI]
			if !ok {
				return nil, fmt.Errorf("domain field %s references unknown value constraint %s", iri, constraintIRI)
			}
			field.Constraint = constraint.clone()
		}
		fields[iri] = field
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("ontology does not define dcs:DomainField entries")
	}
	return &domainOntology{
		fields:           fields,
		entityRolePrefix: parseEntityRolePrefix(graph, dcs),
	}, nil
}

// parseEntityRolePrefix derives the IRI prefix of controlled-vocabulary
// contract-party role values (…/taxonomy/v1#role-…): the namespace of the
// role property's value constraint plus the conventional "role-" local-name
// prefix its individuals carry.
func parseEntityRolePrefix(graph *shacl.Graph, dcs string) string {
	for _, property := range graph.Subjects(shacl.IRI(shacl.RDFS+"range"), shacl.IRI(dcs+"ContractPartyRoleCode")) {
		if constraintIRI := firstObjectValue(graph, property, dcs+"hasValueConstraint"); constraintIRI != "" {
			return iriNamespace(constraintIRI) + "role-"
		}
	}
	return ""
}

// iriNamespace returns the IRI up to and including its last '#' or '/'.
func iriNamespace(iri string) string {
	if index := strings.LastIndexAny(iri, "#/"); index >= 0 {
		return iri[:index+1]
	}
	return iri
}

func parseOntologyValueConstraint(graph *shacl.Graph, subject shacl.Term, dcs string, catalogOptions map[string]valueOption) *valueConstraint {
	allowedValues := objectValues(graph, subject, dcs+"allowedValue")
	options := make([]valueOption, 0, len(allowedValues))
	for _, value := range allowedValues {
		if option, ok := catalogOptions[value]; ok {
			options = append(options, option)
		}
	}
	return &valueConstraint{
		Format:           firstObjectValue(graph, subject, dcs+"format"),
		Pattern:          firstObjectValue(graph, subject, dcs+"pattern"),
		ValueType:        firstObjectValue(graph, subject, dcs+"valueType"),
		AllowedValues:    allowedValues,
		ValueOptions:     options,
		AllowedValuesRef: firstObjectValue(graph, subject, dcs+"allowedValuesRef"),
		Min:              firstObjectNumber(graph, subject, dcs+"minInclusive"),
		Max:              firstObjectNumber(graph, subject, dcs+"maxInclusive"),
		Description:      firstObjectValue(graph, subject, shacl.RDFS+"label"),
	}
}

// parseOntologyValueOptions indexes the taxonomy's skos value concepts by
// their notation — the display metadata behind a constraint's allowed
// values.
func parseOntologyValueOptions(graph *shacl.Graph) map[string]valueOption {
	dcs := dcsNamespace()
	notation := shacl.IRI(skosNamespace + "notation")
	options := map[string]valueOption{}
	for _, triple := range graph.All(nil, &notation, nil) {
		value := triple.Object.Value()
		if value == "" {
			continue
		}
		options[value] = valueOption{
			Value:  value,
			Label:  englishObjectValue(graph, triple.Subject, skosNamespace+"prefLabel"),
			Symbol: firstObjectValue(graph, triple.Subject, dcs+"valueSymbol"),
			IRI:    triple.Subject.Value(),
		}
	}
	return options
}

// resolveAllowedValuesRefs fills a constraint that declares only an
// allowedValuesRef from a sibling constraint sharing the same reference, so
// consumers never chase the reference themselves.
func resolveAllowedValuesRefs(constraints map[string]*valueConstraint) {
	for _, constraint := range constraints {
		if len(constraint.AllowedValues) > 0 {
			continue
		}
		ref := normalizedAllowedValuesRef(constraint.AllowedValuesRef)
		if ref == "" {
			continue
		}
		for _, candidate := range constraints {
			if candidate == constraint || normalizedAllowedValuesRef(candidate.AllowedValuesRef) != ref {
				continue
			}
			if len(candidate.AllowedValues) > 0 {
				constraint.AllowedValues = append([]string(nil), candidate.AllowedValues...)
				constraint.ValueOptions = append([]valueOption(nil), candidate.ValueOptions...)
				break
			}
		}
	}
}

func normalizedAllowedValuesRef(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func firstObjectValue(graph *shacl.Graph, subject shacl.Term, predicate string) string {
	objects := graph.Objects(subject, shacl.IRI(predicate))
	if len(objects) == 0 {
		return ""
	}
	return objects[0].Value()
}

// englishObjectValue prefers the @en literal among a predicate's values.
func englishObjectValue(graph *shacl.Graph, subject shacl.Term, predicate string) string {
	objects := graph.Objects(subject, shacl.IRI(predicate))
	for _, object := range objects {
		if object.Language() == "en" {
			return object.Value()
		}
	}
	if len(objects) == 0 {
		return ""
	}
	return objects[0].Value()
}

func objectValues(graph *shacl.Graph, subject shacl.Term, predicate string) []string {
	objects := graph.Objects(subject, shacl.IRI(predicate))
	values := make([]string, 0, len(objects))
	for _, object := range objects {
		values = append(values, object.Value())
	}
	return values
}

func firstObjectNumber(graph *shacl.Graph, subject shacl.Term, predicate string) *float64 {
	raw := firstObjectValue(graph, subject, predicate)
	if raw == "" {
		return nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return &value
}
