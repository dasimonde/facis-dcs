package semantichub

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"digital-contracting-service/internal/base/validation"

	"github.com/piprate/json-gold/ld"
	"github.com/tggo/goRDFlib/shacl"
)

// ValidateSchemaContent checks whether content can be used as a hub entry of
// the given kind. Callers that gate Register or Activate should use this so
// admission matches what runtime loaders already accept.
func ValidateSchemaContent(kind, content string) error {
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("schema content must not be empty")
	}
	switch kind {
	case "shapes":
		return validateShapesTurtle(content)
	case "ontology":
		return validateOntologyTurtle(content)
	case "profile":
		_, err := validation.LoadValidationProfileYAML([]byte(content))
		if err != nil {
			return fmt.Errorf("profile schema content is not a valid validation profile: %w", err)
		}
		return nil
	case "context":
		return validateContextJSONLD(content)
	default:
		return fmt.Errorf("unsupported schema kind %q", kind)
	}
}

func validateShapesTurtle(content string) error {
	graph, err := loadAdmissionTurtle(content, "urn:dcs:hub:admission:shapes")
	if err != nil {
		return err
	}
	rdfType := shacl.IRI(shacl.RDFType)
	nodeShapes := graph.Subjects(rdfType, shacl.IRI(shacl.SH+"NodeShape"))
	propertyShapes := graph.Subjects(rdfType, shacl.IRI(shacl.SH+"PropertyShape"))
	if len(nodeShapes) == 0 && len(propertyShapes) == 0 {
		return fmt.Errorf("shapes schema content must declare at least one sh:NodeShape or sh:PropertyShape")
	}
	if err := validateShapesStructure(graph); err != nil {
		return err
	}
	return ensureShapesEngineAccepts(graph)
}

// validateShapesStructure catches shapes the Turtle parser accepts but that
// cannot be used as SHACL: every sh:property needs sh:path, and count/length
// parameters must be integers.
func validateShapesStructure(graph *shacl.Graph) error {
	propPred := shacl.IRI(shacl.SH + "property")
	pathPred := shacl.IRI(shacl.SH + "path")
	for _, triple := range graph.All(nil, &propPred, nil) {
		if len(graph.Objects(triple.Object, pathPred)) == 0 {
			return fmt.Errorf("shapes schema content has sh:property without sh:path")
		}
	}
	for _, typed := range graph.Subjects(shacl.IRI(shacl.RDFType), shacl.IRI(shacl.SH+"PropertyShape")) {
		if len(graph.Objects(typed, pathPred)) == 0 {
			return fmt.Errorf("shapes schema content has sh:PropertyShape without sh:path")
		}
	}

	for _, name := range []string{
		"minCount", "maxCount", "minLength", "maxLength",
		"qualifiedMinCount", "qualifiedMaxCount",
	} {
		pred := shacl.IRI(shacl.SH + name)
		for _, triple := range graph.All(nil, &pred, nil) {
			raw := strings.Trim(strings.TrimSpace(triple.Object.Value()), `"`)
			if _, err := strconv.Atoi(raw); err != nil {
				return fmt.Errorf("shapes schema content has non-integer sh:%s value %q", name, raw)
			}
		}
	}
	return nil
}

// ensureShapesEngineAccepts runs the same SHACL engine the audit path uses so
// a shapes graph that panics or is otherwise unusable is refused at admission.
func ensureShapesEngineAccepts(shapesGraph *shacl.Graph) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("shapes schema content is not usable by the SHACL engine: %v", recovered)
		}
	}()
	empty, loadErr := shacl.LoadTurtleString("@prefix ex: <urn:dcs:hub:admission:empty#> .", "urn:dcs:hub:admission:empty")
	if loadErr != nil {
		return fmt.Errorf("shapes schema content could not be checked: %w", loadErr)
	}
	_ = shacl.Validate(empty, shapesGraph)
	return nil
}

var allowedParameterTypes = map[string]struct{}{
	"string":  {},
	"decimal": {},
	"integer": {},
	"boolean": {},
	"date":    {},
	"enum":    {},
}

func validateOntologyTurtle(content string) error {
	graph, err := loadAdmissionTurtle(content, "urn:dcs:hub:admission:ontology")
	if err != nil {
		return err
	}
	if len(graph.All(nil, nil, nil)) == 0 {
		return fmt.Errorf("ontology schema content must contain at least one RDF triple")
	}
	return validateAppliesToParameterTypes(graph)
}

// validateAppliesToParameterTypes checks dcs:appliesToParameterType (any
// namespace) against the ContractField parameter-type vocabulary. Matching is
// by RDF local name so ontology prefix IRIs may change without bypassing the gate.
func validateAppliesToParameterTypes(graph *shacl.Graph) error {
	for _, triple := range graph.All(nil, nil, nil) {
		if rdfLocalName(triple.Predicate.Value()) != "appliesToParameterType" {
			continue
		}
		value := strings.Trim(strings.TrimSpace(triple.Object.Value()), `"`)
		if _, ok := allowedParameterTypes[value]; !ok {
			return fmt.Errorf("ontology schema content has invalid appliesToParameterType %q (allowed: string, decimal, integer, boolean, date, enum)", value)
		}
	}
	return nil
}

func rdfLocalName(iri string) string {
	if i := strings.LastIndexAny(iri, "#/"); i >= 0 && i+1 < len(iri) {
		return iri[i+1:]
	}
	return iri
}

func loadAdmissionTurtle(content, baseIRI string) (*shacl.Graph, error) {
	graph, err := shacl.LoadTurtleString(content, baseIRI)
	if err != nil {
		return nil, fmt.Errorf("schema content is not valid Turtle: %w", err)
	}
	return graph, nil
}

func validateContextJSONLD(content string) error {
	var doc struct {
		Context any `json:"@context"`
	}
	if err := json.Unmarshal([]byte(content), &doc); err != nil {
		return fmt.Errorf("context schema content must be a JSON-LD document with a non-empty @context object")
	}
	if !contextObjectNonEmpty(doc.Context) {
		return fmt.Errorf("context schema content must be a JSON-LD document with a non-empty @context object")
	}

	// Same processor the audit path uses (json-gold Expand). A probe node is
	// enough to force context processing — including @container / @type maps —
	// without depending on document-specific terms.
	probe := map[string]any{
		"@context": doc.Context,
		"@id":      "urn:dcs:hub:admission:context-probe",
	}
	options := ld.NewJsonLdOptions("")
	if _, err := ld.NewJsonLdProcessor().Expand(probe, options); err != nil {
		return fmt.Errorf("context schema content is not a usable JSON-LD context: %w", err)
	}
	return nil
}

func contextObjectNonEmpty(ctx any) bool {
	switch v := ctx.(type) {
	case map[string]any:
		return len(v) > 0
	case []any:
		for _, item := range v {
			if contextObjectNonEmpty(item) {
				return true
			}
		}
		return false
	case string:
		return strings.TrimSpace(v) != ""
	default:
		return false
	}
}
