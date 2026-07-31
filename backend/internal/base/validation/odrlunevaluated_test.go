package validation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// Two ODRL constructs the authoring UI can produce carry a requirement this
// audit cannot check: odrl:andSequence requires an order no contract document
// records, and odrl:unit denominates a boundary whose value carries no unit of
// its own. Both used to pass through as an ordinary green verdict — the
// requirement vanished and the audit reported success. These tests hold the
// audit to saying what it did not check.

func atomicFieldConstraint(fieldID, operator string, rightOperand any) map[string]any {
	return map[string]any{
		"@type":             "odrl:Constraint",
		"odrl:leftOperand":  map[string]any{"@id": fieldID},
		"odrl:operator":     map[string]any{"@id": operator},
		"odrl:rightOperand": rightOperand,
	}
}

func orderedConjunctionDuty(ruleID, fieldID string) map[string]any {
	return map[string]any{
		"@id":         ruleID,
		"@type":       "odrl:Duty",
		"dcs:prose":   map[string]any{"@id": "urn:uuid:block-clause-1"},
		"odrl:action": map[string]any{"@id": "dcs:provideCompliantValue"},
		"odrl:constraint": []any{map[string]any{
			"@type": "odrl:LogicalConstraint",
			"odrl:andSequence": map[string]any{"@list": []any{
				atomicFieldConstraint(fieldID, "odrl:gteq", float64(100)),
				atomicFieldConstraint(fieldID, "odrl:lteq", float64(1000)),
			}},
		}},
	}
}

func TestAuditContractDoesNotCallAnOrderedConjunctionSatisfied(t *testing.T) {
	fieldID := "urn:dcs:field:amount"
	ruleID := "FACIS-ORDERED-CONJUNCTION"

	// Both operands hold, but the order they must hold in is unobservable —
	// the audit must not turn that into a satisfied verdict.
	contract := odrlContract(fieldID, "payment", "amount", []any{orderedConjunctionDuty(ruleID, fieldID)}, float64(500))
	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, ruleID, "warning"),
		"an andSequence whose ordering is not evaluated must say so")
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			require.NotContains(t, finding.Message, "constraint satisfied",
				"the audit must not report an unevaluated ordering as satisfied: %s", finding.Message)
		}
	}
}

func TestAuditContractStillFlagsAnOrderedConjunctionWithAFailingOperand(t *testing.T) {
	fieldID := "urn:dcs:field:amount"
	ruleID := "FACIS-ORDERED-CONJUNCTION"

	// 2000 breaks the lteq operand; no ordering of the operands rescues it.
	contract := odrlContract(fieldID, "payment", "amount", []any{orderedConjunctionDuty(ruleID, fieldID)}, float64(2000))
	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, ruleID, "error"),
		"a definitely failing operand is decisive under any ordering")
}

func TestAuditContractSurfacesAConstraintUnitNothingChecks(t *testing.T) {
	fieldID := "urn:dcs:field:pay-amount"
	ruleID := "FACIS-PAY-AMOUNT-EUR"

	duty := map[string]any{
		"@id":         ruleID,
		"@type":       "odrl:Duty",
		"dcs:prose":   map[string]any{"@id": "urn:uuid:block-clause-1"},
		"odrl:action": map[string]any{"@id": "dcs:provideCompliantValue"},
		"odrl:constraint": []any{map[string]any{
			"@type":             "odrl:Constraint",
			"odrl:leftOperand":  map[string]any{"@id": fieldID},
			"odrl:operator":     map[string]any{"@id": "odrl:lteq"},
			"odrl:rightOperand": float64(500),
			"odrl:unit":         map[string]any{"@id": "https://w3id.org/facis/dcs/taxonomy/v1#currency-EUR"},
		}},
	}

	contract := odrlContract(fieldID, "payment", "amount", []any{duty}, float64(400))
	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	// Reported, but not as a warning: every dcs:ContractField carries a bare
	// value with no unit, so a warning here would make the workflow gate REVIEW
	// every contract that denominates a boundary at all — which is every correct
	// use of odrl:unit. Only an unagreed negotiated unit is the author's to fix.
	require.True(t, hasFindingSeverity(findings, ruleID, SeverityDeferred),
		"a boundary denominated in a unit nothing compares against must be reported as unchecked")
	require.False(t, hasFindingSeverity(findings, ruleID, "warning"),
		"a declared unit is a property of the field model, not a contract defect to review")
}

func TestAuditContractRefusesToCompareBoundariesInDifferentUnits(t *testing.T) {
	fieldID := "urn:dcs:field:pay-amount"

	inUnit := func(ruleID, operator string, boundary float64, unit string) map[string]any {
		constraint := atomicFieldConstraint(fieldID, operator, boundary)
		constraint["odrl:unit"] = map[string]any{"@id": unit}
		return map[string]any{
			"@id":             ruleID,
			"@type":           "odrl:Duty",
			"dcs:prose":       map[string]any{"@id": "urn:uuid:block-clause-1"},
			"odrl:action":     map[string]any{"@id": "dcs:provideCompliantValue"},
			"odrl:constraint": []any{constraint},
		}
	}

	// One field, two currencies: 450 satisfies both bounds numerically, which
	// is exactly the false green the audit must refuse to give.
	contract := odrlContract(fieldID, "payment", "amount", []any{
		inUnit("FACIS-PAY-EUR", "odrl:lteq", 500, "https://w3id.org/facis/dcs/taxonomy/v1#currency-EUR"),
		inUnit("FACIS-PAY-GBP", "odrl:gteq", 400, "https://w3id.org/facis/dcs/taxonomy/v1#currency-GBP"),
	}, float64(450))

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "FACIS-PAY-EUR", "error"))
	require.True(t, hasFindingSeverity(findings, "FACIS-PAY-GBP", "error"))
	for _, finding := range findings {
		require.NotContains(t, finding.Message, "satisfied",
			"incommensurable boundaries must not produce a satisfied verdict: %s", finding.Message)
	}
}
