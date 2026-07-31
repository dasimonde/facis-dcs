package validation

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/open-policy-agent/opa/v1/rego"
)

// ODRL constraint satisfaction on Open Policy Agent (ADR-11). The operator
// semantics that were a hand-rolled Go switch (evaluateODRLConstraint) are
// expressed once as Rego and evaluated by the embedded engine: string
// equality is case-insensitive, numeric strings coerce to numbers, numeric
// comparisons carry the same 1e-7 tolerance, and set membership is
// upper/trim-normalised — matching evaluateODRLConstraint verdict-for-verdict
// (opaodrl_test.go is the parity gate).
const odrlRegoModule = `
package dcs.odrl

import rego.v1

tol := 0.0000001

to_num(x) := x if is_number(x)
to_num(x) := to_number(trim_space(x)) if is_string(x)

both_num(a, b) if {
	to_num(a)
	to_num(b)
}

both_string(a, b) if {
	is_string(a)
	is_string(b)
}

norm(x) := upper(trim_space(sprintf("%v", [x])))

is_ts_string(x) if {
	is_string(x)
	regex.match("^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}([.][0-9]+)?(Z|[+-][0-9]{2}:[0-9]{2})$", x)
}

to_ts(x) := time.parse_rfc3339_ns(x) if is_ts_string(x)

both_ts(a, b) if {
	to_ts(a)
	to_ts(b)
}

values_equal(a, b) if {
	both_string(a, b)
	lower(trim_space(a)) == lower(trim_space(b))
}

values_equal(a, b) if {
	not both_string(a, b)
	both_num(a, b)
	abs(to_num(a) - to_num(b)) <= tol
}

values_equal(a, b) if {
	not both_string(a, b)
	not both_num(a, b)
	sprintf("%v", [a]) == sprintf("%v", [b])
}

any_match if {
	some item in input.right
	norm(item) == norm(input.actual)
}

default satisfied := false

satisfied if {
	input.operator == "eq"
	values_equal(input.actual, input.right)
}

# isA — the left operand is an instance of the class named by the right
# operand. With no class graph to reason over at the value level, this reduces
# to (normalised) equality of the class identifier.
satisfied if {
	input.operator == "isA"
	values_equal(input.actual, input.right)
}

satisfied if {
	input.operator == "neq"
	not values_equal(input.actual, input.right)
}

satisfied if {
	input.operator == "gt"
	both_num(input.actual, input.right)
	to_num(input.actual) > to_num(input.right) + tol
}

satisfied if {
	input.operator == "gteq"
	both_num(input.actual, input.right)
	to_num(input.actual) + tol >= to_num(input.right)
}

satisfied if {
	input.operator == "lt"
	both_num(input.actual, input.right)
	to_num(input.actual) < to_num(input.right) - tol
}

satisfied if {
	input.operator == "lteq"
	both_num(input.actual, input.right)
	to_num(input.actual) <= to_num(input.right) + tol
}

# Ordering over RFC3339 timestamps (SRS Appendix C dateTime constraints):
# when the operands are not numbers but parse as instants, compare instants.
satisfied if {
	input.operator == "gt"
	not both_num(input.actual, input.right)
	both_ts(input.actual, input.right)
	to_ts(input.actual) > to_ts(input.right)
}

satisfied if {
	input.operator == "gteq"
	not both_num(input.actual, input.right)
	both_ts(input.actual, input.right)
	to_ts(input.actual) >= to_ts(input.right)
}

satisfied if {
	input.operator == "lt"
	not both_num(input.actual, input.right)
	both_ts(input.actual, input.right)
	to_ts(input.actual) < to_ts(input.right)
}

satisfied if {
	input.operator == "lteq"
	not both_num(input.actual, input.right)
	both_ts(input.actual, input.right)
	to_ts(input.actual) <= to_ts(input.right)
}

satisfied if {
	input.operator == "isAnyOf"
	any_match
}

satisfied if {
	input.operator == "isNoneOf"
	not any_match
}

satisfied if {
	input.operator == "hasPart"
	is_string(input.actual)
	contains(input.actual, sprintf("%v", [input.right]))
}

# isPartOf — the value-level converse of hasPart, with no partonomy graph to
# consult: the actual value is part of a right operand that enumerates a set
# (membership) or spells out a whole (substring).
satisfied if {
	input.operator == "isPartOf"
	is_array(input.right)
	some item in input.right
	norm(item) == norm(input.actual)
}

satisfied if {
	input.operator == "isPartOf"
	is_string(input.right)
	contains(input.right, sprintf("%v", [input.actual]))
}

# isAllOf — the actual value comprises every member of the right operand set
# (a scalar right is read as a one-member set).
right_items := input.right if is_array(input.right)

right_items := [input.right] if not is_array(input.right)

satisfied if {
	input.operator == "isAllOf"
	count(right_items) > 0
	every item in right_items {
		norm(item) == norm(input.actual)
	}
}
`

var (
	odrlRegoOnce  sync.Once
	odrlRegoQuery rego.PreparedEvalQuery
	odrlRegoErr   error
)

func preparedODRLQuery() (rego.PreparedEvalQuery, error) {
	odrlRegoOnce.Do(func() {
		odrlRegoQuery, odrlRegoErr = rego.New(
			rego.Query("data.dcs.odrl.satisfied"),
			rego.Module("odrl.rego", odrlRegoModule),
		).PrepareForEval(context.Background())
	})
	return odrlRegoQuery, odrlRegoErr
}

var (
	tzLessDateTime = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?$`)
	dateOnly       = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// normalizeTemporal renders a timezone-less ISO-8601 dateTime (or a bare date)
// as RFC3339 UTC so the Rego engine's time.parse_rfc3339_ns can order it;
// non-temporal values pass through untouched. The ODRL vocabulary the UI emits
// carries such tz-less dateTimes (SRS Appendix C: "2025-05-10T23:59:59").
func normalizeTemporal(value any) any {
	s, ok := value.(string)
	if !ok {
		return value
	}
	trimmed := strings.TrimSpace(s)
	switch {
	case tzLessDateTime.MatchString(trimmed):
		return trimmed + "Z"
	case dateOnly.MatchString(trimmed):
		return trimmed + "T00:00:00Z"
	default:
		return value
	}
}

// xsdDurationPattern matches the xsd:duration lexical space (XSD 1.1 §3.3.6,
// plus the ISO-8601 week designator). "at least one component" and "a T
// section is non-empty" are checked in parseXSDDuration — RE2 has no
// lookahead to state them here.
var xsdDurationPattern = regexp.MustCompile(`^(-?)P(\d+Y)?(\d+M)?(\d+W)?(\d+D)?(T(\d+H)?(\d+M)?(\d+(?:\.\d+)?S)?)?$`)

// xsdDurationMagnitude is a duration as XSD defines it: a month count and a
// second count held apart, because a month is not a fixed number of seconds.
type xsdDurationMagnitude struct {
	months  float64
	seconds float64
}

func parseXSDDuration(raw any) (xsdDurationMagnitude, bool) {
	text, ok := raw.(string)
	if !ok {
		return xsdDurationMagnitude{}, false
	}
	trimmed := strings.TrimSpace(text)
	groups := xsdDurationPattern.FindStringSubmatch(trimmed)
	if groups == nil || strings.HasSuffix(trimmed, "T") {
		return xsdDurationMagnitude{}, false
	}
	// group 6 is the whole T section, a delimiter rather than a component.
	scales := map[int]xsdDurationMagnitude{
		2: {months: 12},
		3: {months: 1},
		4: {seconds: 7 * 86400},
		5: {seconds: 86400},
		7: {seconds: 3600},
		8: {seconds: 60},
		9: {seconds: 1},
	}
	magnitude := xsdDurationMagnitude{}
	components := 0
	for group, scale := range scales {
		if groups[group] == "" {
			continue
		}
		amount, err := strconv.ParseFloat(strings.TrimRight(groups[group], "YMWDHS"), 64)
		if err != nil {
			return xsdDurationMagnitude{}, false
		}
		components++
		magnitude.months += amount * scale.months
		magnitude.seconds += amount * scale.seconds
	}
	if components == 0 {
		return xsdDurationMagnitude{}, false
	}
	if groups[1] == "-" {
		magnitude.months, magnitude.seconds = -magnitude.months, -magnitude.seconds
	}
	return magnitude, true
}

// orderableODRLOperands replaces a pair of xsd:duration literals with their
// numeric magnitude so the engine's numeric rules order them, rather than
// falling through to a verdict of "unsatisfied" for every duration boundary
// the SLA profile states ("elapsed time <= P14D"). Only a commensurable pair
// is substituted: a month is 28 to 31 days, so "P1M" against "P30D" has no
// defined order and is left alone (unsatisfied, never silently decided).
// Anything that is not a duration on both sides passes through untouched.
func orderableODRLOperands(actual, rightOperand any) (any, any) {
	left, leftOK := parseXSDDuration(actual)
	right, rightOK := parseXSDDuration(rightOperand)
	if !leftOK || !rightOK {
		return actual, rightOperand
	}
	switch {
	case left.months == 0 && right.months == 0:
		return left.seconds, right.seconds
	case left.seconds == 0 && right.seconds == 0:
		return left.months, right.months
	default:
		return actual, rightOperand
	}
}

// evaluateODRLConstraintOPA reports whether an actual value satisfies an ODRL
// constraint operator against its right operand, evaluated on OPA. The
// operator is reduced to its local name and the values compacted exactly as
// evaluateODRLConstraint does, so the two agree on every verdict.
func evaluateODRLConstraintOPA(ctx context.Context, operator string, actualValue, rightOperand any) (bool, error) {
	query, err := preparedODRLQuery()
	if err != nil {
		return false, err
	}
	actual, right := orderableODRLOperands(
		normalizeTemporal(compactJSONLDValue(actualValue)),
		normalizeTemporal(compactJSONLDValue(rightOperand)),
	)
	input := map[string]any{
		"operator": compactTerm(operator),
		"actual":   actual,
		"right":    right,
	}
	rs, err := query.Eval(ctx, rego.EvalInput(input))
	if err != nil {
		return false, err
	}
	if len(rs) == 0 || len(rs[0].Expressions) == 0 {
		return false, nil
	}
	satisfied, _ := rs[0].Expressions[0].Value.(bool)
	return satisfied, nil
}
