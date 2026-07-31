package validation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testRoleEntityType  = "https://w3id.org/facis/dcs/ontology/v1#CompanyParty"
	testRoleProvider    = "provider"
	testRoleCustomer    = "customer"
	testRoleReseller    = "reseller"
	testPaymentTermType = "https://w3id.org/facis/dcs/ontology/v1#PaymentTerm"
)

// statementScopedTestProfile is the shipped facis.sla.basic profile reduced
// to its statement-scoped (where-clause) rules — the subset
// auditContractStatementValidationRules evaluates.
func statementScopedTestProfile(t *testing.T) ValidationProfile {
	t.Helper()
	profile, err := LoadValidationProfileYAML([]byte(mustReadRepoFile("backend/internal/semantichub/assets/facis.sla.basic.v1.yaml")))
	require.NoError(t, err)
	rules := make([]ValidationRule, 0, len(profile.Rules))
	for _, rule := range profile.Rules {
		if len(rule.Where) > 0 {
			rules = append(rules, rule)
		}
	}
	profile.Rules = rules
	return profile
}

func validContractStatementsForValidation() []map[string]any {
	return []map[string]any{
		{
			"@id":       "party-provider",
			"@type":     testRoleEntityType,
			"role":      testRoleProvider,
			"legalName": "Provider GmbH",
		},
		{
			"@id":       "party-customer",
			"@type":     testRoleEntityType,
			"role":      testRoleCustomer,
			"legalName": "Customer GmbH",
		},
		{
			"@id":      "payment-main",
			"@type":    testPaymentTermType,
			"amount":   1000.0,
			"currency": "EUR",
			"dueDate":  "2026-06-19",
		},
	}
}

func validationIssueIDs(issues []ValidationIssue) []string {
	ids := make([]string, len(issues))
	for i, issue := range issues {
		ids[i] = issue.RuleID
	}
	return ids
}

func TestValidateContractStatementsAcceptsValidContract(t *testing.T) {
	issues := ValidateContractStatements(validContractStatementsForValidation(), statementScopedTestProfile(t))
	require.Empty(t, issues)
}

func TestValidateContractStatementsRejectsUnknownPartyRole(t *testing.T) {
	statements := append(validContractStatementsForValidation(), map[string]any{
		"@id":   "party-overlord",
		"@type": testRoleEntityType,
		"role":  "overlord",
	})

	issues := ValidateContractStatements(statements, statementScopedTestProfile(t))

	require.Equal(t, []string{"company-party-role-vocabulary"}, validationIssueIDs(issues))
	require.Equal(t, "party-overlord", issues[0].StatementID)
}

func TestValidateContractStatementsAcceptsRolelessParty(t *testing.T) {
	// Parties attached before role binding carry no role yet — a vocabulary
	// rule constrains values that are present, it does not require them.
	statements := append(validContractStatementsForValidation(), map[string]any{
		"@id":       "party-unbound",
		"@type":     testRoleEntityType,
		"legalName": "Unbound GmbH",
	})

	issues := ValidateContractStatements(statements, statementScopedTestProfile(t))

	require.Empty(t, issues)
}

func TestValidateContractStatementsReportsUnknownRuleType(t *testing.T) {
	issues := ValidateContractStatements(validContractStatementsForValidation(), ValidationProfile{
		ID:      "test",
		Version: "1",
		Rules: []ValidationRule{
			{ID: "unknown", Type: "rego", Severity: "error"},
		},
	})

	require.Len(t, issues, 1)
	require.Equal(t, "unknown", issues[0].RuleID)
	require.Contains(t, issues[0].Message, "unknown validation rule type")
}

func TestValidateContractStatementsSupportsReusableRuleTypes(t *testing.T) {
	statements := append(validContractStatementsForValidation(), map[string]any{
		"@id":       "party-provider-duplicate-name",
		"@type":     testRoleEntityType,
		"role":      testRoleReseller,
		"legalName": "Provider GmbH",
	})
	profile := ValidationProfile{
		ID:      "test",
		Version: "1",
		Rules: []ValidationRule{
			{
				ID:    "payment-exists",
				Type:  ValidationRuleExists,
				Where: map[string]any{"@type": testPaymentTermType},
			},
			{
				ID:       "provider-role",
				Type:     ValidationRuleFieldValue,
				Target:   "role",
				Where:    map[string]any{"@id": "party-provider"},
				Operator: "eq",
				Value:    testRoleProvider,
			},
			{
				ID:       "positive-payment",
				Type:     ValidationRuleComparison,
				Target:   "amount",
				Where:    map[string]any{"@type": testPaymentTermType},
				Operator: "gt",
				Value:    0,
			},
			{
				ID:     "unique-party-names",
				Type:   ValidationRuleUnique,
				Target: "legalName",
				Where:  map[string]any{"@type": testRoleEntityType},
			},
		},
	}

	issues := ValidateContractStatements(statements, profile)

	require.Equal(t, []string{"unique-party-names"}, validationIssueIDs(issues))
	require.Equal(t, "party-provider-duplicate-name", issues[0].StatementID)
}

func TestLoadValidationProfileRejectsInvalidDefinitions(t *testing.T) {
	_, err := LoadValidationProfileYAML([]byte(`
id: broken
version: "1"
rules:
  - id: missing-target
    type: comparison
    operator: gt
    value: 0
`))

	require.ErrorContains(t, err, "requires a target")
}

func TestLoadValidationProfileYAML(t *testing.T) {
	defaultProfile, err := LoadValidationProfileYAML([]byte(mustReadRepoFile("backend/internal/semantichub/assets/facis.sla.basic.v1.yaml")))
	require.NoError(t, err)
	require.Equal(t, "facis.sla.basic", defaultProfile.ID)
	require.Contains(t, validationIssueIDs(ValidateContractStatements(
		[]map[string]any{{"@id": "party-x", "@type": testRoleEntityType, "role": "overlord"}},
		defaultProfile,
	)), "company-party-role-vocabulary")

	yamlProfile, err := LoadValidationProfileYAML([]byte(`
id: facis.marketplace.contract.v1
version: "1"
rules:
  - id: payment-exists
    type: exists
    severity: error
    where:
      "@type": https://w3id.org/facis/dcs/ontology/v1#PaymentTerm
`))
	require.NoError(t, err)
	require.Equal(t, "facis.marketplace.contract.v1", yamlProfile.ID)
}

func TestStatementQueryUtilities(t *testing.T) {
	statements := validContractStatementsForValidation()

	require.True(t, MatchesWhereClause(statements[0], map[string]any{"role": testRoleProvider}))
	require.Len(t, FindStatements(statements, map[string]any{"@type": testRoleEntityType}), 2)
	require.Equal(t, 1, CountStatements(statements, map[string]any{"@type": testPaymentTermType}))
	require.Len(t, FilterStatements(statements, func(statement map[string]any) bool {
		_, ok := statement["currency"]
		return ok
	}), 1)
}
