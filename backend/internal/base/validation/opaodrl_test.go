package validation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// evaluateODRLConstraint is the retired hand-rolled operator switch, kept
// here as the parity oracle only — it is not compiled into the production
// binary (ADR-11). evaluateODRLConstraintOPA must match it verdict-for-verdict.
const floatTolerance = 0.0000001

func evaluateODRLConstraint(operator string, actualValue any, rightOperand any) bool {
	op := compactTerm(operator)
	actualValue = compactJSONLDValue(actualValue)
	rightOperand = compactJSONLDValue(rightOperand)
	actualValue, rightOperand = orderableODRLOperands(actualValue, rightOperand)
	switch op {
	case "eq", "isA":
		return odrlValuesEqual(actualValue, rightOperand)
	case "neq":
		return !odrlValuesEqual(actualValue, rightOperand)
	case "gt":
		if f1, ok1 := toFloat(actualValue); ok1 {
			if f2, ok2 := toFloat(rightOperand); ok2 {
				return f1 > f2+floatTolerance
			}
		}
		t1, ok1 := toTime(actualValue)
		t2, ok2 := toTime(rightOperand)
		return ok1 && ok2 && t1.After(t2)
	case "gteq":
		if f1, ok1 := toFloat(actualValue); ok1 {
			if f2, ok2 := toFloat(rightOperand); ok2 {
				return f1+floatTolerance >= f2
			}
		}
		t1, ok1 := toTime(actualValue)
		t2, ok2 := toTime(rightOperand)
		return ok1 && ok2 && !t1.Before(t2)
	case "lt":
		if f1, ok1 := toFloat(actualValue); ok1 {
			if f2, ok2 := toFloat(rightOperand); ok2 {
				return f1 < f2-floatTolerance
			}
		}
		t1, ok1 := toTime(actualValue)
		t2, ok2 := toTime(rightOperand)
		return ok1 && ok2 && t1.Before(t2)
	case "lteq":
		if f1, ok1 := toFloat(actualValue); ok1 {
			if f2, ok2 := toFloat(rightOperand); ok2 {
				return f1 <= f2+floatTolerance
			}
		}
		t1, ok1 := toTime(actualValue)
		t2, ok2 := toTime(rightOperand)
		return ok1 && ok2 && !t1.After(t2)
	case "isAnyOf":
		items, ok := asArray(rightOperand)
		if !ok {
			return false
		}
		normalized := strings.ToUpper(strings.TrimSpace(fmt.Sprint(actualValue)))
		for _, item := range items {
			if strings.ToUpper(strings.TrimSpace(fmt.Sprint(compactJSONLDValue(item)))) == normalized {
				return true
			}
		}
		return false
	case "isNoneOf":
		items, ok := asArray(rightOperand)
		if !ok {
			return true
		}
		normalized := strings.ToUpper(strings.TrimSpace(fmt.Sprint(actualValue)))
		for _, item := range items {
			if strings.ToUpper(strings.TrimSpace(fmt.Sprint(compactJSONLDValue(item)))) == normalized {
				return false
			}
		}
		return true
	case "hasPart":
		str, ok := actualValue.(string)
		if !ok {
			return false
		}
		return strings.Contains(str, fmt.Sprint(compactJSONLDValue(rightOperand)))
	case "isPartOf":
		if items, ok := asArray(rightOperand); ok {
			normalized := strings.ToUpper(strings.TrimSpace(fmt.Sprint(actualValue)))
			for _, item := range items {
				if strings.ToUpper(strings.TrimSpace(fmt.Sprint(compactJSONLDValue(item)))) == normalized {
					return true
				}
			}
			return false
		}
		str, ok := rightOperand.(string)
		if !ok {
			return false
		}
		return strings.Contains(str, fmt.Sprint(actualValue))
	case "isAllOf":
		items, ok := asArray(rightOperand)
		if !ok {
			items = []any{rightOperand}
		}
		if len(items) == 0 {
			return false
		}
		normalized := strings.ToUpper(strings.TrimSpace(fmt.Sprint(actualValue)))
		for _, item := range items {
			if strings.ToUpper(strings.TrimSpace(fmt.Sprint(compactJSONLDValue(item)))) != normalized {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func odrlValuesEqual(a, b any) bool {
	a = compactJSONLDValue(a)
	b = compactJSONLDValue(b)
	sa, saOk := a.(string)
	sb, sbOk := b.(string)
	if saOk && sbOk {
		return strings.EqualFold(sa, sb)
	}
	fa, faOk := toFloat(a)
	fb, fbOk := toFloat(b)
	if faOk && fbOk {
		return math.Abs(fa-fb) <= floatTolerance
	}
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func toFloat(value any) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, !math.IsNaN(typed)
	case float32:
		return float64(typed), !math.IsNaN(float64(typed))
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case json.Number:
		float, err := typed.Float64()
		return float, err == nil
	case string:
		// Whole-string parse, matching Rego's to_number: "2025-05-10T…" is not
		// a number (Sscanf would greedily read the leading year).
		parsed, err := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

// toTime parses an ISO-8601 instant — RFC3339, or timezone-less dateTime/date
// read as UTC — mirroring normalizeTemporal + the Rego time.parse_rfc3339_ns
// branch, so the oracle and OPA agree on temporal ordering.
func toTime(value any) (time.Time, bool) {
	s, ok := value.(string)
	if !ok {
		return time.Time{}, false
	}
	trimmed := strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

// TestOPADurationConstraintsOrderByMagnitude pins the verdicts themselves
// (the parity gate only proves the two evaluators agree): an xsd:duration
// boundary orders by magnitude. Byte order puts "PT6H" after "PT24H", so a
// lexical comparison reports a six-hour window as breaching a 24-hour bound —
// an answer, and the wrong one.
func TestOPADurationConstraintsOrderByMagnitude(t *testing.T) {
	cases := []struct {
		operator string
		actual   string
		right    string
		want     bool
	}{
		{"lteq", "PT6H", "PT24H", true},
		{"gt", "PT6H", "PT24H", false},
		{"lteq", "P5D", "P14D", true},
		{"lteq", "P30D", "P14D", false},
		{"lteq", "P14D", "P14D", true},
		{"lt", "P1W", "P14D", true},
		{"eq", "PT24H", "P1D", true},
		{"gteq", "P1Y", "P1M", true},
		// A month is 28 to 31 days: "P1M" against "P30D" has no defined order,
		// and an undecidable boundary is not a satisfied one.
		{"lteq", "P1M", "P30D", false},
		{"gteq", "P1M", "P30D", false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%s/%s", tc.operator, tc.actual, tc.right), func(t *testing.T) {
			got, err := evaluateODRLConstraintOPA(context.Background(), tc.operator,
				map[string]any{"@value": tc.actual, "@type": "xsd:duration"},
				map[string]any{"@value": tc.right, "@type": "xsd:duration"})
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestOPAConstraintParityWithHandRolled is the ADR-11 parity gate: the OPA
// ODRL evaluation must return the identical verdict to the hand-rolled
// evaluateODRLConstraint for every operator/value combination, including the
// case-insensitivity, numeric-string coercion, and tolerance edge cases the
// contract audit corpus exercises. Until this passes, the hand-rolled
// evaluator stays as the oracle; once it does, the switch can be retired.
func TestOPAConstraintParityWithHandRolled(t *testing.T) {
	cases := []struct {
		operator string
		actual   any
		right    any
	}{
		// eq: string exact, case-insensitive, numeric-string coercion
		{"eq", "DEU", "DEU"},
		{"eq", "DEU", "deu"},
		{"eq", "DEU", "USA"},
		{"eq", "91448", "91448"},
		{"eq", float64(9500), float64(9500)},
		{"eq", "9500", float64(9500)},
		{"eq", float64(9500), float64(10000)},
		{"eq", float64(99.5), float64(99.5000000001)}, // within tolerance
		{"eq", "DEU", float64(5)},                     // non-numeric string vs number
		// neq
		{"neq", "DEU", "USA"},
		{"neq", "DEU", "deu"},
		{"neq", float64(9500), float64(9500)},
		// gt / gteq / lt / lteq (numeric, boundary + tolerance)
		{"gt", float64(150000), float64(100000)},
		{"gt", float64(100000), float64(150000)},
		{"gt", float64(99.5), float64(99.5)},
		{"gteq", float64(99.5), float64(99.5)},
		{"gteq", float64(99.4), float64(99.5)},
		{"gteq", float64(100), float64(99.5)},
		{"lt", float64(99.4), float64(99.5)},
		{"lt", float64(99.5), float64(99.5)},
		{"lteq", float64(99.5), float64(99.5)},
		{"lteq", float64(99.6), float64(99.5)},
		{"lteq", "9500", float64(10000)}, // numeric-string coercion
		{"gt", "notanumber", float64(1)}, // non-numeric -> false both sides
		// dateTime ordering (SRS Appendix C): tz-less instants and dates,
		// before/at/after the boundary, and mixed number/dateTime operands.
		{"lteq", "2025-05-09T10:00:00", "2025-05-10T23:59:59"},
		{"lteq", "2025-05-10T23:59:59", "2025-05-10T23:59:59"},
		{"lteq", "2025-05-11T00:00:00", "2025-05-10T23:59:59"},
		{"gteq", "2025-05-11T00:00:00", "2025-05-10T23:59:59"},
		{"lt", "2025-05-09", "2025-05-10"},
		{"gt", "2025-05-11T00:00:00Z", "2025-05-10T23:59:59Z"},
		{"lteq", "2025-05-10T10:00:00", float64(500)}, // dateTime vs number -> false
		{"eq", "2025-05-10", "2025-05-10"},            // string equality unaffected
		// xsd:duration ordering (the SLA profile's "elapsed time <= P14D"):
		// magnitude, never lexical — "PT6H" sorts after "PT24H" by bytes.
		{"lteq", "PT6H", "PT24H"},
		{"gt", "PT6H", "PT24H"},
		{"lteq", "P5D", "P14D"},
		{"lteq", "P30D", "P14D"},
		{"lteq", "P14D", "P14D"},
		{"lt", "P1W", "P14D"},
		{"eq", "PT24H", "P1D"},
		{"lteq", "P1M", "P1Y"},
		{"lteq", "P1M", "P30D"},     // month vs days: no defined order -> false
		{"lteq", "P5D", float64(1)}, // duration vs number -> false
		// isAnyOf / isNoneOf (upper/trim-normalised membership)
		{"isAnyOf", "DEU", []any{"DEU", "AUT", "CHE"}},
		{"isAnyOf", "deu", []any{"DEU", "AUT", "CHE"}},
		{"isAnyOf", "ZZZ", []any{"DEU", "AUT", "CHE"}},
		{"isAnyOf", "DE", []any{"DE", "AT", "CH"}},
		{"isNoneOf", "RUS", []any{"RUS"}},
		{"isNoneOf", "DEU", []any{"RUS"}},
		{"isNoneOf", "DEU", []any{}},
		// hasPart (substring)
		{"hasPart", "hello world", "world"},
		{"hasPart", "hello", "xyz"},
		// isPartOf: membership over a set, substring over a spelled-out whole
		{"isPartOf", "DEU", []any{"DEU", "AUT", "CHE"}},
		{"isPartOf", "deu", []any{"DEU", "AUT"}},
		{"isPartOf", "ZZZ", []any{"DEU"}},
		{"isPartOf", "Bay", "Bayern, Germany"},
		{"isPartOf", "xyz", "Bayern"},
		{"isPartOf", "DEU", float64(5)}, // non-string, non-array right -> false
		// isA: value-level class-identifier equality (case-insensitive)
		{"isA", "odrl:Asset", "odrl:Asset"},
		{"isA", "odrl:asset", "odrl:Asset"},
		{"isA", "odrl:Asset", "odrl:Party"},
		// isAllOf: the actual must match every member of the right set
		{"isAllOf", "DEU", []any{"DEU"}},
		{"isAllOf", "DEU", []any{"DEU", "DEU"}},
		{"isAllOf", "DEU", []any{"DEU", "AUT"}}, // not all equal -> false
		{"isAllOf", "DEU", "DEU"},               // scalar coerces to singleton set
		{"isAllOf", "DEU", []any{}},             // empty set -> false
		// unknown operator -> false
		{"unknownOp", "DEU", "DEU"},
	}

	for _, tc := range cases {
		t.Run(fmt.Sprintf("%s/%v/%v", tc.operator, tc.actual, tc.right), func(t *testing.T) {
			want := evaluateODRLConstraint(tc.operator, tc.actual, tc.right)
			got, err := evaluateODRLConstraintOPA(context.Background(), tc.operator, tc.actual, tc.right)
			require.NoError(t, err)
			require.Equal(t, want, got, "OPA verdict must match hand-rolled evaluator")
		})
	}
}
