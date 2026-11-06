package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildShapeTargetIndex(t *testing.T) {
	ttl := `
@prefix sh: <http://www.w3.org/ns/shacl#> .
@prefix dcs: <https://w3id.org/facis/dcs/ontology/v1#> .
dcs:PaymentClauseShape a sh:NodeShape ; sh:targetClass dcs:PaymentClause .
dcs:ContractShape a sh:NodeShape ; sh:targetClass dcs:Contract .
`
	index, err := BuildShapeTargetIndex(ttl)
	require.NoError(t, err)
	require.Contains(t, index["PaymentClause"], "PaymentClauseShape")
	require.Contains(t, index["dcs:PaymentClause"], "PaymentClauseShape")
	require.Contains(t, index["Contract"], "ContractShape")
}

func TestDeriveClauseCoverage_plainClauseNoApplicable(t *testing.T) {
	index := ShapeTargetIndex{
		"PaymentClause":     {"PaymentClauseShape"},
		"dcs:PaymentClause": {"PaymentClauseShape"},
	}
	clause := ClauseComponent{ID: "urn:c#1", Title: "Clause 01", Types: []string{"dcs:Clause"}}
	cov := DeriveClauseCoverage(ClauseCoverageInput{
		Clause: clause, AuditRan: true, IndexOK: true, Index: index,
	})
	require.Equal(t, ApplicableChecksNone, cov.ApplicableChecks)
	require.Equal(t, ClauseStatusNotAssessed, cov.Status)
	require.Equal(t, "None", FormatApplicableChecksLabel(cov))
}

func TestDeriveClauseCoverage_typedClauseCompliant(t *testing.T) {
	index := ShapeTargetIndex{
		"PaymentClause":     {"PaymentClauseShape"},
		"dcs:PaymentClause": {"PaymentClauseShape"},
	}
	clause := ClauseComponent{ID: "urn:c#1", Types: []string{"dcs:PaymentClause"}}
	cov := DeriveClauseCoverage(ClauseCoverageInput{
		Clause: clause, AuditRan: true, IndexOK: true, Index: index,
	})
	require.Equal(t, ApplicableChecksEstablished, cov.ApplicableChecks)
	require.Equal(t, []string{"PaymentClauseShape"}, cov.ApplicableCheckIDs)
	require.Equal(t, ClauseStatusCompliant, cov.Status)
}

func TestDeriveClauseCoverage_fieldRefODRLApplicable(t *testing.T) {
	clauses := []ClauseComponent{
		{ID: "urn:c#a", Types: []string{"dcs:Clause"}, Refs: []string{"urn:c#field-a"}},
		{ID: "urn:c#b", Types: []string{"dcs:Clause"}, Refs: []string{"urn:c#field-b"}},
	}
	findings := []PolicyFinding{{
		RuleID:    "odrl-min-amount",
		Severity:  SeveritySatisfied,
		Message:   "ODRL policy satisfied",
		FieldIri:  "urn:c#field-a",
		FocusNode: "urn:c#field-a",
		Source:    SourceContractODRL,
	}}
	cov := DeriveClauseCoverage(ClauseCoverageInput{
		Clause:      clauses[0],
		Attributed:  findings,
		AllFindings: findings,
		AllClauses:  clauses,
		AuditRan:    true,
		IndexOK:     true,
		Index:       ShapeTargetIndex{},
	})
	require.Equal(t, ApplicableChecksEstablished, cov.ApplicableChecks)
	require.Contains(t, cov.ApplicableCheckIDs, "odrl-min-amount")
	require.Equal(t, ClauseStatusCompliant, cov.Status)
}

func TestDeriveClauseCoverage_sharedRefAmbiguous(t *testing.T) {
	clauses := []ClauseComponent{
		{ID: "urn:c#a", Types: []string{"dcs:Clause"}, Refs: []string{"urn:c#shared"}},
		{ID: "urn:c#b", Types: []string{"dcs:Clause"}, Refs: []string{"urn:c#shared"}},
	}
	findings := []PolicyFinding{{
		RuleID:   "profile-shared",
		Severity: SeveritySatisfied,
		Message:  "ok",
		FieldIri: "urn:c#shared",
		Source:   SourceValidationProfile,
	}}
	cov := DeriveClauseCoverage(ClauseCoverageInput{
		Clause:      clauses[0],
		AllFindings: findings,
		AllClauses:  clauses,
		AuditRan:    true,
		IndexOK:     true,
		Index:       ShapeTargetIndex{},
	})
	require.Equal(t, ClauseStatusUndetermined, cov.Status)
	require.Contains(t, cov.VerdictNote, "multiple clauses")
}

func TestDeriveClauseCoverage_issuesAndUndetermined(t *testing.T) {
	clause := ClauseComponent{ID: "urn:c#1", Types: []string{"dcs:Clause"}}
	cov := DeriveClauseCoverage(ClauseCoverageInput{
		Clause: clause,
		Attributed: []PolicyFinding{{
			Severity: SeverityError, Message: "bad",
		}},
		AuditRan: true, IndexOK: true, Index: ShapeTargetIndex{},
	})
	require.Equal(t, ClauseStatusIssuesIdentified, cov.Status)

	cov = DeriveClauseCoverage(ClauseCoverageInput{
		Clause: clause, AuditRan: true, IndexOK: false,
	})
	require.Equal(t, ApplicableChecksNotEstablished, cov.ApplicableChecks)
	require.Equal(t, ClauseStatusUndetermined, cov.Status)
	require.Equal(t, "Not established", FormatApplicableChecksLabel(cov))

	cov = DeriveClauseCoverage(ClauseCoverageInput{
		Clause: clause, AuditRan: false,
	})
	require.Equal(t, ClauseStatusNotAssessed, cov.Status)
}

func TestDeriveClauseCoverage_deferredIsUndetermined(t *testing.T) {
	clause := ClauseComponent{
		ID: "urn:c#1", Types: []string{"dcs:Clause"}, Refs: []string{"urn:c#f"},
	}
	findings := []PolicyFinding{{
		RuleID:   "odrl-duty",
		Severity: SeverityDeferred,
		Message:  "deferred to runtime",
		FieldIri: "urn:c#f",
	}}
	cov := DeriveClauseCoverage(ClauseCoverageInput{
		Clause:      clause,
		Attributed:  findings,
		AllFindings: findings,
		AllClauses:  []ClauseComponent{clause},
		AuditRan:    true,
		IndexOK:     true,
		Index:       ShapeTargetIndex{},
	})
	require.Equal(t, ClauseStatusUndetermined, cov.Status)
}

func TestExtractClauseComponentsIncludesTypes(t *testing.T) {
	doc := map[string]any{
		"@id": "urn:c",
		"dcs:documentStructure": map[string]any{
			"dcs:blocks": map[string]any{
				"@list": []any{
					map[string]any{
						"@id":       "urn:c#1",
						"@type":     "dcs:PaymentClause",
						"dcs:title": "Pay",
					},
				},
			},
		},
	}
	clauses, err := ExtractClauseComponents(doc)
	require.NoError(t, err)
	require.Len(t, clauses, 1)
	require.Equal(t, []string{"dcs:PaymentClause"}, clauses[0].Types)
}
