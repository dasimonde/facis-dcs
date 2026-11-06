package validation

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ContractFieldInfo is one top-level dcs:ContractField used for clause coverage.
type ContractFieldInfo struct {
	ID       string
	ShapeIRI string
	Label    string
	Value    any
	HasValue bool
}

// IndexContractFields indexes ContractField nodes by @id, capturing dcs:shape
// semantic identifiers and inline values. Does not mutate the document.
func IndexContractFields(contractDocument any) (map[string]ContractFieldInfo, error) {
	doc, err := normalizeObject(contractDocument)
	if err != nil {
		return nil, err
	}
	out := map[string]ContractFieldInfo{}
	seen := map[string]bool{}
	var walk func(any)
	walk = func(current any) {
		switch value := current.(type) {
		case map[string]any:
			if rawFields, ok := topLevelValue(documentData(value), "contractFields").([]any); ok {
				for _, rawField := range rawFields {
					field, ok := rawField.(map[string]any)
					if !ok {
						continue
					}
					fieldID := strings.TrimSpace(asString(field["@id"]))
					if fieldID == "" || seen[fieldID] {
						continue
					}
					seen[fieldID] = true
					info := ContractFieldInfo{ID: fieldID}
					if label, _ := firstOf(field, "dcs:label", "label").(string); label != "" {
						info.Label = label
					}
					info.ShapeIRI = contractFieldShapeIRI(field)
					info.Value, info.HasValue = inlineFieldValue(field)
					out[fieldID] = info
				}
			}
			for _, nested := range value {
				walk(nested)
			}
		case []any:
			for _, nested := range value {
				walk(nested)
			}
		}
	}
	walk(doc)
	return out, nil
}

func contractFieldShapeIRI(field map[string]any) string {
	switch shape := field["dcs:shape"].(type) {
	case map[string]any:
		return strings.TrimSpace(asString(shape["@id"]))
	case string:
		return strings.TrimSpace(shape)
	default:
		return ""
	}
}

// AssessClauseDomainFields evaluates ontology ValueConstraints for fields
// referenced by each clause's dcs:content. Emits PolicyFindings with FocusNode
// = field @id so MapFindingsToClauses can attribute them to the parent clause
// when the field ref is unique. Compliant requires these executed results —
// a matching dcs:shape alone is not enough.
func AssessClauseDomainFields(ctx context.Context, clauses []ClauseComponent, fields map[string]ContractFieldInfo) ([]PolicyFinding, error) {
	if len(clauses) == 0 || len(fields) == 0 {
		return nil, nil
	}
	ontology, err := requireDomainOntology(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]PolicyFinding, 0)
	seenField := map[string]struct{}{}
	for _, clause := range clauses {
		for _, ref := range clause.Refs {
			field, ok := fields[ref]
			if !ok {
				continue
			}
			if _, done := seenField[field.ID]; done {
				continue
			}
			// Only assess when exactly one clause owns this field ref —
			// otherwise leave to unassigned / Undetermined path.
			if owners := clausesOwningRef(ref, clauses); len(owners) != 1 {
				continue
			}
			seenField[field.ID] = struct{}{}

			shapeIRI := strings.TrimSpace(field.ShapeIRI)
			if shapeIRI == "" {
				continue
			}
			df, ok := lookupDomainField(ontology, shapeIRI)
			if !ok {
				continue
			}
			// Unknown shape → no finding (not grounds for Compliant).
			// Known DomainField → always emit an executed result; ValueConstraint
			// rules run when present, otherwise presence/type against rdfs:range.
			out = append(out, evaluateDomainField(field, df)...)
		}
	}
	return out, nil
}

func clausesOwningRef(ref string, clauses []ClauseComponent) []string {
	owners := make([]string, 0)
	keys := refMatchKeys(ref)
	for _, c := range clauses {
		for _, r := range c.Refs {
			for _, key := range refMatchKeys(r) {
				for _, want := range keys {
					if key == want {
						owners = appendUnique(owners, c.ID)
					}
				}
			}
		}
	}
	return uniqueStable(owners)
}

func lookupDomainField(ontology *domainOntology, shapeIRI string) (domainField, bool) {
	if ontology == nil {
		return domainField{}, false
	}
	if f, ok := ontology.fields[shapeIRI]; ok {
		return f, true
	}
	// Tolerate CURIE / local-name variants against full taxonomy IRIs.
	for iri, f := range ontology.fields {
		for _, key := range typeMatchKeys(shapeIRI) {
			for _, cand := range typeMatchKeys(iri) {
				if key == cand {
					return f, true
				}
			}
		}
	}
	return domainField{}, false
}

func evaluateDomainField(field ContractFieldInfo, df domainField) []PolicyFinding {
	shapeIRI := strings.TrimSpace(df.IRI)
	label := domainFieldDisplayLabel(field, df)

	base := PolicyFinding{
		RuleID:       shapeIRI, // taxonomy DomainField @id (stable identity)
		Title:        label,    // human label, e.g. "Payment Amount"
		Path:         field.ID,
		FieldIri:     field.ID, // ContractField @id — MapFindingsToClauses matches clause Refs
		FocusNode:    field.ID,
		Source:       "domain-ontology",
		OntologyTerm: shapeIRI,
	}

	if !field.HasValue {
		base.Severity = SeverityError
		base.Message = fmt.Sprintf("%s requires a value", label)
		return []PolicyFinding{base}
	}

	actual := compactAuditValue(field.Value)
	actualStr := fmt.Sprint(actual)

	if df.Constraint != nil {
		return evaluateDomainValueConstraint(base, df, actual, actualStr)
	}

	// No ValueConstraint: still execute a presence/type check against rdfs:range
	// so payment-amount (and similar) are not silent.
	if isNumericXSDRange(df.Range) {
		if _, ok := asFloat(actual); !ok {
			base.Severity = SeverityError
			base.Message = fmt.Sprintf("%s expects a numeric value", label)
			base.ActualValue = actual
			return []PolicyFinding{base}
		}
	}
	base.Severity = SeveritySatisfied
	base.Message = fmt.Sprintf("%s present", label)
	base.ActualValue = actual
	return []PolicyFinding{base}
}

// domainFieldDisplayLabel prefers the contract field's dcs:label, then the
// ontology rdfs:label, then the shape local name.
func domainFieldDisplayLabel(field ContractFieldInfo, df domainField) string {
	if label := strings.TrimSpace(field.Label); label != "" {
		return label
	}
	if label := strings.TrimSpace(df.Label); label != "" {
		return label
	}
	if local := iriLocalName(df.IRI); local != "" {
		return local
	}
	return strings.TrimSpace(df.IRI)
}

func evaluateDomainValueConstraint(base PolicyFinding, df domainField, actual any, actualStr string) []PolicyFinding {
	constraint := df.Constraint
	label := base.Title
	if label == "" {
		label = base.RuleID
	}

	if len(constraint.AllowedValues) > 0 {
		if !valueInAllowed(actualStr, constraint.AllowedValues) {
			base.Severity = SeverityError
			base.Message = fmt.Sprintf("%s value %q is not in the allowed set", label, actualStr)
			base.ActualValue = actual
			base.ExpectedValues = stringSliceToAny(constraint.AllowedValues)
			return []PolicyFinding{base}
		}
	}
	if pattern := strings.TrimSpace(constraint.Pattern); pattern != "" {
		re, err := regexp.Compile(pattern)
		if err != nil {
			base.Severity = SeverityDeferred
			base.Message = fmt.Sprintf("%s pattern is not evaluable", label)
			return []PolicyFinding{base}
		}
		if !re.MatchString(actualStr) {
			base.Severity = SeverityError
			base.Message = fmt.Sprintf("%s value %q does not match pattern %s", label, actualStr, pattern)
			base.ActualValue = actual
			return []PolicyFinding{base}
		}
	}
	if constraint.Min != nil || constraint.Max != nil {
		num, ok := asFloat(actual)
		if !ok {
			base.Severity = SeverityError
			base.Message = fmt.Sprintf("%s expects a numeric value", label)
			base.ActualValue = actual
			return []PolicyFinding{base}
		}
		if constraint.Min != nil && num < *constraint.Min {
			base.Severity = SeverityError
			base.Message = fmt.Sprintf("%s value %v is below minimum %v", label, num, *constraint.Min)
			base.ActualValue = actual
			base.ExpectedValue = *constraint.Min
			return []PolicyFinding{base}
		}
		if constraint.Max != nil && num > *constraint.Max {
			base.Severity = SeverityError
			base.Message = fmt.Sprintf("%s value %v is above maximum %v", label, num, *constraint.Max)
			base.ActualValue = actual
			base.ExpectedValue = *constraint.Max
			return []PolicyFinding{base}
		}
	}

	base.Severity = SeveritySatisfied
	base.Message = fmt.Sprintf("%s satisfied", label)
	base.ActualValue = actual
	return []PolicyFinding{base}
}

func isNumericXSDRange(rangeIRI string) bool {
	local := strings.ToLower(iriLocalName(rangeIRI))
	switch local {
	case "decimal", "integer", "int", "long", "short", "byte",
		"nonnegativeinteger", "positiveinteger", "float", "double":
		return true
	default:
		return false
	}
}

func valueInAllowed(actual string, allowed []string) bool {
	actual = strings.TrimSpace(actual)
	for _, a := range allowed {
		a = strings.TrimSpace(a)
		if a == actual {
			return true
		}
		// Allowed values may be full IRIs; also compare local names / notations.
		if iriLocalName(a) == actual || iriLocalName(a) == iriLocalName(actual) {
			return true
		}
	}
	return false
}

func asFloat(v any) (float64, bool) {
	switch t := v.(type) {
	case float64:
		return t, true
	case float32:
		return float64(t), true
	case int:
		return float64(t), true
	case int64:
		return float64(t), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f, err == nil
	default:
		f, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(v)), 64)
		return f, err == nil
	}
}

func stringSliceToAny(in []string) []any {
	out := make([]any, 0, len(in))
	for _, s := range in {
		out = append(out, s)
	}
	return out
}
