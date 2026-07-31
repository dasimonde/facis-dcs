package validation

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	ValidationRuleCount          = "count"
	ValidationRuleExists         = "exists"
	ValidationRuleRequiredFields = "required_fields"
	ValidationRuleFieldValue     = "field_value"
	ValidationRuleUnique         = "unique"
	ValidationRuleComparison     = "comparison"
	ValidationRuleReferences     = "references"
	ValidationRuleValueIn        = "value_in"
	ValidationRuleSignatureLevel = "signature_level_at_least"
)

type ValidationProfile struct {
	ID          string `json:"id" yaml:"id"`
	Version     string `json:"version" yaml:"version"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
	// AppliesTo scopes the profile's statement rules: they evaluate only
	// when the document's expanded graph contains a node typed with one of
	// these class IRIs.
	AppliesTo []string         `json:"appliesTo,omitempty" yaml:"appliesTo,omitempty"`
	Rules     []ValidationRule `json:"rules" yaml:"rules"`
}

type ValidationRule struct {
	ID              string               `json:"id" yaml:"id"`
	Type            string               `json:"type" yaml:"type"`
	Severity        string               `json:"severity,omitempty" yaml:"severity,omitempty"`
	Message         string               `json:"message,omitempty" yaml:"message,omitempty"`
	Target          string               `json:"target,omitempty" yaml:"target,omitempty"`
	Where           map[string]any       `json:"where,omitempty" yaml:"where,omitempty"`
	Operator        string               `json:"operator,omitempty" yaml:"operator,omitempty"`
	Value           any                  `json:"value,omitempty" yaml:"value,omitempty"`
	Values          []string             `json:"values,omitempty" yaml:"values,omitempty"`
	RequiredFields  []string             `json:"requiredFields,omitempty" yaml:"requiredFields,omitempty"`
	ReferenceFields []StatementReference `json:"referenceFields,omitempty" yaml:"referenceFields,omitempty"`
}

type StatementReference struct {
	Field string         `json:"field" yaml:"field"`
	Where map[string]any `json:"where,omitempty" yaml:"where,omitempty"`
}

type ValidationIssue struct {
	RuleID      string
	Severity    string
	Message     string
	StatementID string
}

type ContractStatementValidationError struct {
	Issues []ValidationIssue
}

func LoadValidationProfileYAML(raw []byte) (ValidationProfile, error) {
	var profile ValidationProfile
	if err := yaml.Unmarshal(raw, &profile); err != nil {
		return profile, err
	}
	return profile, ValidateValidationProfile(profile)
}

func ValidateValidationProfile(profile ValidationProfile) error {
	if strings.TrimSpace(profile.ID) == "" {
		return errors.New("validation profile id is required")
	}
	if strings.TrimSpace(profile.Version) == "" {
		return errors.New("validation profile version is required")
	}
	if len(profile.Rules) == 0 {
		return errors.New("validation profile must contain at least one rule")
	}
	for _, rule := range profile.Rules {
		if strings.TrimSpace(rule.ID) == "" {
			return errors.New("validation rule id is required")
		}
		if !knownValidationRuleType(rule.Type) {
			return fmt.Errorf("validation rule %q uses unknown type %q", rule.ID, rule.Type)
		}
		if rule.Type == ValidationRuleCount && strings.TrimSpace(rule.Operator) == "" {
			return fmt.Errorf("validation rule %q requires an operator", rule.ID)
		}
		if (rule.Type == ValidationRuleFieldValue || rule.Type == ValidationRuleComparison || rule.Type == ValidationRuleUnique) && strings.TrimSpace(rule.Target) == "" {
			return fmt.Errorf("validation rule %q requires a target", rule.ID)
		}
		if (rule.Type == ValidationRuleValueIn || rule.Type == ValidationRuleSignatureLevel) && strings.TrimSpace(rule.Target) == "" {
			return fmt.Errorf("validation rule %q requires a target", rule.ID)
		}
		if rule.Type == ValidationRuleValueIn && len(rule.Values) == 0 {
			return fmt.Errorf("validation rule %q requires values", rule.ID)
		}
		if rule.Type == ValidationRuleSignatureLevel && rule.Value == nil {
			return fmt.Errorf("validation rule %q requires a value", rule.ID)
		}
		if rule.Type == ValidationRuleRequiredFields && len(rule.RequiredFields) == 0 {
			return fmt.Errorf("validation rule %q requires requiredFields", rule.ID)
		}
		if rule.Type == ValidationRuleReferences {
			if len(rule.ReferenceFields) == 0 {
				return fmt.Errorf("validation rule %q requires referenceFields", rule.ID)
			}
			for _, reference := range rule.ReferenceFields {
				if strings.TrimSpace(reference.Field) == "" {
					return fmt.Errorf("validation rule %q contains a reference field without field", rule.ID)
				}
			}
		}
	}
	return nil
}

func ValidateContractStatements(statements []map[string]any, profile ValidationProfile) []ValidationIssue {
	issues := []ValidationIssue{}
	for _, rule := range profile.Rules {
		switch rule.Type {
		case ValidationRuleCount:
			issues = append(issues, evaluateCountRule(statements, rule)...)
		case ValidationRuleExists:
			issues = append(issues, evaluateExistsRule(statements, rule)...)
		case ValidationRuleRequiredFields:
			issues = append(issues, evaluateRequiredFieldsRule(statements, rule)...)
		case ValidationRuleFieldValue:
			issues = append(issues, evaluateFieldValueRule(statements, rule)...)
		case ValidationRuleUnique:
			issues = append(issues, evaluateUniqueRule(statements, rule)...)
		case ValidationRuleComparison:
			issues = append(issues, evaluateComparisonRule(statements, rule)...)
		case ValidationRuleReferences:
			issues = append(issues, evaluateReferencesRule(statements, rule)...)
		case ValidationRuleValueIn:
			issues = append(issues, evaluateStatementValueInRule(statements, rule)...)
		case ValidationRuleSignatureLevel:
			issues = append(issues, evaluateStatementSignatureLevelRule(statements, rule)...)
		default:
			issues = append(issues, validationIssue(rule, "", fmt.Sprintf("unknown validation rule type %q", rule.Type)))
		}
	}
	return issues
}

func FindStatements(statements []map[string]any, where map[string]any) []map[string]any {
	return FilterStatements(statements, func(statement map[string]any) bool {
		return MatchesWhereClause(statement, where)
	})
}

func FilterStatements(statements []map[string]any, predicate func(map[string]any) bool) []map[string]any {
	result := []map[string]any{}
	for _, statement := range statements {
		if predicate(statement) {
			result = append(result, statement)
		}
	}
	return result
}

func CountStatements(statements []map[string]any, where map[string]any) int {
	return len(FindStatements(statements, where))
}

func MatchesWhereClause(statement map[string]any, where map[string]any) bool {
	for field, expected := range where {
		actual, ok := statement[field]
		if !ok || !valuesEqual(actual, expected) {
			return false
		}
	}
	return true
}

func evaluateCountRule(statements []map[string]any, rule ValidationRule) []ValidationIssue {
	count := CountStatements(statements, rule.Where)
	if compareValues(count, rule.Operator, rule.Value) {
		return nil
	}
	return []ValidationIssue{validationIssue(rule, "", "")}
}

func evaluateExistsRule(statements []map[string]any, rule ValidationRule) []ValidationIssue {
	if CountStatements(statements, rule.Where) > 0 {
		return nil
	}
	return []ValidationIssue{validationIssue(rule, "", "")}
}

func evaluateRequiredFieldsRule(statements []map[string]any, rule ValidationRule) []ValidationIssue {
	matches := FindStatements(statements, rule.Where)
	if len(matches) == 0 {
		return []ValidationIssue{validationIssue(rule, "", "")}
	}
	issues := []ValidationIssue{}
	for _, statement := range matches {
		for _, field := range rule.RequiredFields {
			if missingStatementField(statement, field) {
				issues = append(issues, validationIssue(rule, statementID(statement), ""))
				break
			}
		}
	}
	return issues
}

func evaluateFieldValueRule(statements []map[string]any, rule ValidationRule) []ValidationIssue {
	issues := []ValidationIssue{}
	for _, statement := range FindStatements(statements, rule.Where) {
		if !compareValues(statement[rule.Target], defaultOperator(rule.Operator, "eq"), rule.Value) {
			issues = append(issues, validationIssue(rule, statementID(statement), ""))
		}
	}
	return issues
}

func evaluateUniqueRule(statements []map[string]any, rule ValidationRule) []ValidationIssue {
	seen := map[string]string{}
	issues := []ValidationIssue{}
	for _, statement := range FindStatements(statements, rule.Where) {
		value, ok := statement[rule.Target]
		if !ok {
			continue
		}
		key := fmt.Sprintf("%#v", value)
		if firstID, duplicate := seen[key]; duplicate {
			currentID := statementID(statement)
			issues = append(issues, validationIssue(rule, currentID, fmt.Sprintf("%s duplicates %s", issueMessage(rule), firstID)))
			continue
		}
		seen[key] = statementID(statement)
	}
	return issues
}

func evaluateComparisonRule(statements []map[string]any, rule ValidationRule) []ValidationIssue {
	issues := []ValidationIssue{}
	for _, statement := range FindStatements(statements, rule.Where) {
		if !compareValues(statement[rule.Target], rule.Operator, rule.Value) {
			issues = append(issues, validationIssue(rule, statementID(statement), ""))
		}
	}
	return issues
}

func evaluateReferencesRule(statements []map[string]any, rule ValidationRule) []ValidationIssue {
	sources := FindStatements(statements, rule.Where)
	issues := []ValidationIssue{}
	for _, source := range sources {
		for _, reference := range rule.ReferenceFields {
			referencedID, _ := source[reference.Field].(string)
			if referencedID == "" || !statementReferenceExists(statements, referencedID, reference.Where) {
				issues = append(issues, validationIssue(rule, statementID(source), ""))
				break
			}
		}
	}
	return issues
}

func evaluateStatementValueInRule(statements []map[string]any, rule ValidationRule) []ValidationIssue {
	issues := []ValidationIssue{}
	allowed := normalizedSet(rule.Values)
	for _, statement := range FindStatements(statements, rule.Where) {
		// A vocabulary rule constrains values that are present; requiring the
		// field at all is a required_fields rule's job.
		value, ok := statement[rule.Target]
		if !ok {
			continue
		}
		if !allowed[strings.ToUpper(strings.TrimSpace(fmt.Sprint(value)))] {
			issues = append(issues, validationIssue(rule, statementID(statement), ""))
		}
	}
	return issues
}

func evaluateStatementSignatureLevelRule(statements []map[string]any, rule ValidationRule) []ValidationIssue {
	issues := []ValidationIssue{}
	required, _ := rule.Value.(string)
	for _, statement := range FindStatements(statements, rule.Where) {
		actual, _ := statement[rule.Target].(string)
		if !signatureLevelSatisfies(actual, required) {
			issues = append(issues, validationIssue(rule, statementID(statement), ""))
		}
	}
	return issues
}

func statementReferenceExists(statements []map[string]any, id string, where map[string]any) bool {
	for _, statement := range statements {
		statementID, _ := statement["@id"].(string)
		if statementID != id {
			continue
		}
		return MatchesWhereClause(statement, where)
	}
	return false
}

func missingStatementField(statement map[string]any, field string) bool {
	value, ok := statement[field]
	if !ok || value == nil {
		return true
	}
	text, ok := value.(string)
	return ok && strings.TrimSpace(text) == ""
}

func compareValues(actual any, operator string, expected any) bool {
	switch strings.ToLower(strings.TrimSpace(operator)) {
	case "", "eq", "equals", "==":
		return valuesEqual(actual, expected)
	case "ne", "not_equals", "!=":
		return !valuesEqual(actual, expected)
	case "gt", "greater_than", "greaterthan", ">":
		left, leftOK := numericValue(actual)
		right, rightOK := numericValue(expected)
		return leftOK && rightOK && left > right
	case "gte", "greater_than_or_equal", "greaterthanorequal", ">=":
		left, leftOK := numericValue(actual)
		right, rightOK := numericValue(expected)
		return leftOK && rightOK && left >= right
	case "lt", "less_than", "lessthan", "<":
		left, leftOK := numericValue(actual)
		right, rightOK := numericValue(expected)
		return leftOK && rightOK && left < right
	case "lte", "less_than_or_equal", "lessthanorequal", "<=":
		left, leftOK := numericValue(actual)
		right, rightOK := numericValue(expected)
		return leftOK && rightOK && left <= right
	case "in":
		values, ok := asArray(expected)
		if !ok {
			return false
		}
		for _, item := range values {
			if valuesEqual(actual, item) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func valuesEqual(left any, right any) bool {
	if leftNumber, leftOK := numericValue(left); leftOK {
		if rightNumber, rightOK := numericValue(right); rightOK {
			return leftNumber == rightNumber
		}
	}
	return reflect.DeepEqual(left, right)
}

func validationIssue(rule ValidationRule, statementID string, message string) ValidationIssue {
	if message == "" {
		message = issueMessage(rule)
	}
	return ValidationIssue{
		RuleID:      rule.ID,
		Severity:    defaultSeverity(rule.Severity),
		Message:     message,
		StatementID: statementID,
	}
}

func issueMessage(rule ValidationRule) string {
	if strings.TrimSpace(rule.Message) != "" {
		return rule.Message
	}
	return fmt.Sprintf("validation rule %q failed", rule.ID)
}

func defaultSeverity(severity string) string {
	if strings.TrimSpace(severity) == "" {
		return "error"
	}
	return severity
}

func defaultOperator(operator string, fallback string) string {
	if strings.TrimSpace(operator) == "" {
		return fallback
	}
	return operator
}

func statementID(statement map[string]any) string {
	id, _ := statement["@id"].(string)
	return id
}

func knownValidationRuleType(ruleType string) bool {
	switch ruleType {
	case ValidationRuleCount, ValidationRuleExists, ValidationRuleRequiredFields, ValidationRuleFieldValue, ValidationRuleUnique, ValidationRuleComparison, ValidationRuleReferences, ValidationRuleValueIn, ValidationRuleSignatureLevel:
		return true
	default:
		return false
	}
}
