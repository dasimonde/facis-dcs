package validation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// PACM-07 By Clauses integration scenarios: the same PaymentClauseShape rule
// must yield Compliant when values pass and Issues when they violate. Plain
// dcs:Clause with no targeted shape stays Not assessed. Shared field refs
// stay Undetermined / unassigned — never invent attribution.

func installClauseReportShapes(t *testing.T) func() {
	t.Helper()
	return swapShapeSource(t, fixtureShapeSource{
		shapesTTL:   mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-shapes.ttl"),
		catalogTTL:  mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-clause-catalog.ttl"),
		profileYAML: "id: t\nversion: t\nrules: []\n",
		contextJSON: mustReadRepoFile("backend/internal/semantichub/assets/facis-dcs-context.jsonld"),
	})
}

func paymentClauseContract(clauseID string, amount int, currency string) map[string]any {
	return map[string]any{
		"@context": map[string]any{
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#",
			"xsd": "http://www.w3.org/2001/XMLSchema#",
		},
		"@id":   "urn:facis:dcs:contract:clause-report-it",
		"@type": "dcs:Contract",
		"dcs:documentStructure": map[string]any{
			"dcs:blocks": map[string]any{
				"@list": []any{
					map[string]any{
						"@id":          clauseID,
						"@type":        "dcs:PaymentClause",
						"dcs:title":    "Clause 01",
						"dcs:amount":   map[string]any{"@value": amount, "@type": "xsd:integer"},
						"dcs:currency": currency,
					},
				},
			},
		},
	}
}

func projectForTest(t *testing.T, doc map[string]any) []ClauseSectionProjection {
	t.Helper()
	ctx := context.Background()
	clauses, err := ExtractClauseComponents(doc)
	require.NoError(t, err)
	require.NotEmpty(t, clauses)

	findings, err := AuditContractContent(ctx, doc, mapPolicy(true, false), ContractContentAuditMetadata{
		ContractDID: "clause-report-it",
	})
	require.NoError(t, err)

	findings, err = MergeClauseDomainFieldFindings(ctx, doc, clauses, findings)
	require.NoError(t, err)

	index, err := LoadAuditShapeTargetIndex(ctx, doc)
	require.NoError(t, err)

	sections, _ := ProjectClauseSections(clauses, findings, DocumentID(doc), index, true, true)
	require.Len(t, sections, len(clauses))
	return sections
}

const (
	taxonomyPaymentAmount   = "https://w3id.org/facis/dcs/taxonomy/v1#field-contract-payment-amount"
	taxonomyPaymentCurrency = "https://w3id.org/facis/dcs/taxonomy/v1#field-contract-payment-currency"
)

// genericPaymentClauseContract mirrors the UI-produced structure: plain
// dcs:Clause whose dcs:content refs ContractFields carrying dcs:shape IRIs.
func genericPaymentClauseContract(clauseID, amountFieldID, currencyFieldID string, amount any, currency string) map[string]any {
	return map[string]any{
		"@context": map[string]any{
			"dcs": "https://w3id.org/facis/dcs/ontology/v1#",
			"xsd": "http://www.w3.org/2001/XMLSchema#",
		},
		"@id":   "urn:facis:dcs:contract:generic-payment-clause",
		"@type": "dcs:Contract",
		"dcs:contractFields": []any{
			map[string]any{
				"@id":          amountFieldID,
				"@type":        "dcs:ContractField",
				"dcs:label":    "Payment Amount",
				"dcs:datatype": "xsd:decimal",
				"dcs:required": true,
				"dcs:shape":    map[string]any{"@id": taxonomyPaymentAmount},
				"dcs:value":    map[string]any{"@value": amount, "@type": "xsd:decimal"},
			},
			map[string]any{
				"@id":          currencyFieldID,
				"@type":        "dcs:ContractField",
				"dcs:label":    "Payment Currency",
				"dcs:datatype": "xsd:string",
				"dcs:required": true,
				"dcs:shape":    map[string]any{"@id": taxonomyPaymentCurrency},
				"dcs:value":    map[string]any{"@value": currency},
			},
		},
		"dcs:documentStructure": map[string]any{
			"dcs:blocks": map[string]any{
				"@list": []any{
					map[string]any{
						"@id":       clauseID,
						"@type":     "dcs:Clause",
						"dcs:title": "Clause 01",
						"dcs:content": map[string]any{
							"@list": []any{
								map[string]any{"@id": amountFieldID},
								map[string]any{"@id": currencyFieldID},
							},
						},
					},
				},
			},
		},
	}
}

func TestClauseReport_PaymentClausePassesIsCompliant(t *testing.T) {
	restore := installClauseReportShapes(t)
	defer restore()

	const clauseID = "urn:facis:dcs:clause:pay-pass"
	sections := projectForTest(t, paymentClauseContract(clauseID, 100, "EUR"))
	sec := sections[0]
	require.Equal(t, clauseID, sec.ClauseID)
	require.Equal(t, "Clause 01", sec.Title)
	require.Equal(t, ClauseStatusCompliant, sec.Status, "valid PaymentClause must be Compliant under PaymentClauseShape")
	require.Equal(t, ApplicableChecksEstablished, sec.ApplicableChecksStatus)
	require.Contains(t, sec.ApplicableChecks, "PaymentClauseShape")
	require.Empty(t, sec.Findings)
}

func TestClauseReport_PaymentClauseViolationIsIssues(t *testing.T) {
	restore := installClauseReportShapes(t)
	defer restore()

	const clauseID = "urn:facis:dcs:clause:pay-fail"
	// Same shape/rule as the pass case — negative amount violates sh:minInclusive 0.
	sections := projectForTest(t, paymentClauseContract(clauseID, -5, "EUR"))
	sec := sections[0]
	require.Equal(t, clauseID, sec.ClauseID)
	require.Equal(t, ClauseStatusIssuesIdentified, sec.Status, "invalid PaymentClause must be Issues under the same PaymentClauseShape")
	require.NotEmpty(t, sec.Findings)
	require.Equal(t, ApplicableChecksEstablished, sec.ApplicableChecksStatus)
	require.Contains(t, sec.ApplicableChecks, "PaymentClauseShape")

	// Attribute to this clause (focus = clause @id), not left unassigned.
	foundFocus := false
	for _, f := range sec.Findings {
		if f.FocusNode == clauseID || f.Severity == SeverityError || f.Severity == "error" {
			foundFocus = true
			break
		}
	}
	require.True(t, foundFocus, "violation findings should be attributed to the PaymentClause node")
}

func TestClauseReport_GenericClausePaymentFieldsPassIsCompliant(t *testing.T) {
	restore := installClauseReportShapes(t)
	defer restore()

	const (
		clauseID   = "urn:facis:dcs:clause:generic-pay-pass"
		amountID   = "urn:facis:dcs:field:pay-amount"
		currencyID = "urn:facis:dcs:field:pay-currency"
	)
	sections := projectForTest(t, genericPaymentClauseContract(clauseID, amountID, currencyID, 1, "EUR"))
	sec := sections[0]
	require.Equal(t, clauseID, sec.ClauseID)
	require.Equal(t, "Clause 01", sec.Title)
	require.Equal(t, ClauseStatusCompliant, sec.Status,
		"generic dcs:Clause with valid payment amount/currency DomainField checks must be Compliant")
	require.Equal(t, ApplicableChecksEstablished, sec.ApplicableChecksStatus)
	require.Contains(t, sec.ApplicableChecks, "Payment Amount ("+taxonomyPaymentAmount+")")
	require.Contains(t, sec.ApplicableChecks, "Payment Currency ("+taxonomyPaymentCurrency+")")
	require.Empty(t, sec.Findings)
}

func TestClauseReport_GenericClausePaymentCurrencyViolationIsIssues(t *testing.T) {
	restore := installClauseReportShapes(t)
	defer restore()

	const (
		clauseID   = "urn:facis:dcs:clause:generic-pay-fail"
		amountID   = "urn:facis:dcs:field:pay-amount-bad"
		currencyID = "urn:facis:dcs:field:pay-currency-bad"
	)
	// Same shapes/fields as the pass case — invalid currency fails ISO-4217 ValueConstraint.
	sections := projectForTest(t, genericPaymentClauseContract(clauseID, amountID, currencyID, 1, "XXX"))
	sec := sections[0]
	require.Equal(t, clauseID, sec.ClauseID)
	require.Equal(t, ClauseStatusIssuesIdentified, sec.Status,
		"invalid payment currency on generic dcs:Clause must be Issues under the same DomainField rules")
	require.Equal(t, ApplicableChecksEstablished, sec.ApplicableChecksStatus)
	require.NotEmpty(t, sec.Findings)
	foundCurrency := false
	for _, f := range sec.Findings {
		if f.FocusNode == currencyID || f.RuleID == taxonomyPaymentCurrency {
			foundCurrency = true
			require.Equal(t, SeverityError, f.Severity)
			require.Equal(t, "Payment Currency", f.Title)
			require.Equal(t, taxonomyPaymentCurrency, f.OntologyTerm)
		}
	}
	require.True(t, foundCurrency, "currency DomainField violation must attribute to the clause")
}

func TestClauseReport_PlainClauseNoApplicableIsNotAssessed(t *testing.T) {
	restore := installClauseReportShapes(t)
	defer restore()

	const clauseID = "urn:facis:dcs:clause:plain-01"
	doc := map[string]any{
		"@context": map[string]any{"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id":      "urn:facis:dcs:contract:plain-clause",
		"@type":    "dcs:Contract",
		"dcs:documentStructure": map[string]any{
			"dcs:blocks": map[string]any{
				"@list": []any{
					map[string]any{
						"@id":       clauseID,
						"@type":     "dcs:Clause",
						"dcs:title": "Clause 01",
						"dcs:content": map[string]any{
							"@list": []any{
								"Payment amount is ",
								map[string]any{"@id": "urn:facis:dcs:field:amount"},
							},
						},
					},
				},
			},
		},
	}
	sections := projectForTest(t, doc)
	sec := sections[0]
	require.Equal(t, ClauseStatusNotAssessed, sec.Status)
	require.Equal(t, ApplicableChecksNone, sec.ApplicableChecksStatus)
	require.Equal(t, "None", sec.ApplicableChecks)
	require.Empty(t, sec.Findings)
}

func TestClauseReport_SharedFieldRefIsAmbiguous(t *testing.T) {
	restore := installClauseReportShapes(t)
	defer restore()

	sharedField := "urn:facis:dcs:field:shared-currency"
	doc := map[string]any{
		"@context": map[string]any{"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id":      "urn:facis:dcs:contract:shared-ref",
		"@type":    "dcs:Contract",
		"dcs:documentStructure": map[string]any{
			"dcs:blocks": map[string]any{
				"@list": []any{
					map[string]any{
						"@id":       "urn:facis:dcs:clause:a",
						"@type":     "dcs:Clause",
						"dcs:title": "Clause A",
						"dcs:content": map[string]any{
							"@list": []any{map[string]any{"@id": sharedField}},
						},
					},
					map[string]any{
						"@id":       "urn:facis:dcs:clause:b",
						"@type":     "dcs:Clause",
						"dcs:title": "Clause B",
						"dcs:content": map[string]any{
							"@list": []any{map[string]any{"@id": sharedField}},
						},
					},
				},
			},
		},
	}

	// Inject a synthetic field-linked finding the way ODRL/profile would:
	// focus on the shared field so attribution cannot pick a single clause.
	clauses, err := ExtractClauseComponents(doc)
	require.NoError(t, err)
	require.Len(t, clauses, 2)

	findings := []PolicyFinding{{
		RuleID:    "FACIS-SHARED-CURRENCY",
		Severity:  SeverityError,
		Message:   "currency constraint failed",
		FieldIri:  sharedField,
		FocusNode: sharedField,
		Source:    SourceContractODRL,
	}}
	index, err := LoadAuditShapeTargetIndex(context.Background(), doc)
	require.NoError(t, err)

	sections, unassigned := ProjectClauseSections(clauses, findings, DocumentID(doc), index, true, true)
	require.Len(t, sections, 2)
	require.Len(t, unassigned, 1, "shared-field finding must stay unassigned")
	require.Equal(t, "FACIS-SHARED-CURRENCY", unassigned[0].RuleID)

	for _, sec := range sections {
		require.Empty(t, sec.Findings, "neither clause may claim the shared finding")
		// No typed shapes; shared field rule is ambiguous → Undetermined.
		require.Equal(t, ClauseStatusUndetermined, sec.Status, sec.ClauseID)
	}
}

// TestClauseReport_ProjectsClausesNotAppendices: the clause report (PACM-07)
// projects the clauses of a contract. An appendix is not a clause-report
// component, and a projected section carries no credential metadata.
func TestClauseReport_ProjectsClausesNotAppendices(t *testing.T) {
	restore := installClauseReportShapes(t)
	defer restore()

	doc := map[string]any{
		"@context": map[string]any{"dcs": "https://w3id.org/facis/dcs/ontology/v1#"},
		"@id":      "urn:facis:dcs:contract:appendix",
		"@type":    "dcs:Contract",
		"dcs:documentStructure": map[string]any{
			"dcs:blocks": map[string]any{
				"@list": []any{
					map[string]any{
						"@id":       "urn:facis:dcs:clause:c1",
						"@type":     "dcs:Clause",
						"dcs:title": "Clause 01",
					},
					map[string]any{
						"@id":       "urn:facis:dcs:appendix:a1",
						"@type":     "dcs:Appendix",
						"dcs:title": "Appendix A",
					},
				},
			},
		},
	}
	clauses, err := ExtractClauseComponents(doc)
	require.NoError(t, err)
	require.Len(t, clauses, 1, "an appendix is not a clause-report component")
	require.Equal(t, "Clause 01", clauses[0].Title)

	sections := projectForTest(t, doc)
	require.Len(t, sections, 1)
	// ClauseSectionProjection has no credential-metadata fields.
	require.Equal(t, ClauseStatusNotAssessed, sections[0].Status)
}
