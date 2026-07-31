package validation

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// An odrl:unit may name a declared contract field instead of a fixed concept —
// a payment amount haggled in a currency the parties also haggle. These tests
// hold the audit to resolving such a unit to its agreed value, to treating two
// constraints denominated in the same agreed unit as one unit, and to still
// refusing the comparison when two boundaries are denominated differently.

const (
	payAmountFieldID   = "urn:dcs:field:pay-amount"
	payCurrencyFieldID = "urn:dcs:field:pay-currency"
	altCurrencyFieldID = "urn:dcs:field:alt-currency"
	fixedEUR           = "https://w3id.org/facis/dcs/taxonomy/v1#currency-EUR"
)

// currencyField declares a negotiable currency field, filled only when
// agreed != "".
func currencyField(id, label, agreed string) map[string]any {
	field := map[string]any{
		"@id":          id,
		"@type":        "dcs:ContractField",
		"dcs:label":    label,
		"dcs:datatype": "xsd:string",
		"dcs:required": true,
	}
	if agreed != "" {
		field["dcs:value"] = agreed
	}
	return field
}

// payAmountContract builds a contract whose amount field is bounded by the
// given duties, alongside the currency fields those duties denominate in.
func payAmountContract(amount any, currencies []any, duties []any) map[string]any {
	fields := []any{
		map[string]any{
			"@id":          payAmountFieldID,
			"@type":        "dcs:ContractField",
			"dcs:label":    "Payment Amount",
			"dcs:datatype": "xsd:decimal",
			"dcs:required": true,
			"dcs:value":    amount,
		},
	}
	fields = append(fields, currencies...)
	return map[string]any{
		"dcs:contractFields": fields,
		"dcs:policies":       wrapODRLSet(duties),
	}
}

// payAmountDuty bounds the amount field, denominated in the given odrl:unit
// node (a fixed concept reference or a contract-field reference).
func payAmountDuty(ruleID, operator string, boundary float64, unit any) map[string]any {
	constraint := atomicFieldConstraint(payAmountFieldID, operator, boundary)
	constraint["odrl:unit"] = unit
	return map[string]any{
		"@id":             ruleID,
		"@type":           "odrl:Duty",
		"dcs:prose":       map[string]any{"@id": "urn:uuid:block-clause-1"},
		"odrl:action":     map[string]any{"@id": "dcs:provideCompliantValue"},
		"odrl:constraint": []any{constraint},
	}
}

func findingMessages(findings []PolicyFinding, ruleID string) []string {
	messages := []string{}
	for _, finding := range findings {
		if finding.RuleID == ruleID {
			messages = append(messages, finding.Message)
		}
	}
	return messages
}

func someMessageContains(messages []string, substrings ...string) bool {
	for _, message := range messages {
		matched := true
		for _, substring := range substrings {
			if !strings.Contains(message, substring) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func TestAuditContractResolvesANegotiatedUnitToItsAgreedValue(t *testing.T) {
	ruleID := "FACIS-PAY-IN-AGREED-CURRENCY"

	contract := payAmountContract(float64(400),
		[]any{currencyField(payCurrencyFieldID, "Payment Currency", "EUR")},
		[]any{payAmountDuty(ruleID, "odrl:lteq", 500, map[string]any{"@id": payCurrencyFieldID})})

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	messages := findingMessages(findings, ruleID)
	require.True(t, someMessageContains(messages, "Payment Currency", "EUR"),
		"a negotiated unit must be reported as the value it was agreed to: %v", messages)
	require.False(t, hasFindingSeverity(findings, ruleID, "error"),
		"one boundary in one agreed unit is comparable: %v", messages)
	require.True(t, someMessageContains(messages, "satisfied"),
		"the comparison must still reach a verdict: %v", messages)
}

func TestAuditContractTreatsTwoFieldsAgreedToTheSameUnitAsOneUnit(t *testing.T) {
	// Two boundaries denominated in two different currency fields, both agreed
	// to EUR. Resolving each unit to its agreed value makes them one unit, so
	// the boundaries are comparable.
	contract := payAmountContract(float64(450), []any{
		currencyField(payCurrencyFieldID, "Payment Currency", "EUR"),
		currencyField(altCurrencyFieldID, "Settlement Currency", "EUR"),
	}, []any{
		payAmountDuty("FACIS-PAY-CEILING", "odrl:lteq", 500, map[string]any{"@id": payCurrencyFieldID}),
		payAmountDuty("FACIS-PAY-FLOOR", "odrl:gteq", 400, map[string]any{"@id": altCurrencyFieldID}),
	})

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	for _, ruleID := range []string{"FACIS-PAY-CEILING", "FACIS-PAY-FLOOR"} {
		messages := findingMessages(findings, ruleID)
		require.False(t, hasFindingSeverity(findings, ruleID, "error"),
			"boundaries agreed to the same unit must stay comparable: %v", messages)
		require.True(t, someMessageContains(messages, "satisfied"),
			"the comparison must reach a verdict: %v", messages)
	}
}

func TestAuditContractRefusesToCompareAFixedUnitAgainstANegotiatedOne(t *testing.T) {
	// A boundary fixed to the EUR concept and one denominated in a currency the
	// parties agreed as the string "EUR" are not known to be the same unit: the
	// field holds a notation, not the concept, and nothing here maps one to the
	// other. 450 satisfies both bounds numerically — the false green to refuse.
	contract := payAmountContract(float64(450),
		[]any{currencyField(payCurrencyFieldID, "Payment Currency", "EUR")},
		[]any{
			payAmountDuty("FACIS-PAY-FIXED-EUR", "odrl:lteq", 500, map[string]any{"@id": fixedEUR}),
			payAmountDuty("FACIS-PAY-NEGOTIATED", "odrl:gteq", 400, map[string]any{"@id": payCurrencyFieldID}),
		})

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	require.True(t, hasFindingSeverity(findings, "FACIS-PAY-FIXED-EUR", "error"))
	require.True(t, hasFindingSeverity(findings, "FACIS-PAY-NEGOTIATED", "error"))
	for _, finding := range findings {
		require.NotContains(t, finding.Message, "satisfied",
			"incommensurable boundaries must not produce a satisfied verdict: %s", finding.Message)
	}
}

func TestAuditContractDefersAnUnagreedNegotiatedUnitWithoutFailingClosed(t *testing.T) {
	ruleID := "FACIS-PAY-UNAGREED-CURRENCY"

	contract := payAmountContract(float64(400),
		[]any{currencyField(payCurrencyFieldID, "Payment Currency", "")},
		[]any{payAmountDuty(ruleID, "odrl:lteq", 500, map[string]any{"@id": payCurrencyFieldID})})

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	messages := findingMessages(findings, ruleID)
	require.True(t, someMessageContains(messages, "Payment Currency", "no agreed value"),
		"an unagreed unit must be reported as unagreed: %v", messages)
	require.True(t, hasFindingSeverity(findings, ruleID, "warning"),
		"an unagreed unit is a gap the audit states, not a verdict: %v", messages)
	require.False(t, hasFindingSeverity(findings, ruleID, "error"),
		"an unagreed unit must not fail the boundary closed: %v", messages)
	require.True(t, someMessageContains(messages, "satisfied"),
		"the boundary comparison itself is unaffected by the unit: %v", messages)
}

func TestAuditContractTreatsOneUnagreedUnitFieldAsOneUnit(t *testing.T) {
	// Two boundaries denominated in the same unagreed field are denominated in
	// whatever that field becomes — one unit, whichever it turns out to be.
	contract := payAmountContract(float64(450),
		[]any{currencyField(payCurrencyFieldID, "Payment Currency", "")},
		[]any{
			payAmountDuty("FACIS-PAY-CEILING", "odrl:lteq", 500, map[string]any{"@id": payCurrencyFieldID}),
			payAmountDuty("FACIS-PAY-FLOOR", "odrl:gteq", 400, map[string]any{"@id": payCurrencyFieldID}),
		})

	findings, err := AuditContractContent(context.Background(), contract, emptyPolicy(), ContractContentAuditMetadata{})
	require.NoError(t, err)

	for _, ruleID := range []string{"FACIS-PAY-CEILING", "FACIS-PAY-FLOOR"} {
		require.False(t, hasFindingSeverity(findings, ruleID, "error"),
			"one unagreed unit field is one unit: %v", findingMessages(findings, ruleID))
	}
}
