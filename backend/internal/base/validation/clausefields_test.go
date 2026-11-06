package validation

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIndexContractFieldsCapturesShapeAndValue(t *testing.T) {
	amountID := "urn:facis:dcs:field:amount"
	doc := map[string]any{
		"@id": "urn:facis:dcs:contract:fields",
		"dcs:contractFields": []any{
			map[string]any{
				"@id":       amountID,
				"dcs:label": "Payment Amount",
				"dcs:shape": map[string]any{"@id": taxonomyPaymentAmount},
				"dcs:value": map[string]any{"@value": 1, "@type": "xsd:decimal"},
			},
		},
	}
	fields, err := IndexContractFields(doc)
	require.NoError(t, err)
	info, ok := fields[amountID]
	require.True(t, ok)
	require.Equal(t, taxonomyPaymentAmount, info.ShapeIRI)
	require.Equal(t, "Payment Amount", info.Label)
	require.True(t, info.HasValue)
}

func TestAssessClauseDomainFieldsCurrencyPassAndFail(t *testing.T) {
	restore := installClauseReportShapes(t)
	defer restore()

	ctx := context.Background()
	clauseID := "urn:facis:dcs:clause:c1"
	amountID := "urn:facis:dcs:field:a"
	currencyID := "urn:facis:dcs:field:c"

	passDoc := genericPaymentClauseContract(clauseID, amountID, currencyID, 1, "EUR")
	clauses, err := ExtractClauseComponents(passDoc)
	require.NoError(t, err)
	fields, err := IndexContractFields(passDoc)
	require.NoError(t, err)

	passFindings, err := AssessClauseDomainFields(ctx, clauses, fields)
	require.NoError(t, err)
	require.Len(t, passFindings, 2)
	for _, f := range passFindings {
		require.Equal(t, SeveritySatisfied, f.Severity, f.RuleID)
		require.Equal(t, "domain-ontology", f.Source)
	}

	failDoc := genericPaymentClauseContract(clauseID, amountID, currencyID, 1, "XXX")
	failFields, err := IndexContractFields(failDoc)
	require.NoError(t, err)
	failFindings, err := AssessClauseDomainFields(ctx, clauses, failFields)
	require.NoError(t, err)
	var currencyFinding *PolicyFinding
	for i := range failFindings {
		if failFindings[i].RuleID == taxonomyPaymentCurrency {
			currencyFinding = &failFindings[i]
		}
	}
	require.NotNil(t, currencyFinding)
	require.Equal(t, SeverityError, currencyFinding.Severity)
	require.Equal(t, currencyID, currencyFinding.FocusNode)
	require.Equal(t, "Payment Currency", currencyFinding.Title)
	require.Equal(t, taxonomyPaymentCurrency, currencyFinding.OntologyTerm)
	require.Contains(t, findingRuleLabel(*currencyFinding), "Payment Currency (")
	require.Contains(t, findingRuleLabel(*currencyFinding), taxonomyPaymentCurrency)
}

func TestAssessClauseDomainFieldsSkipsSharedRefs(t *testing.T) {
	restore := installClauseReportShapes(t)
	defer restore()

	shared := "urn:facis:dcs:field:shared"
	doc := map[string]any{
		"@id": "urn:facis:dcs:contract:shared",
		"dcs:contractFields": []any{
			map[string]any{
				"@id":       shared,
				"dcs:shape": map[string]any{"@id": taxonomyPaymentCurrency},
				"dcs:value": "EUR",
			},
		},
	}
	clauses := []ClauseComponent{
		{ID: "urn:c#a", Refs: []string{shared}},
		{ID: "urn:c#b", Refs: []string{shared}},
	}
	fields, err := IndexContractFields(doc)
	require.NoError(t, err)
	findings, err := AssessClauseDomainFields(context.Background(), clauses, fields)
	require.NoError(t, err)
	require.Empty(t, findings, "shared field refs must not emit attributed domain findings")
}
