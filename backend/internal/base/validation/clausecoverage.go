package validation

import (
	"context"
	"fmt"
	"strings"

	"github.com/tggo/goRDFlib/shacl"
)

// Applicable-checks establishment for PACM-07 clause reporting.
const (
	ApplicableChecksNotEstablished = "not_established"
	ApplicableChecksNone           = "none"
	ApplicableChecksEstablished    = "established"
)

// Clause coverage / verdict statuses (PACM-07). Compliant is only emitted when
// applicable checks are established and all passed — never from empty findings alone.
const (
	ClauseStatusNotAssessed      = "not_assessed"
	ClauseStatusUndetermined     = "undetermined"
	ClauseStatusWarningsOnly     = "warnings_only"
	ClauseStatusIssuesIdentified = "issues_identified"
	ClauseStatusCompliant        = "compliant"
)

// ShapeTargetIndex maps RDF type local-names / IRIs to NodeShape display names
// that sh:targetClass that type. Built from the same shapes TTL AuditContractContent uses.
type ShapeTargetIndex map[string][]string

// ClauseCoverage is the per-clause coverage projection used to derive status.
type ClauseCoverage struct {
	// ApplicableChecks is not_established | none | established.
	ApplicableChecks string
	// ApplicableCheckIDs are NodeShape local names (or ODRL/profile rule ids) that apply.
	ApplicableCheckIDs []string
	// Status is the PACM-07 clause verdict.
	Status string
	// VerdictNote explains undetermined / not_assessed when needed.
	VerdictNote string
}

// BuildShapeTargetIndex parses shapes Turtle for NodeShape → sh:targetClass links.
func BuildShapeTargetIndex(shapesTTL string) (ShapeTargetIndex, error) {
	shapesTTL = strings.TrimSpace(shapesTTL)
	if shapesTTL == "" {
		return nil, fmt.Errorf("empty shapes turtle")
	}
	graph, err := shacl.LoadTurtleString(shapesTTL, "urn:dcs:clause-coverage:shapes")
	if err != nil {
		return nil, fmt.Errorf("parse shapes for coverage: %w", err)
	}
	targetClassPred := shacl.IRI(shacl.SH + "targetClass")
	index := ShapeTargetIndex{}
	for _, triple := range graph.All(nil, &targetClassPred, nil) {
		shapeIRI := strings.TrimSpace(triple.Subject.Value())
		classIRI := strings.TrimSpace(triple.Object.Value())
		if shapeIRI == "" || classIRI == "" {
			continue
		}
		shapeName := iriLocalName(shapeIRI)
		for _, key := range typeMatchKeys(classIRI) {
			index[key] = appendUnique(index[key], shapeName)
		}
	}
	return index, nil
}

// LoadAuditShapeTargetIndex loads the shapes bundle AuditContractContent validates
// against and indexes sh:targetClass. Returns not_established semantics when the
// shape source is unavailable.
func LoadAuditShapeTargetIndex(ctx context.Context, contractDocument any) (ShapeTargetIndex, error) {
	source, err := requireShapeSource()
	if err != nil {
		return nil, err
	}
	contract, err := normalizeObject(contractDocument)
	if err != nil {
		return nil, err
	}
	shapesTTL, _, err := declaredShapes(ctx, contract, source)
	if err != nil {
		return nil, err
	}
	return BuildShapeTargetIndex(shapesTTL)
}

// ApplicableShapesForClause returns NodeShape names whose targetClass matches
// one of the clause's @types. Bare structural typing alone does not imply checks.
func ApplicableShapesForClause(clause ClauseComponent, index ShapeTargetIndex) []string {
	if index == nil {
		return nil
	}
	out := make([]string, 0)
	for _, typ := range clause.Types {
		for _, key := range typeMatchKeys(typ) {
			out = append(out, index[key]...)
		}
	}
	return uniqueStable(out)
}

// ClauseCoverageInput carries everything needed to derive a clause verdict.
type ClauseCoverageInput struct {
	Clause      ClauseComponent
	Attributed  []PolicyFinding
	AllFindings []PolicyFinding
	AllClauses  []ClauseComponent
	AuditRan    bool
	IndexOK     bool
	Index       ShapeTargetIndex
}

// ApplicableFieldRulesForClause discovers ODRL/profile (and other) rule IDs whose
// FocusNode / FieldIri / Path uniquely ties to this clause's Refs. Shared refs
// that also appear on other clauses are reported as ambiguous (coverage incomplete).
func ApplicableFieldRulesForClause(clause ClauseComponent, allFindings []PolicyFinding, allClauses []ClauseComponent) (applicable []string, ambiguous bool) {
	if len(clause.Refs) == 0 || len(allFindings) == 0 {
		return nil, false
	}
	refOwners := buildRefOwners(allClauses)
	clauseRefSet := map[string]struct{}{}
	for _, ref := range clause.Refs {
		for _, key := range refMatchKeys(ref) {
			clauseRefSet[key] = struct{}{}
		}
	}

	for _, f := range allFindings {
		ruleID := findingRuleLabel(f)
		if ruleID == "" {
			continue
		}
		for _, cand := range findingFieldCandidates(f) {
			keys := refMatchKeys(cand)
			matchedThis := false
			for _, key := range keys {
				if _, ok := clauseRefSet[key]; ok {
					matchedThis = true
					break
				}
			}
			if !matchedThis {
				continue
			}
			// How many clauses share this candidate field?
			owners := ownersForCandidate(cand, refOwners)
			if len(owners) > 1 {
				ambiguous = true
				continue
			}
			if len(owners) == 1 && owners[0] == clause.ID {
				applicable = appendUnique(applicable, ruleID)
			}
		}
	}
	return uniqueStable(applicable), ambiguous
}

// DeriveClauseCoverage computes applicable checks + status from coverage index,
// attributed findings, and field-ref discovery across the full audit finding set.
func DeriveClauseCoverage(in ClauseCoverageInput) ClauseCoverage {
	if !in.AuditRan {
		return ClauseCoverage{
			ApplicableChecks: ApplicableChecksNotEstablished,
			Status:           ClauseStatusNotAssessed,
			VerdictNote:      "Content-policy / SHACL validation did not complete for this report.",
		}
	}

	errors, warnings, satisfied, deferred := partitionAttributedSeverities(in.Attributed)

	if !in.IndexOK {
		if len(errors) > 0 {
			return ClauseCoverage{
				ApplicableChecks: ApplicableChecksNotEstablished,
				Status:           ClauseStatusIssuesIdentified,
			}
		}
		if len(warnings) > 0 {
			return ClauseCoverage{
				ApplicableChecks: ApplicableChecksNotEstablished,
				Status:           ClauseStatusWarningsOnly,
			}
		}
		return ClauseCoverage{
			ApplicableChecks: ApplicableChecksNotEstablished,
			Status:           ClauseStatusUndetermined,
			VerdictNote:      "Available validation results do not establish a clause-level compliance verdict.",
		}
	}

	shapeApp := ApplicableShapesForClause(in.Clause, in.Index)
	fieldApp, fieldAmbiguous := ApplicableFieldRulesForClause(in.Clause, in.AllFindings, in.AllClauses)

	applicable := append([]string(nil), shapeApp...)
	applicable = append(applicable, fieldApp...)
	for _, f := range satisfied {
		if id := findingRuleLabel(f); id != "" {
			applicable = appendUnique(applicable, id)
		}
	}
	applicable = uniqueStable(applicable)

	if len(applicable) == 0 && !fieldAmbiguous {
		cov := ClauseCoverage{
			ApplicableChecks: ApplicableChecksNone,
			Status:           ClauseStatusNotAssessed,
			VerdictNote:      "No clause-targeted shapes or attributable checks apply to this clause.",
		}
		if len(errors) > 0 {
			cov.Status = ClauseStatusIssuesIdentified
			cov.VerdictNote = ""
		} else if len(warnings) > 0 {
			cov.Status = ClauseStatusWarningsOnly
			cov.VerdictNote = ""
		}
		return cov
	}

	cov := ClauseCoverage{
		ApplicableChecks:   ApplicableChecksEstablished,
		ApplicableCheckIDs: applicable,
	}
	if len(applicable) == 0 && fieldAmbiguous {
		cov.ApplicableChecks = ApplicableChecksNotEstablished
		cov.ApplicableCheckIDs = nil
	}

	if len(errors) > 0 {
		cov.Status = ClauseStatusIssuesIdentified
		return cov
	}
	if len(warnings) > 0 {
		cov.Status = ClauseStatusWarningsOnly
		return cov
	}
	// Deferred checks or ambiguous field attribution: cannot claim Compliant.
	if len(deferred) > 0 || (fieldAmbiguous && len(shapeApp) == 0) {
		cov.Status = ClauseStatusUndetermined
		cov.VerdictNote = "Available validation results do not establish a clause-level compliance verdict."
		if fieldAmbiguous && len(shapeApp) == 0 {
			cov.VerdictNote = "Field-linked checks touch multiple clauses; attribution is incomplete."
		}
		return cov
	}
	// Shape-targeted clause with no defects, or uniquely attributed field checks
	// that all passed (satisfied / SHACL violation-only silence).
	if len(applicable) == 0 {
		cov.Status = ClauseStatusUndetermined
		cov.VerdictNote = "Available validation results do not establish a clause-level compliance verdict."
		return cov
	}
	cov.Status = ClauseStatusCompliant
	return cov
}

func findingRuleLabel(f PolicyFinding) string {
	// Domain-field checks: show contract/ontology label plus the taxonomy @id,
	// e.g. `Payment Amount (https://w3id.org/facis/dcs/taxonomy/v1#field-contract-payment-amount)`.
	if strings.EqualFold(strings.TrimSpace(f.Source), "domain-ontology") {
		iri := strings.TrimSpace(f.OntologyTerm)
		if iri == "" {
			iri = strings.TrimSpace(f.RuleID)
		}
		label := strings.TrimSpace(f.Title)
		if label == "" || label == iri || label == iriLocalName(iri) {
			if iri != "" {
				return iri
			}
		} else if iri != "" {
			return fmt.Sprintf("%s (%s)", label, iri)
		} else if label != "" {
			return label
		}
	}
	if id := strings.TrimSpace(f.RuleID); id != "" {
		return id
	}
	return strings.TrimSpace(f.Title)
}

func findingFieldCandidates(f PolicyFinding) []string {
	return uniqueNonEmpty(f.FocusNode, f.FieldIri, f.Path)
}

func refMatchKeys(ref string) []string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	keys := []string{ref, strings.ToLower(ref)}
	local := iriLocalName(ref)
	if local != "" {
		keys = append(keys, local, strings.ToLower(local))
	}
	return uniqueStable(keys)
}

func buildRefOwners(clauses []ClauseComponent) map[string][]string {
	owners := map[string][]string{}
	for _, c := range clauses {
		for _, ref := range c.Refs {
			for _, key := range refMatchKeys(ref) {
				owners[key] = appendUnique(owners[key], c.ID)
			}
		}
	}
	return owners
}

func ownersForCandidate(candidate string, refOwners map[string][]string) []string {
	out := make([]string, 0)
	for _, key := range refMatchKeys(candidate) {
		out = append(out, refOwners[key]...)
	}
	return uniqueStable(out)
}

func partitionAttributedSeverities(findings []PolicyFinding) (errors, warnings, satisfied, deferred []PolicyFinding) {
	for _, f := range findings {
		sev := strings.ToLower(strings.TrimSpace(f.Severity))
		switch sev {
		case SeveritySatisfied:
			satisfied = append(satisfied, f)
		case SeverityWarning:
			warnings = append(warnings, f)
		case SeverityInfo:
			warnings = append(warnings, f)
		case SeverityDeferred:
			deferred = append(deferred, f)
		case SeverityError, "violation", "failed", "":
			if sev == "" && strings.TrimSpace(f.Message) == "" {
				continue
			}
			errors = append(errors, f)
		default:
			errors = append(errors, f)
		}
	}
	return errors, warnings, satisfied, deferred
}

func typeMatchKeys(typ string) []string {
	typ = strings.TrimSpace(typ)
	if typ == "" {
		return nil
	}
	keys := []string{typ, strings.ToLower(typ)}
	local := iriLocalName(typ)
	if local != "" && local != typ {
		keys = append(keys, local, "dcs:"+local)
	}
	// CURIE dcs:Foo → also Foo
	if i := strings.IndexByte(typ, ':'); i > 0 && !strings.Contains(typ, "://") {
		local = typ[i+1:]
		keys = append(keys, local)
	}
	return uniqueStable(keys)
}

func appendUnique(in []string, v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return in
	}
	for _, existing := range in {
		if existing == v {
			return in
		}
	}
	return append(in, v)
}

func uniqueStable(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// FormatApplicableChecksLabel is the UI/API display string for applicable checks.
func FormatApplicableChecksLabel(cov ClauseCoverage) string {
	switch cov.ApplicableChecks {
	case ApplicableChecksNone:
		return "None"
	case ApplicableChecksEstablished:
		if len(cov.ApplicableCheckIDs) == 0 {
			return "None"
		}
		return strings.Join(cov.ApplicableCheckIDs, ", ")
	default:
		return "Not established"
	}
}
