package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/piprate/json-gold/ld"
)

const (
	defaultDCSOntologyIRI = "https://w3id.org/facis/dcs/ontology/v1#"
	odrlIRI               = "http://www.w3.org/ns/odrl/2/"
)

// dcsNamespace returns the dcs: ontology namespace the active hub context
// declares; the historical default applies until the hub anchors are
// installed.
func dcsNamespace() string {
	if iri, ok := currentCanonicalOntologyIRIs()["dcs"]; ok && iri != "" {
		return iri
	}
	return defaultDCSOntologyIRI
}

// expandForAudit returns the json-gold expansion of a document, resolved
// hermetically through the ShapeSource. The hub context is merged in as the
// outermost context so prefix-form keys resolve to full IRIs even when the
// document declares only a subset of the hub prefixes (conflicting
// redefinitions are already rejected at creation).
func expandForAudit(ctx context.Context, document map[string]any, source ShapeSource) (map[string]any, error) {
	var contextContent string
	var err error
	if pinned := pinnedHubContextVersion(document); pinned > 0 {
		contextContent, err = source.ContextAt(ctx, pinned)
	} else {
		contextContent, _, err = source.ActiveContext(ctx)
	}
	if err != nil {
		return nil, fmt.Errorf("load JSON-LD context: %w", err)
	}
	var hubContextDoc map[string]any
	if err := json.Unmarshal([]byte(contextContent), &hubContextDoc); err != nil {
		return nil, fmt.Errorf("parse hub JSON-LD context: %w", err)
	}

	input := make(map[string]any, len(document))
	for key, value := range document {
		input[key] = value
	}
	mergedContext := []any{hubContextDoc["@context"]}
	switch docContext := document["@context"].(type) {
	case []any:
		mergedContext = append(mergedContext, docContext...)
	case nil:
	default:
		mergedContext = append(mergedContext, docContext)
	}
	input["@context"] = mergedContext

	loader, err := hermeticContextLoader(ctx, contextContent, source)
	if err != nil {
		return nil, err
	}
	options := ld.NewJsonLdOptions("")
	options.DocumentLoader = loader
	expanded, err := ld.NewJsonLdProcessor().Expand(input, options)
	if err != nil {
		return nil, fmt.Errorf("JSON-LD expansion: %w", err)
	}
	if len(expanded) == 0 {
		return map[string]any{}, nil
	}
	root, ok := expanded[0].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("JSON-LD expansion: unexpected root node %T", expanded[0])
	}
	return root, nil
}

// expandedValues returns a property's value nodes on an expanded node.
func expandedValues(node map[string]any, property string) []any {
	values, _ := node[property].([]any)
	return values
}

func expandedFirst(node map[string]any, property string) (map[string]any, bool) {
	for _, value := range expandedValues(node, property) {
		if obj, ok := value.(map[string]any); ok {
			return obj, true
		}
	}
	return nil, false
}

func expandedID(value any) string {
	if obj, ok := value.(map[string]any); ok {
		id, _ := obj["@id"].(string)
		return id
	}
	return ""
}

// expandedLiteral unwraps an expanded value node to a plain Go value.
func expandedLiteral(value any) any {
	obj, ok := value.(map[string]any)
	if !ok {
		return value
	}
	if nested, ok := obj["@value"]; ok {
		return nested
	}
	if id, ok := obj["@id"].(string); ok {
		return id
	}
	if list, ok := obj["@list"].([]any); ok {
		return expandedLiterals(list)
	}
	return value
}

func expandedLiterals(values []any) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, expandedLiteral(value))
	}
	return out
}

func expandedFirstLiteralString(node map[string]any, property string) string {
	for _, value := range expandedValues(node, property) {
		if s, ok := expandedLiteral(value).(string); ok {
			return s
		}
	}
	return ""
}

// expandedTypeLocalName returns the local name of the node's first @type.
func expandedTypeLocalName(node map[string]any) string {
	if types, ok := node["@type"].([]any); ok {
		for _, t := range types {
			if iri, ok := t.(string); ok {
				return shaclLocalName(iri)
			}
		}
	}
	return ""
}

var odrlRuleBucketIRIs = []string{
	odrlIRI + "permission",
	odrlIRI + "prohibition",
	odrlIRI + "obligation",
}

// expandedODRLPolicyRules flattens the document's policies into rule nodes:
// a single enclosing policy (odrl:Offer on templates, odrl:Agreement on
// contracts) whose rules live under permission/prohibition/obligation.
func expandedODRLPolicyRules(root map[string]any) []map[string]any {
	rules := []map[string]any{}
	for _, rawSet := range expandedValues(root, dcsNamespace()+"policies") {
		set, ok := rawSet.(map[string]any)
		if !ok {
			continue
		}
		for _, bucket := range odrlRuleBucketIRIs {
			for _, rawRule := range expandedValues(set, bucket) {
				if rule, ok := rawRule.(map[string]any); ok {
					rules = append(rules, rule)
				}
			}
		}
	}
	return rules
}

// expandedODRLFieldIndex maps ContractField @ids to their label and inline
// dcs:value — the same IRI the ODRL
// constraint names as its odrl:leftOperand.
func expandedODRLFieldIndex(root map[string]any) map[string]odrlFieldInfo {
	index := map[string]odrlFieldInfo{}
	for _, rawField := range expandedValues(root, dcsNamespace()+"contractFields") {
		field, ok := rawField.(map[string]any)
		if !ok {
			continue
		}
		fieldID, _ := field["@id"].(string)
		if fieldID == "" {
			continue
		}
		value, hasValue := expandedInlineFieldValue(field)
		index[fieldID] = odrlFieldInfo{
			label:    expandedFirstLiteralString(field, dcsNamespace()+"label"),
			value:    value,
			hasValue: hasValue,
		}
	}
	return index
}

// expandedInlineFieldValue reads a ContractField's inline dcs:value.
func expandedInlineFieldValue(field map[string]any) (any, bool) {
	values := expandedValues(field, dcsNamespace()+"value")
	if len(values) == 0 {
		return nil, false
	}
	if len(values) == 1 {
		value := expandedLiteral(values[0])
		return value, value != nil
	}
	return expandedLiterals(values), true
}

// auditExpandedODRLPolicies evaluates every ODRL rule node against the
// values the document carries inline on its requirement fields.
func auditExpandedODRLPolicies(ctx context.Context, root map[string]any, rules []map[string]any) ([]PolicyFinding, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	fieldIndex := expandedODRLFieldIndex(root)
	applyDeclaredUnits(fieldIndex, rules)
	findings := []PolicyFinding{}
	for _, rule := range rules {
		ruleFindings, err := auditExpandedODRLRule(ctx, root, rule, fieldIndex)
		if err != nil {
			return nil, err
		}
		findings = append(findings, ruleFindings...)
	}
	return findings, nil
}

func auditExpandedODRLRule(ctx context.Context, _ map[string]any, rule map[string]any, fieldIndex map[string]odrlFieldInfo) ([]PolicyFinding, error) {
	ruleID, _ := rule["@id"].(string)
	if ruleID == "" {
		ruleID = "FACIS-CONTRACT-ODRL-POLICY"
	}
	policyType := expandedTypeLocalName(rule)
	isProhibition := policyType == "Prohibition"
	isPermission := policyType == "Permission"
	severity := "error"
	if isPermission {
		severity = "info"
	}

	findings, err := auditConstraintBearingNode(ctx, ruleID, rule, fieldIndex, isProhibition, isPermission, severity)
	if err != nil {
		return nil, err
	}

	// A permission's duties (ODRL IM §2.5) are obligations the assignee must
	// fulfil to exercise it. The obligated action is performed at use-time; the
	// audit records it and evaluates the duty's own constraints as obligations.
	dutyFindings, err := auditExpandedODRLDutyNodes(ctx, ruleID, expandedNodes(rule, odrlIRI+"duty"), fieldIndex)
	if err != nil {
		return nil, err
	}
	return append(findings, dutyFindings...), nil
}

// auditConstraintBearingNode audits the ODRL constraints carried by a rule or
// duty node (a conjunction, ODRL IM §2.5): each is evaluated, and any violated
// data-field constraint fails the node. isProhibition/isPermission set the
// violation semantics (a prohibition is violated when satisfied; an obligation
// when not); a duty is audited as an obligation.
func auditConstraintBearingNode(ctx context.Context, ruleID string, node map[string]any, fieldIndex map[string]odrlFieldInfo, isProhibition, isPermission bool, severity string) ([]PolicyFinding, error) {
	findings := []PolicyFinding{}
	for _, rawConstraint := range expandedValues(node, odrlIRI+"constraint") {
		constraint, ok := rawConstraint.(map[string]any)
		if !ok {
			continue
		}

		// A logical constraint (odrl:and/or/xone) is a tree of nested
		// constraints — evaluate it recursively rather than as an atomic leaf.
		// When its outcome depends on use-time context it is deferred.
		// odrl:andSequence additionally requires an order this audit cannot
		// observe, so it is audited separately and never called satisfied.
		if logicalOp, children, isLogical := constraintLogical(constraint); isLogical {
			if logicalOp == odrlAndSequence {
				finding, err := auditOrderedConjunction(ctx, ruleID, children, fieldIndex, isProhibition, isPermission, severity)
				if err != nil {
					return nil, err
				}
				findings = append(findings, finding)
				continue
			}
			satisfied, resolvable, err := evaluateConstraintNode(ctx, constraint, fieldIndex)
			if err != nil {
				return nil, fmt.Errorf("evaluate ODRL policy %q logical constraint: %w", ruleID, err)
			}
			if !resolvable {
				findings = append(findings, contractFinding(ruleID, ruleID, "info", fmt.Sprintf("ODRL policy %q %s-constraint is enforced at use-time", ruleID, logicalOp), odrlIRI+logicalOp, ""))
				continue
			}
			violated := (isProhibition && satisfied) || (!isProhibition && !isPermission && !satisfied)
			sev := "info"
			msg := fmt.Sprintf("ODRL policy %q logical (%s) constraint satisfied", ruleID, logicalOp)
			if violated {
				sev = severity
				msg = fmt.Sprintf("ODRL policy %q logical (%s) constraint violated", ruleID, logicalOp)
			}
			findings = append(findings, contractFinding(ruleID, ruleID, sev, msg, odrlIRI+logicalOp, ""))
			continue
		}

		leftOperand, ok := expandedFirst(constraint, odrlIRI+"leftOperand")
		if !ok {
			continue
		}
		operandID, _ := leftOperand["@id"].(string)
		if operandID == "" {
			continue
		}
		operatorNode, ok := expandedFirst(constraint, odrlIRI+"operator")
		if !ok {
			continue
		}
		operator := shaclLocalName(expandedID(operatorNode))
		if operator == "" {
			continue
		}
		rightOperand := resolveRightOperand(constraint, operator, fieldIndex)

		// odrl:unit denominates the boundary, but a dcs:ContractField carries a
		// bare value with no unit of its own, so nothing here can confirm the
		// two quantities are commensurable. Record the gap rather than let the
		// unit imply a check that does not happen.
		if unit := resolveConstraintUnit(constraint, fieldIndex); unit.token != "" {
			findings = append(findings, contractFinding(ruleID, ruleID, "warning",
				unitUncheckedMessage(ruleID, unit), operandID, odrlIRI+"unit"))
		}

		// ODRL context operands (spatial, dateTime, purpose, …) are evaluated
		// at use-time by the execution environment against the access context
		// it reports; the contract audit only records that they apply.
		//
		// "info" here means DEFERRED, not passed. Nothing in this deployment
		// closes the loop: the only use-time channel is the KPI callback, which
		// cannot see a constraint nested in a logical constraint or a duty, and
		// the deployment payload deploy.go hands the target system carries an
		// empty odrl:Set. The audit view buckets info with the passing
		// severities and renders it green — do not take a green row here as
		// evidence that the constraint was checked.
		if isODRLContextOperand(operandID) {
			finding := contractFinding(ruleID, ruleID, "info", fmt.Sprintf("ODRL policy %q constraint on %s is enforced at use-time", ruleID, shaclLocalName(operandID)), operandID, "")
			applyODRLPolicyDetails(&finding, operandID, operator, nil, false, rightOperand)
			findings = append(findings, finding)
			continue
		}

		fieldInfo, ok := fieldIndex[operandID]
		if !ok {
			finding := contractFinding(ruleID, ruleID, "error", fmt.Sprintf("ODRL policy %q references nonexistent contract field %q", ruleID, operandID), operandID, "dcs:ContractField")
			applyODRLPolicyDetails(&finding, operandID, operator, nil, false, rightOperand)
			findings = append(findings, finding)
			continue
		}
		if len(fieldInfo.units) > 1 {
			findings = append(findings, contractFinding(ruleID, ruleID, "error",
				fmt.Sprintf("ODRL policy %q is not evaluated: field %q is bounded in more than one unit (%s), and this audit has no conversion between them", ruleID, operandID, strings.Join(localNames(fieldInfo.units), ", ")),
				operandID, odrlIRI+"unit"))
			continue
		}
		actualValue, hasValue := fieldInfo.value, fieldInfo.hasValue

		if !hasValue {
			if isProhibition || isPermission {
				finding := contractFinding(ruleID, ruleID, "info", fmt.Sprintf("ODRL policy %q: value not present", ruleID), operandID, "")
				applyODRLPolicyDetails(&finding, operandID, operator, nil, false, rightOperand)
				findings = append(findings, finding)
				continue
			}
			finding := contractFinding(ruleID, ruleID, severity, fmt.Sprintf("ODRL policy %q: required value not provided", ruleID), operandID, "")
			applyODRLPolicyDetails(&finding, operandID, operator, nil, false, rightOperand)
			findings = append(findings, finding)
			continue
		}

		satisfied, err := evaluateODRLConstraintOPA(ctx, operator, actualValue, rightOperand)
		if err != nil {
			return nil, fmt.Errorf("evaluate ODRL policy %q: %w", ruleID, err)
		}
		violated := (isProhibition && satisfied) || (!isProhibition && !isPermission && !satisfied)
		if violated {
			finding := contractFinding(ruleID, ruleID, severity, fmt.Sprintf("ODRL policy %q violated: value %v does not satisfy %s", ruleID, actualValue, operator), operandID, "")
			applyODRLPolicyDetails(&finding, operandID, operator, actualValue, true, rightOperand)
			findings = append(findings, finding)
			continue
		}
		finding := contractFinding(ruleID, ruleID, "info", fmt.Sprintf("ODRL policy %q satisfied", ruleID), operandID, "")
		applyODRLPolicyDetails(&finding, operandID, operator, actualValue, true, rightOperand)
		findings = append(findings, finding)
	}
	return findings, nil
}

// auditOrderedConjunction audits an odrl:andSequence (ODRL IM §2.6): its
// operands must all be satisfied, and satisfied in the order declared. A
// contract document records no order of events, so the ordering half is not
// checkable here and the sequence is never reported as satisfied. An operand
// that definitely fails is still decisive: no ordering rescues it.
func auditOrderedConjunction(ctx context.Context, ruleID string, children []map[string]any, fieldIndex map[string]odrlFieldInfo, isProhibition, isPermission bool, severity string) (PolicyFinding, error) {
	for _, child := range children {
		satisfied, resolvable, err := evaluateConstraintNode(ctx, child, fieldIndex)
		if err != nil {
			return PolicyFinding{}, fmt.Errorf("evaluate ODRL policy %q andSequence constraint: %w", ruleID, err)
		}
		if !resolvable || satisfied {
			continue
		}
		if isProhibition || isPermission {
			return contractFinding(ruleID, ruleID, "info",
				fmt.Sprintf("ODRL policy %q ordered (andSequence) constraint does not hold: an operand is not satisfied", ruleID),
				odrlIRI+odrlAndSequence, ""), nil
		}
		return contractFinding(ruleID, ruleID, severity,
			fmt.Sprintf("ODRL policy %q ordered (andSequence) constraint violated: an operand is not satisfied, so no ordering of the operands satisfies it", ruleID),
			odrlIRI+odrlAndSequence, ""), nil
	}
	return contractFinding(ruleID, ruleID, "warning",
		fmt.Sprintf("ODRL policy %q ordered (andSequence) constraint is not evaluated: its operands are checked individually, but the order they must hold in is not, so this audit reaches no verdict on it", ruleID),
		odrlIRI+odrlAndSequence, ""), nil
}

// applyDeclaredUnits records, per contract field, the distinct odrl:unit IRIs
// the document's constraints denominate that field's boundaries in. A field
// bounded in two units at once cannot be compared against either boundary.
func applyDeclaredUnits(fieldIndex map[string]odrlFieldInfo, rules []map[string]any) {
	var walkConstraint func(constraint map[string]any)
	walkConstraint = func(constraint map[string]any) {
		if _, children, isLogical := constraintLogical(constraint); isLogical {
			for _, child := range children {
				walkConstraint(child)
			}
			return
		}
		unit := resolveConstraintUnit(constraint, fieldIndex)
		if unit.token == "" {
			return
		}
		leftOperand, ok := expandedFirst(constraint, odrlIRI+"leftOperand")
		if !ok {
			return
		}
		operandID, _ := leftOperand["@id"].(string)
		info, known := fieldIndex[operandID]
		if !known || slices.Contains(info.units, unit.token) {
			return
		}
		info.units = append(info.units, unit.token)
		fieldIndex[operandID] = info
	}
	var walkNode func(node map[string]any)
	walkNode = func(node map[string]any) {
		for _, raw := range expandedValues(node, odrlIRI+"constraint") {
			if constraint, ok := raw.(map[string]any); ok {
				walkConstraint(constraint)
			}
		}
		for _, duty := range expandedNodes(node, odrlIRI+"duty") {
			walkNode(duty)
		}
		for _, consequence := range expandedNodes(node, odrlIRI+"consequence") {
			walkNode(consequence)
		}
	}
	for _, rule := range rules {
		walkNode(rule)
	}
}

// odrlUnit is a constraint's resolved odrl:unit.
type odrlUnit struct {
	// token identifies the unit for the "are these the same unit?" test: the
	// declared IRI of a fixed unit, the agreed value of a negotiated one, and
	// the field @id while that field is unfilled — so constraints naming the
	// same unagreed field stay one unit, whichever unit it turns out to be.
	//
	// A dcs:ContractField holds a literal, so a negotiated unit's agreed value
	// is a notation ("EUR"), never the concept IRI a fixed unit names. The two
	// therefore never compare equal, and must not: mapping one to the other
	// would be guessing which scheme the notation came from.
	token string
	// fieldID is the contract field a negotiated unit names, empty for a fixed
	// unit; label is that field's label and agreed reports whether it is filled.
	fieldID string
	label   string
	agreed  bool
}

// resolveConstraintUnit returns the unit a constraint denominates its boundary
// in (ODRL IM §2.5), zero when it declares none. A unit naming a declared
// contract field is negotiated — the parties agree it the way they agree a
// right-operand boundary — and resolves to that field's filled value, mirroring
// resolveRightOperand.
func resolveConstraintUnit(constraint map[string]any, fieldIndex map[string]odrlFieldInfo) odrlUnit {
	for _, raw := range expandedValues(constraint, odrlIRI+"unit") {
		id := expandedID(raw)
		if id == "" {
			if literal, ok := expandedLiteral(raw).(string); ok && literal != "" {
				return odrlUnit{token: literal}
			}
			continue
		}
		info, negotiated := fieldIndex[id]
		if !negotiated {
			return odrlUnit{token: id}
		}
		if info.hasValue {
			return odrlUnit{token: fmt.Sprint(info.value), fieldID: id, label: info.label, agreed: true}
		}
		return odrlUnit{token: id, fieldID: id, label: info.label}
	}
	return odrlUnit{}
}

// unitUncheckedMessage states what the audit did not check about a boundary's
// unit. A dcs:ContractField carries a bare value with no unit of its own, so a
// declared unit is never compared against anything. A negotiated unit adds a
// second gap while unagreed — nothing yet says what the boundary is in — which
// ValidateContractClosed refuses to seal, so the audit states it and moves on.
func unitUncheckedMessage(ruleID string, unit odrlUnit) string {
	if unit.fieldID == "" {
		return fmt.Sprintf("ODRL policy %q denominates its boundary in %s, but the compared value declares no unit, so this audit does not verify the two are the same quantity", ruleID, shaclLocalName(unit.token))
	}
	name := unit.label
	if name == "" {
		name = shaclLocalName(unit.fieldID)
	}
	if !unit.agreed {
		return fmt.Sprintf("ODRL policy %q denominates its boundary in the negotiated unit %q, which has no agreed value, so this audit cannot tell what the boundary is measured in", ruleID, name)
	}
	return fmt.Sprintf("ODRL policy %q denominates its boundary in the negotiated unit %q, agreed as %s, but the compared value declares no unit, so this audit does not verify the two are the same quantity", ruleID, name, unit.token)
}

func localNames(iris []string) []string {
	names := make([]string, 0, len(iris))
	for _, iri := range iris {
		names = append(names, shaclLocalName(iri))
	}
	return names
}

// auditExpandedODRLDutyNodes audits a permission's duties (ODRL IM §2.5). Each
// duty records the obligated action (fulfilled at use-time) and has its own
// constraints evaluated as obligations; its consequence — a duty triggered
// when the duty is not fulfilled — is audited the same way, recursively.
func auditExpandedODRLDutyNodes(ctx context.Context, ownerID string, duties []map[string]any, fieldIndex map[string]odrlFieldInfo) ([]PolicyFinding, error) {
	findings := []PolicyFinding{}
	for _, duty := range duties {
		dutyID, _ := duty["@id"].(string)
		if dutyID == "" {
			dutyID = ownerID
		}
		findings = append(findings, contractFinding(ownerID, ownerID, "info",
			fmt.Sprintf("ODRL policy %q duty (%s) is fulfilled at use-time", ownerID, dutyActionLabel(duty)), odrlIRI+"duty", ""))

		constraintFindings, err := auditConstraintBearingNode(ctx, dutyID, duty, fieldIndex, false, false, "error")
		if err != nil {
			return nil, err
		}
		findings = append(findings, constraintFindings...)

		consequenceFindings, err := auditExpandedODRLDutyNodes(ctx, dutyID, expandedNodes(duty, odrlIRI+"consequence"), fieldIndex)
		if err != nil {
			return nil, err
		}
		findings = append(findings, consequenceFindings...)
	}
	return findings, nil
}

// expandedNodes returns a property's value nodes as maps, unwrapping an @list
// container (ODRL duty/consequence are plain sets, but a producer may serialize
// them as an ordered list).
func expandedNodes(node map[string]any, property string) []map[string]any {
	out := []map[string]any{}
	for _, raw := range expandedValues(node, property) {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if list, ok := item["@list"].([]any); ok {
			for _, li := range list {
				if child, ok := li.(map[string]any); ok {
					out = append(out, child)
				}
			}
			continue
		}
		out = append(out, item)
	}
	return out
}

// dutyActionLabel joins the local names of a duty's action(s) for a finding.
func dutyActionLabel(duty map[string]any) string {
	names := []string{}
	for _, raw := range expandedValues(duty, odrlIRI+"action") {
		if id := expandedID(raw); id != "" {
			names = append(names, shaclLocalName(id))
		}
	}
	if len(names) == 0 {
		return "action"
	}
	return strings.Join(names, ", ")
}

// odrlContextOperandIRIs is the full ODRL 2.2 core Left Operand vocabulary. A
// context operand names access/use context the enforcer reports at use-time
// (not a document data field), so the contract-time audit records that the
// constraint applies and defers its verdict rather than resolving it.
var odrlContextOperandIRIs = map[string]bool{
	odrlIRI + "absolutePosition":         true,
	odrlIRI + "absoluteSpatialPosition":  true,
	odrlIRI + "absoluteTemporalPosition": true,
	odrlIRI + "absoluteSize":             true,
	odrlIRI + "count":                    true,
	odrlIRI + "dateTime":                 true,
	odrlIRI + "delayPeriod":              true,
	odrlIRI + "deliveryChannel":          true,
	odrlIRI + "elapsedTime":              true,
	odrlIRI + "event":                    true,
	odrlIRI + "fileFormat":               true,
	odrlIRI + "industry":                 true,
	odrlIRI + "language":                 true,
	odrlIRI + "media":                    true,
	odrlIRI + "meteredTime":              true,
	odrlIRI + "payAmount":                true,
	odrlIRI + "percentage":               true,
	odrlIRI + "product":                  true,
	odrlIRI + "purpose":                  true,
	odrlIRI + "recipient":                true,
	odrlIRI + "relativePosition":         true,
	odrlIRI + "relativeSpatialPosition":  true,
	odrlIRI + "relativeTemporalPosition": true,
	odrlIRI + "relativeSize":             true,
	odrlIRI + "resolution":               true,
	odrlIRI + "spatial":                  true,
	odrlIRI + "spatialCoordinates":       true,
	odrlIRI + "systemDevice":             true,
	odrlIRI + "timeInterval":             true,
	odrlIRI + "unitOfCount":              true,
	odrlIRI + "version":                  true,
	odrlIRI + "virtualLocation":          true,
}

// isODRLContextOperand reports whether a left operand is an ODRL context
// operand — access context reported at use-time, not a document data field.
func isODRLContextOperand(iri string) bool {
	return odrlContextOperandIRIs[iri]
}

// resolveRightOperand converts a constraint's right operand to plain Go
// values. A boundary that references a requirement field — a value agreed at
// negotiation — resolves to that field's filled value; set operators always
// receive a list, as expansion erases the one-element-set/scalar distinction.
func resolveRightOperand(constraint map[string]any, operator string, fieldIndex map[string]odrlFieldInfo) any {
	values := expandedValues(constraint, odrlIRI+"rightOperand")
	resolve := func(v any) any {
		if obj, ok := v.(map[string]any); ok {
			if id, ok := obj["@id"].(string); ok {
				if info, found := fieldIndex[id]; found && info.hasValue {
					return info.value
				}
			}
		}
		return expandedLiteral(v)
	}
	if op := normalizePolicyOperator(operator); op == "in" || op == "notIn" {
		return mapAny(values, resolve)
	}
	switch len(values) {
	case 0:
		return nil
	case 1:
		return resolve(values[0])
	default:
		return mapAny(values, resolve)
	}
}

func mapAny(values []any, fn func(any) any) []any {
	out := make([]any, 0, len(values))
	for _, v := range values {
		out = append(out, fn(v))
	}
	return out
}

// odrlAndSequence is the ODRL ordered conjunction: its operands must all be
// satisfied, and satisfied in the order the list declares.
const odrlAndSequence = "andSequence"

// odrlLogicalOperators are the ODRL LogicalConstraint operators (IM §2.6):
// each takes a list of operand constraints, atomic or logical (recursive).
var odrlLogicalOperators = []string{"and", "or", "xone", odrlAndSequence}

// constraintLogical reports whether a constraint node is a LogicalConstraint,
// returning its operator local name and child constraint nodes. The children
// arrive as an @list (an ordered ODRL operand list) or a bare set.
func constraintLogical(node map[string]any) (string, []map[string]any, bool) {
	for _, op := range odrlLogicalOperators {
		children := []map[string]any{}
		for _, raw := range expandedValues(node, odrlIRI+op) {
			item, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if list, ok := item["@list"].([]any); ok {
				for _, li := range list {
					if child, ok := li.(map[string]any); ok {
						children = append(children, child)
					}
				}
				continue
			}
			children = append(children, item)
		}
		if len(children) > 0 {
			return op, children, true
		}
	}
	return "", nil, false
}

// evaluateConstraintNode evaluates an atomic or logical ODRL constraint tree
// against the document's inline field values. resolvable is false when the
// outcome depends on use-time context (spatial/dateTime/…) or a value not yet
// provided, so the audit defers rather than decides.
func evaluateConstraintNode(ctx context.Context, node map[string]any, fieldIndex map[string]odrlFieldInfo) (satisfied bool, resolvable bool, err error) {
	if op, children, isLogical := constraintLogical(node); isLogical {
		results := make([]bool, 0, len(children))
		for _, child := range children {
			childSatisfied, childResolvable, err := evaluateConstraintNode(ctx, child, fieldIndex)
			if err != nil {
				return false, false, err
			}
			if !childResolvable {
				// The whole tree defers if any operand needs use-time context.
				return false, false, nil
			}
			results = append(results, childSatisfied)
		}
		switch op {
		case "or":
			for _, r := range results {
				if r {
					return true, true, nil
				}
			}
			return false, true, nil
		case "xone":
			satisfiedCount := 0
			for _, r := range results {
				if r {
					satisfiedCount++
				}
			}
			return satisfiedCount == 1, true, nil
		case odrlAndSequence:
			// Ordered conjunction: a failing operand is decisive under any
			// order, but operands that all hold say nothing about the order
			// they hold in, which nothing here observes — so no verdict.
			for _, r := range results {
				if !r {
					return false, true, nil
				}
			}
			return false, false, nil
		default: // and
			for _, r := range results {
				if !r {
					return false, true, nil
				}
			}
			return true, true, nil
		}
	}

	leftOperand, ok := expandedFirst(node, odrlIRI+"leftOperand")
	if !ok {
		return false, false, nil
	}
	operandID, _ := leftOperand["@id"].(string)
	if isODRLContextOperand(operandID) {
		return false, false, nil // use-time context — deferred
	}
	operatorNode, ok := expandedFirst(node, odrlIRI+"operator")
	if !ok {
		return false, false, nil
	}
	operator := shaclLocalName(expandedID(operatorNode))
	info, ok := fieldIndex[operandID]
	if !ok || !info.hasValue {
		return false, false, nil // no value yet — deferred
	}
	if len(info.units) > 1 {
		return false, false, nil // bounded in two units at once — not comparable
	}
	satisfied, err = evaluateODRLConstraintOPA(ctx, operator, info.value, resolveRightOperand(node, operator, fieldIndex))
	return satisfied, true, err
}

// expandExternalODRLRules expands rule nodes supplied by an audit policy
// document (compact JSON with odrl-prefixed keys) with the hub context so
// they evaluate identically to document-embedded rules.
func expandExternalODRLRules(ctx context.Context, rules []map[string]any, source ShapeSource) ([]map[string]any, error) {
	if len(rules) == 0 {
		return nil, nil
	}
	wrapper := map[string]any{"dcs:policies": map[string]any{"odrl:obligation": anySlice(rules)}}
	root, err := expandForAudit(ctx, wrapper, source)
	if err != nil {
		return nil, fmt.Errorf("expand external ODRL policies: %w", err)
	}
	return expandedODRLPolicyRules(root), nil
}

func anySlice[T any](items []T) []any {
	out := make([]any, len(items))
	for i, item := range items {
		out[i] = item
	}
	return out
}

// expandedStatements flattens the expanded graph into statement rows for
// the validation-profile engine: every typed node becomes
// {"@id", "@type" (first class IRI), <property local name>: value}.
func expandedStatements(root map[string]any) []map[string]any {
	statements := []map[string]any{}
	var walk func(node map[string]any)
	walk = func(node map[string]any) {
		if types, ok := node["@type"].([]any); ok && len(types) > 0 {
			statement := map[string]any{}
			if id, ok := node["@id"].(string); ok {
				statement["@id"] = id
			}
			if iri, ok := types[0].(string); ok {
				statement["@type"] = iri
			}
			for property, raw := range node {
				if strings.HasPrefix(property, "@") {
					continue
				}
				values, ok := raw.([]any)
				if !ok || len(values) == 0 {
					continue
				}
				if len(values) == 1 {
					statement[shaclLocalName(property)] = expandedLiteral(values[0])
				} else {
					statement[shaclLocalName(property)] = expandedLiterals(values)
				}
			}
			statements = append(statements, statement)
		}
		for property, raw := range node {
			if strings.HasPrefix(property, "@") && property != "@graph" {
				continue
			}
			values, ok := raw.([]any)
			if !ok {
				continue
			}
			for _, value := range values {
				child, ok := value.(map[string]any)
				if !ok {
					continue
				}
				if list, ok := child["@list"].([]any); ok {
					for _, item := range list {
						if node, ok := item.(map[string]any); ok {
							walk(node)
						}
					}
					continue
				}
				walk(child)
			}
		}
	}
	walk(root)
	return statements
}

// statementsCoverProfile reports whether any statement carries one of the
// profile's appliesTo class IRIs; a profile without appliesTo covers
// everything.
func statementsCoverProfile(statements []map[string]any, appliesTo []string) bool {
	if len(appliesTo) == 0 {
		return true
	}
	for _, statement := range statements {
		typeIRI, _ := statement["@type"].(string)
		for _, class := range appliesTo {
			if typeIRI == class {
				return true
			}
		}
	}
	return false
}
