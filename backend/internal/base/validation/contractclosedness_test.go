package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// contractWithBoundary builds a minimal contract whose permission is bounded by
// a spatial constraint whose boundary is the negotiated "region" field. The
// field is filled only when value != "".
func contractWithBoundary(value string) map[string]any {
	field := map[string]any{
		"@id":          "urn:dcs:field:region",
		"@type":        "dcs:ContractField",
		"dcs:label":    "region",
		"dcs:datatype": "xsd:string",
		"dcs:required": true,
	}
	if value != "" {
		field["dcs:value"] = value
	}
	return map[string]any{
		"@type":              "dcs:Contract",
		"dcs:contractFields": []any{field},
		"dcs:policies": map[string]any{
			"@type": "odrl:Agreement",
			"odrl:permission": []any{
				map[string]any{
					"@id":         "R1",
					"@type":       "odrl:Permission",
					"odrl:action": map[string]any{"@id": "odrl:use"},
					"odrl:constraint": []any{
						map[string]any{
							"@type":             "odrl:Constraint",
							"odrl:leftOperand":  map[string]any{"@id": "odrl:spatial"},
							"odrl:operator":     map[string]any{"@id": "odrl:eq"},
							"odrl:rightOperand": map[string]any{"@id": "urn:dcs:field:region"},
						},
					},
				},
			},
		},
	}
}

func TestValidateContractClosedFlagsUnfilledNegotiatedBoundary(t *testing.T) {
	err := ValidateContractClosed(contractWithBoundary(""))
	require.ErrorIs(t, err, ErrContractNotClosed)
	require.ErrorContains(t, err, "negotiated boundary")
}

func TestValidateContractClosedAcceptsFilledBoundary(t *testing.T) {
	require.NoError(t, ValidateContractClosed(contractWithBoundary("DE")))
}

// contractWithNegotiatedUnit bounds a payment amount denominated in a currency
// the parties also negotiate (odrl:unit referencing a field). The currency is
// agreed only when currency != "".
func contractWithNegotiatedUnit(currency string) map[string]any {
	currencyField := map[string]any{
		"@id":          "urn:dcs:field:currency",
		"@type":        "dcs:ContractField",
		"dcs:label":    "Payment Currency",
		"dcs:datatype": "xsd:string",
		"dcs:required": true,
	}
	if currency != "" {
		currencyField["dcs:value"] = currency
	}
	return map[string]any{
		"@type": "dcs:Contract",
		"dcs:contractFields": []any{
			map[string]any{
				"@id": "urn:dcs:field:amount", "@type": "dcs:ContractField",
				"dcs:label": "Payment Amount", "dcs:datatype": "xsd:decimal",
				"dcs:required": true, "dcs:value": "5000",
			},
			currencyField,
		},
		"dcs:policies": map[string]any{
			"@type": "odrl:Agreement",
			"odrl:obligation": []any{
				map[string]any{
					"@id": "D1", "@type": "odrl:Duty",
					"odrl:action": map[string]any{"@id": "dcs:provideCompliantValue"},
					"odrl:constraint": []any{
						map[string]any{
							"@type":             "odrl:Constraint",
							"odrl:leftOperand":  map[string]any{"@id": "urn:dcs:field:amount"},
							"odrl:operator":     map[string]any{"@id": "odrl:lteq"},
							"odrl:rightOperand": map[string]any{"@value": "10000", "@type": "xsd:decimal"},
							"odrl:unit":         map[string]any{"@id": "urn:dcs:field:currency"},
						},
					},
				},
			},
		},
	}
}

func TestValidateContractClosedFlagsUnagreedNegotiatedUnit(t *testing.T) {
	// A boundary whose currency was never agreed says nothing about what the
	// amount is measured in — no more closed than an unagreed boundary itself.
	err := ValidateContractClosed(contractWithNegotiatedUnit(""))
	require.ErrorIs(t, err, ErrContractNotClosed)
	require.ErrorContains(t, err, "negotiated unit")
}

func TestValidateContractClosedAcceptsAgreedNegotiatedUnit(t *testing.T) {
	require.NoError(t, ValidateContractClosed(contractWithNegotiatedUnit("EUR")))
}

func TestValidateContractClosedFlagsUnfilledRequiredField(t *testing.T) {
	// The permission constrains the field directly (not as a boundary); a
	// required field a policy enforces must carry a value.
	doc := map[string]any{
		"@type": "dcs:Contract",
		"dcs:contractFields": []any{
			map[string]any{
				"@id": "urn:dcs:field:amount", "@type": "dcs:ContractField",
				"dcs:label": "amount", "dcs:datatype": "xsd:decimal", "dcs:required": true,
			},
		},
		"dcs:policies": map[string]any{
			"@type": "odrl:Agreement",
			"odrl:obligation": []any{
				map[string]any{
					"@id": "D1", "@type": "odrl:Duty",
					"odrl:action": map[string]any{"@id": "dcs:provideCompliantValue"},
					"odrl:constraint": []any{
						map[string]any{
							"@type":             "odrl:Constraint",
							"odrl:leftOperand":  map[string]any{"@id": "urn:dcs:field:amount"},
							"odrl:operator":     map[string]any{"@id": "odrl:gteq"},
							"odrl:rightOperand": map[string]any{"@value": "100", "@type": "xsd:decimal"},
						},
					},
				},
			},
		},
	}
	err := ValidateContractClosed(doc)
	require.ErrorIs(t, err, ErrContractNotClosed)
	require.ErrorContains(t, err, "required data field")
}

func TestValidateContractClosedFlagsUnfilledProseContractField(t *testing.T) {
	doc := contractWithBoundary("DE") // boundary filled, but a prose field is not
	doc["dcs:documentStructure"] = map[string]any{
		"@type": "dcs:DocumentStructure",
		"dcs:blocks": map[string]any{"@list": []any{
			map[string]any{
				"@id": "urn:dcs:block:1", "@type": "dcs:Clause",
				"dcs:content": map[string]any{"@list": []any{
					"The party in ",
					map[string]any{"@id": "urn:dcs:field:unfilled"},
				}},
			},
		}},
	}
	err := ValidateContractClosed(doc)
	require.ErrorIs(t, err, ErrContractNotClosed)
	require.ErrorContains(t, err, "prose field")
}
