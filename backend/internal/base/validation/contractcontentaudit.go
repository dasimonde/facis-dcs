package validation

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type ContractContentAuditMetadata struct {
	ContractDID     string
	ContractVersion string
	PolicyVersion   string
	AuditedBy       string
	HolderDID       string
}

type ContractContentPolicy struct {
	PolicySetID string               `json:"policySetId"`
	Version     string               `json:"version"`
	Policies    []any                `json:"dcs:policies"`
	SHACLShapes []ContractSHACLShape `json:"shaclShapes"`
	SHACLFiles  []string             `json:"shaclShapeFiles"`
	Profiles    []string             `json:"validationProfiles"`
	profiles    []ValidationProfile
	SHACL       *ContractSHACLPolicy `json:"shacl"`
}

type ContractSHACLPolicy struct {
	Shapes []ContractSHACLShape `json:"shapes"`
}

type ContractSHACLShape struct {
	ID           string                  `json:"id"`
	Title        string                  `json:"title"`
	Severity     string                  `json:"severity"`
	TargetClass  string                  `json:"targetClass"`
	OntologyTerm string                  `json:"ontologyTerm"`
	Properties   []ContractSHACLProperty `json:"properties"`
	Property     []ContractSHACLProperty `json:"property"`
}

type ContractSHACLProperty struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Path         string   `json:"path"`
	MinCount     *int     `json:"minCount"`
	MaxCount     *int     `json:"maxCount"`
	MinInclusive *float64 `json:"minInclusive"`
	Datatype     string   `json:"datatype"`
	Class        string   `json:"class"`
	Node         string   `json:"node"`
	In           []string `json:"in"`
	Severity     string   `json:"severity"`
	Message      string   `json:"message"`
	OntologyTerm string   `json:"ontologyTerm"`
}

func AuditContractContent(contractDocument any, policyDocument any, metadata ContractContentAuditMetadata) ([]PolicyFinding, error) {
	contract, err := normalizeObject(contractDocument)
	if err != nil {
		return nil, fmt.Errorf("decode contract document: %w", err)
	}
	policy, err := normalizeContractContentPolicy(policyDocument, metadata)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(metadata.PolicyVersion) != "" {
		policy.Version = metadata.PolicyVersion
	}

	findings := []PolicyFinding{}
	shapes := contractSHACLShapes(policy)
	shapeIndex := contractSHACLShapeIndex(shapes)
	for _, shape := range shapes {
		if !isRootContractAuditShape(contract, shape) {
			continue
		}
		findings = append(findings, auditContractSHACLShape(contract, policy, shape, shapeIndex)...)
	}
	for _, profile := range policy.profiles {
		findings = append(findings, auditContractValidationProfile(contract, profile)...)
	}
	findings = append(findings, auditContractODRLPolicies(contract, extractContractODRLPolicies(contract))...)
	findings = append(findings, auditContractODRLPolicies(contract, externalODRLPolicies(policy.Policies))...)
	for i := range findings {
		findings[i].PolicySetID = policy.PolicySetID
		findings[i].PolicyVersion = policy.Version
	}
	return findings, nil
}

type ContractPolicySatisfactionError struct {
	Findings []PolicyFinding
}

func (e ContractPolicySatisfactionError) Error() string {
	if len(e.Findings) == 0 {
		return "contract policy validation failed"
	}
	messages := make([]string, 0, len(e.Findings))
	for _, finding := range e.Findings {
		message := strings.TrimSpace(finding.Message)
		if message == "" {
			message = strings.TrimSpace(finding.RuleID)
		}
		if message != "" {
			messages = append(messages, message)
		}
	}
	if len(messages) == 0 {
		return "contract policy validation failed"
	}
	return "contract policy validation failed: " + strings.Join(messages, "; ")
}

// ValidateContractPolicySatisfaction enforces the per-contract ODRL policies
// embedded as dcs:policies against the submitted semanticConditionValues.
func ValidateContractPolicySatisfaction(contractDocument any, metadata ContractContentAuditMetadata) error {
	contract, err := normalizeObject(contractDocument)
	if err != nil {
		return fmt.Errorf("decode contract document: %w", err)
	}
	findings := auditContractODRLPolicies(contract, extractContractODRLPolicies(contract))
	blocking := make([]PolicyFinding, 0)
	for _, finding := range findings {
		if isBlockingContractPolicyFinding(finding) {
			finding.PolicySetID = defaultContractPolicySetID
			finding.PolicyVersion = metadata.PolicyVersion
			if strings.TrimSpace(finding.PolicyVersion) == "" {
				finding.PolicyVersion = defaultContractPolicyVersion
			}
			blocking = append(blocking, finding)
		}
	}
	if len(blocking) > 0 {
		return ContractPolicySatisfactionError{Findings: blocking}
	}
	return nil
}

func isBlockingContractPolicyFinding(finding PolicyFinding) bool {
	switch strings.ToLower(strings.TrimSpace(finding.Severity)) {
	case "error", "blocking":
		return true
	default:
		return false
	}
}

func normalizeContractContentPolicy(raw any, metadata ContractContentAuditMetadata) (ContractContentPolicy, error) {
	if raw == nil {
		loaded, err := loadDefaultContractContentPolicyDocument()
		if err != nil {
			return ContractContentPolicy{}, err
		}
		raw = loaded
	}

	normalized, err := normalizeObject(raw)
	if err != nil {
		return ContractContentPolicy{}, fmt.Errorf("decode contract content policy: %w", err)
	}
	bytes, err := json.Marshal(normalized)
	if err != nil {
		return ContractContentPolicy{}, err
	}
	var policy ContractContentPolicy
	if err := json.Unmarshal(bytes, &policy); err != nil {
		return ContractContentPolicy{}, fmt.Errorf("decode contract content policy: %w", err)
	}
	if strings.TrimSpace(policy.PolicySetID) == "" {
		policy.PolicySetID = defaultContractPolicySetID
	}
	if strings.TrimSpace(policy.Version) == "" {
		policy.Version = metadata.PolicyVersion
		if strings.TrimSpace(policy.Version) == "" {
			policy.Version = defaultContractPolicyVersion
		}
	}
	fileShapes, err := loadContractSHACLShapeFiles(policy.SHACLFiles)
	if err != nil {
		return ContractContentPolicy{}, err
	}
	policy.SHACLShapes = append(policy.SHACLShapes, fileShapes...)
	profiles, err := loadContractValidationProfiles(policy.Profiles)
	if err != nil {
		return ContractContentPolicy{}, err
	}
	policy.profiles = profiles
	return policy, nil
}

const (
	defaultContractPolicySetID   = "facis.dcs.contract.structure-semantics"
	defaultContractPolicyVersion = "v1"
	defaultContractPolicyFile    = "docs/policies/facis-contract-content-audit-policies.json"
)

func loadDefaultContractContentPolicyDocument() (map[string]any, error) {
	path, err := resolveContractContentPolicyFile()
	if err != nil {
		return nil, err
	}
	bytes, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read contract content policy file %q: %w", path, err)
	}
	var policy map[string]any
	if err := json.Unmarshal(bytes, &policy); err != nil {
		return nil, fmt.Errorf("decode contract content policy file %q: %w", path, err)
	}
	return policy, nil
}

func resolveContractContentPolicyFile() (string, error) {
	if path := strings.TrimSpace(os.Getenv("FACIS_CONTRACT_CONTENT_POLICY_FILE")); path != "" {
		return path, nil
	}
	candidates := []string{
		defaultContractPolicyFile,
		filepath.Join("..", defaultContractPolicyFile),
		filepath.Join("..", "..", defaultContractPolicyFile),
		filepath.Join("..", "..", "..", defaultContractPolicyFile),
		filepath.Join("..", "..", "..", "..", defaultContractPolicyFile),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("contract content policy file not found")
}

func loadContractSHACLShapeFiles(files []string) ([]ContractSHACLShape, error) {
	shapes := []ContractSHACLShape{}
	for _, file := range files {
		path, err := resolveContractContentDocumentFile(file)
		if err != nil {
			return nil, err
		}
		bytes, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read contract SHACL shapes file %q: %w", path, err)
		}
		parsed, err := parseContractSHACLShapesTTL(string(bytes))
		if err != nil {
			return nil, fmt.Errorf("parse contract SHACL shapes file %q: %w", path, err)
		}
		shapes = append(shapes, parsed...)
	}
	return shapes, nil
}

func loadContractValidationProfiles(files []string) ([]ValidationProfile, error) {
	profiles := []ValidationProfile{}
	for _, file := range files {
		path, err := resolveContractContentDocumentFile(file)
		if err != nil {
			return nil, err
		}
		profile, err := LoadValidationProfileFile(path)
		if err != nil {
			return nil, fmt.Errorf("load contract validation profile %q: %w", path, err)
		}
		profiles = append(profiles, profile)
	}
	return profiles, nil
}

func resolveContractContentDocumentFile(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("contract content document path is required")
	}
	if filepath.IsAbs(path) {
		return path, nil
	}
	candidates := []string{
		path,
		filepath.Join("..", path),
		filepath.Join("..", "..", path),
		filepath.Join("..", "..", "..", path),
		filepath.Join("..", "..", "..", "..", path),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("contract content document file %q not found", path)
}

func parseContractSHACLShapesTTL(content string) ([]ContractSHACLShape, error) {
	shapes := []ContractSHACLShape{}
	for _, statement := range ontologyStatements(content) {
		if !contractSHACLStatementHasType(statement, "sh:NodeShape") {
			continue
		}
		targetClass := ontologyResource(statement, "sh:targetClass")
		if targetClass == "" {
			continue
		}
		subject := ontologySubject(statement)
		shape := ContractSHACLShape{
			ID:          subject,
			Title:       fmt.Sprintf("%s SHACL shape", compactTerm(targetClass)),
			Severity:    shaclSeverity(statement),
			TargetClass: targetClass,
			Properties:  parseContractSHACLPropertiesTTL(subject, statement),
		}
		shapes = append(shapes, shape)
	}
	return shapes, nil
}

func contractSHACLStatementHasType(statement string, class string) bool {
	expandedClass := expandOntologyResource(class)
	for _, line := range strings.Split(statement, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		var candidates []string
		switch {
		case fields[0] == "a":
			candidates = fields[1:]
		case len(fields) >= 3 && fields[1] == "a":
			candidates = fields[2:]
		default:
			continue
		}
		for _, rawClass := range candidates {
			candidate := strings.TrimSuffix(strings.TrimSuffix(rawClass, ";"), ",")
			if expandOntologyResource(candidate) == expandedClass {
				return true
			}
		}
	}
	return false
}

func parseContractSHACLPropertiesTTL(shapeID string, statement string) []ContractSHACLProperty {
	properties := []ContractSHACLProperty{}
	remaining := statement
	for {
		start := strings.Index(remaining, "sh:property [")
		if start < 0 {
			break
		}
		blockStart := start + len("sh:property [")
		end := strings.Index(remaining[blockStart:], "]")
		if end < 0 {
			break
		}
		block := remaining[blockStart : blockStart+end]
		index := len(properties)
		property := ContractSHACLProperty{
			ID:           fmt.Sprintf("%s-PROP-%03d", shapeID, index+1),
			Path:         ontologyResource(block, "sh:path"),
			Datatype:     ontologyResource(block, "sh:datatype"),
			Class:        ontologyResource(block, "sh:class"),
			Node:         ontologyResource(block, "sh:node"),
			Message:      ontologyString(block, "sh:message"),
			In:           parseContractSHACLInValues(block),
			MinCount:     ontologyInt(block, "sh:minCount"),
			MaxCount:     ontologyInt(block, "sh:maxCount"),
			MinInclusive: ontologyNumber(block, "sh:minInclusive"),
		}
		properties = append(properties, property)
		remaining = remaining[blockStart+end+1:]
	}
	return properties
}

func parseContractSHACLInValues(block string) []string {
	line := shaclPredicateLine(block, "sh:in")
	if line == "" {
		return nil
	}
	start := strings.Index(line, "(")
	end := strings.LastIndex(line, ")")
	if start < 0 || end <= start {
		return nil
	}
	content := line[start+1 : end]
	values := []string{}
	quoted := regexp.MustCompile(`"([^"]*)"`)
	for _, match := range quoted.FindAllStringSubmatch(content, -1) {
		values = append(values, match[1])
	}
	content = quoted.ReplaceAllString(content, " ")
	for _, token := range strings.Fields(content) {
		values = append(values, strings.TrimSpace(token))
	}
	return values
}

func ontologyInt(statement string, predicate string) *int {
	for _, line := range strings.Split(statement, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), predicate+" ") {
			continue
		}
		match := ontologyNumberValue.FindString(line)
		if match == "" {
			return nil
		}
		value, err := strconv.Atoi(match)
		if err == nil {
			return &value
		}
	}
	return nil
}

func auditContractValidationProfile(contract map[string]any, profile ValidationProfile) []PolicyFinding {
	findings := []PolicyFinding{}
	statementRules := []ValidationRule{}
	for _, rule := range profile.Rules {
		if len(rule.Where) > 0 {
			statementRules = append(statementRules, rule)
			continue
		}
		findings = append(findings, auditContractValidationRule(contract, rule)...)
	}
	if len(statementRules) > 0 {
		findings = append(findings, auditContractStatementValidationRules(contract, ValidationProfile{
			ID:          profile.ID,
			Version:     profile.Version,
			Description: profile.Description,
			Rules:       statementRules,
		})...)
	}
	for i := range findings {
		findings[i].PolicySetID = profile.ID
		findings[i].PolicyVersion = profile.Version
	}
	return findings
}

func auditContractValidationRule(contract map[string]any, rule ValidationRule) []PolicyFinding {
	switch rule.Type {
	case ValidationRuleRequiredFields:
		findings := []PolicyFinding{}
		for _, field := range rule.RequiredFields {
			if value, ok := contractValue(contract, field); !ok || isEmptyAuditValue(value) {
				findings = append(findings, validationRuleFinding(rule, field, defaultSeverity(rule.Severity), fmt.Sprintf("required field %q is missing", field)))
			}
		}
		if len(findings) == 0 {
			return []PolicyFinding{validationRuleFinding(rule, strings.Join(rule.RequiredFields, ", "), "info", issueMessage(rule))}
		}
		return findings
	case ValidationRuleFieldValue:
		value, ok := contractValue(contract, rule.Target)
		if !ok || !compareValues(value, defaultOperator(rule.Operator, "eq"), rule.Value) {
			return []PolicyFinding{validationRuleFindingWithDetails(rule, rule.Target, defaultSeverity(rule.Severity), issueMessage(rule), optionalActualValue(value, ok), rule.Value, nil, defaultOperator(rule.Operator, "eq"))}
		}
		return []PolicyFinding{validationRuleFindingWithDetails(rule, rule.Target, "info", issueMessage(rule), value, rule.Value, nil, defaultOperator(rule.Operator, "eq"))}
	case ValidationRuleComparison:
		value, ok := contractValue(contract, rule.Target)
		if !ok || !compareValues(value, rule.Operator, rule.Value) {
			return []PolicyFinding{validationRuleFindingWithDetails(rule, rule.Target, defaultSeverity(rule.Severity), issueMessage(rule), optionalActualValue(value, ok), rule.Value, nil, rule.Operator)}
		}
		return []PolicyFinding{validationRuleFindingWithDetails(rule, rule.Target, "info", issueMessage(rule), value, rule.Value, nil, rule.Operator)}
	case ValidationRuleValueIn:
		value, ok := contractString(contract, rule.Target)
		if !ok || !normalizedSet(rule.Values)[strings.ToUpper(strings.TrimSpace(value))] {
			return []PolicyFinding{validationRuleFindingWithDetails(rule, rule.Target, defaultSeverity(rule.Severity), issueMessage(rule), optionalActualValue(value, ok), nil, anySliceFromStrings(rule.Values), "in")}
		}
		return []PolicyFinding{validationRuleFindingWithDetails(rule, rule.Target, "info", issueMessage(rule), value, nil, anySliceFromStrings(rule.Values), "in")}
	case ValidationRuleSignatureLevel:
		value, ok := contractString(contract, rule.Target)
		required, _ := rule.Value.(string)
		if !ok || !signatureLevelSatisfies(value, required) {
			return []PolicyFinding{validationRuleFindingWithDetails(rule, rule.Target, defaultSeverity(rule.Severity), issueMessage(rule), optionalActualValue(value, ok), required, nil, "atLeast")}
		}
		return []PolicyFinding{validationRuleFindingWithDetails(rule, rule.Target, "info", issueMessage(rule), value, required, nil, "atLeast")}
	default:
		return nil
	}
}

func auditContractStatementValidationRules(contract map[string]any, profile ValidationProfile) []PolicyFinding {
	statements := contractStatementsFromDocument(contract)
	if len(statements) == 0 {
		return nil
	}
	issues := ValidateContractStatements(statements, profile)
	findings := make([]PolicyFinding, 0, len(issues))
	for _, issue := range issues {
		findings = append(findings, contractFinding(issue.RuleID, issue.RuleID, issue.Severity, issue.Message, issue.StatementID, issue.StatementID, "dcs:ContractStatement"))
	}
	return findings
}

func contractStatementsFromDocument(contract map[string]any) []map[string]any {
	statementSet, ok := contract[statementSetDocumentProperty()].(map[string]any)
	if !ok {
		return nil
	}
	rawStatements, ok := asArray(statementSet["statements"])
	if !ok {
		return nil
	}
	statements := make([]map[string]any, 0, len(rawStatements))
	for _, raw := range rawStatements {
		if statement, ok := raw.(map[string]any); ok {
			statements = append(statements, statement)
		}
	}
	return statements
}

func validationRuleFinding(rule ValidationRule, path string, severity string, message string) PolicyFinding {
	return validationRuleFindingWithDetails(rule, path, severity, message, nil, nil, nil, "")
}

func validationRuleFindingWithDetails(rule ValidationRule, path string, severity string, message string, actualValue any, expectedValue any, expectedValues []any, operator string) PolicyFinding {
	finding := contractFinding(rule.ID, rule.ID, severity, message, path, path, "")
	if len(rule.RequiredFields) > 0 && operator == "" {
		operator = "exists"
		expectedValues = anySliceFromStrings(rule.RequiredFields)
	}
	applyPolicyDetails(&finding, path, operator, actualValue, expectedValue, expectedValues)
	return finding
}

func applyODRLPolicyDetails(finding *PolicyFinding, path string, operator string, actualValue any, hasActualValue bool, rightOperand any) {
	expectedValue, expectedValues := odrlExpectedValues(rightOperand)
	if !hasActualValue {
		actualValue = nil
	}
	applyPolicyDetails(finding, path, operator, actualValue, expectedValue, expectedValues)
}

func applyPolicyDetails(finding *PolicyFinding, path string, operator string, actualValue any, expectedValue any, expectedValues []any) {
	normalizedOperator := normalizePolicyOperator(operator)
	if normalizedOperator != "" {
		finding.Operator = normalizedOperator
	}
	if actualValue != nil {
		finding.ActualValue = compactAuditValue(actualValue)
	}
	if expectedValue != nil {
		finding.ExpectedValue = compactAuditValue(expectedValue)
	}
	if len(expectedValues) > 0 {
		finding.ExpectedValues = compactAuditValues(expectedValues)
	}
	if strings.TrimSpace(finding.Requirement) == "" {
		finding.Requirement = policyRequirement(path, normalizedOperator, finding.ExpectedValue, finding.ExpectedValues)
	}
}

func odrlExpectedValues(rightOperand any) (any, []any) {
	value := compactAuditValue(rightOperand)
	if items, ok := value.([]any); ok {
		return nil, items
	}
	return value, nil
}

func policyRequirement(path string, operator string, expectedValue any, expectedValues []any) string {
	path = strings.TrimSpace(path)
	if path == "" || operator == "" {
		return ""
	}
	switch operator {
	case "exists":
		if len(expectedValues) > 0 {
			return fmt.Sprintf("%s must contain %s", path, formatRequirementValues(expectedValues))
		}
		return fmt.Sprintf("%s must be present", path)
	case "in":
		return fmt.Sprintf("%s must be one of %s", path, formatRequirementValues(expectedValues))
	case "notIn":
		return fmt.Sprintf("%s must be none of %s", path, formatRequirementValues(expectedValues))
	case "minCount":
		return fmt.Sprintf("%s requires at least %s value(s)", path, formatRequirementValue(expectedValue))
	case "maxCount":
		return fmt.Sprintf("%s allows at most %s value(s)", path, formatRequirementValue(expectedValue))
	case "datatype":
		return fmt.Sprintf("%s must use datatype %s", path, formatRequirementValue(expectedValue))
	case "class":
		return fmt.Sprintf("%s must reference class %s", path, formatRequirementValue(expectedValue))
	case "node":
		return fmt.Sprintf("%s must conform to %s", path, formatRequirementValue(expectedValue))
	case "atLeast":
		return fmt.Sprintf("%s must be at least %s", path, formatRequirementValue(expectedValue))
	case "eq":
		return fmt.Sprintf("%s must equal %s", path, formatRequirementValue(expectedValue))
	case "neq":
		return fmt.Sprintf("%s must not equal %s", path, formatRequirementValue(expectedValue))
	case "contains":
		return fmt.Sprintf("%s must contain %s", path, formatRequirementValue(expectedValue))
	default:
		if symbol := policyOperatorSymbol(operator); symbol != "" {
			return fmt.Sprintf("%s must be %s %s", path, symbol, formatRequirementValue(expectedValue))
		}
		return fmt.Sprintf("%s must satisfy %s %s", path, operator, formatRequirementValue(expectedValue))
	}
}

func normalizePolicyOperator(operator string) string {
	switch strings.ToLower(strings.TrimSpace(compactTerm(operator))) {
	case "":
		return ""
	case "gte", "gteq", "greaterthanorequal", "greaterthanorequalto", "mininclusive":
		return "gte"
	case "lte", "lteq", "lessthanorequal", "lessthanorequalto", "maxinclusive":
		return "lte"
	case "gt", "greaterthan":
		return "gt"
	case "lt", "lessthan":
		return "lt"
	case "eq", "equals", "equalto":
		return "eq"
	case "neq", "noteq", "notequals", "notequalto":
		return "neq"
	case "isanyof", "in":
		return "in"
	case "isnoneof", "notin":
		return "notIn"
	case "haspart", "contains":
		return "contains"
	case "mincount":
		return "minCount"
	case "maxcount":
		return "maxCount"
	case "datatype":
		return "datatype"
	case "class":
		return "class"
	case "node":
		return "node"
	case "exists", "required":
		return "exists"
	case "atleast":
		return "atLeast"
	default:
		return strings.TrimSpace(compactTerm(operator))
	}
}

func policyOperatorSymbol(operator string) string {
	switch operator {
	case "gte":
		return ">="
	case "lte":
		return "<="
	case "gt":
		return ">"
	case "lt":
		return "<"
	default:
		return ""
	}
}

func formatRequirementValues(values []any) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, formatRequirementValue(value))
	}
	return strings.Join(parts, ", ")
}

func formatRequirementValue(value any) string {
	switch typed := compactAuditValue(value).(type) {
	case nil:
		return ""
	case string:
		return typed
	case []any:
		return formatRequirementValues(typed)
	default:
		return fmt.Sprint(typed)
	}
}

func optionalActualValue(value any, ok bool) any {
	if !ok {
		return nil
	}
	return value
}

func anySliceFromStrings(values []string) []any {
	if len(values) == 0 {
		return nil
	}
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func compactAuditValues(values []any) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		result = append(result, compactAuditValue(value))
	}
	return result
}

func compactAuditValue(value any) any {
	value = compactJSONLDValue(value)
	switch typed := value.(type) {
	case []any:
		return compactAuditValues(typed)
	default:
		return typed
	}
}

func contractSHACLShapes(policy ContractContentPolicy) []ContractSHACLShape {
	shapes := make([]ContractSHACLShape, 0, len(policy.SHACLShapes))
	shapes = append(shapes, policy.SHACLShapes...)
	if policy.SHACL != nil {
		shapes = append(shapes, policy.SHACL.Shapes...)
	}
	return shapes
}

func contractSHACLShapeIndex(shapes []ContractSHACLShape) map[string]ContractSHACLShape {
	index := make(map[string]ContractSHACLShape, len(shapes))
	for _, shape := range shapes {
		normalized := normalizeContractSHACLShape(shape)
		index[normalized.ID] = normalized
		index[compactTerm(normalized.ID)] = normalized
	}
	return index
}

func isRootContractAuditShape(contract map[string]any, shape ContractSHACLShape) bool {
	targetClass := strings.TrimSpace(shape.TargetClass)
	if targetClass == "" {
		return true
	}
	if compactTerm(targetClass) == "Contract" {
		return true
	}
	return jsonLDTypeMatches(valuesAtPath(contract, "@type"), targetClass)
}

func auditContractSHACLShape(contract map[string]any, policy ContractContentPolicy, shape ContractSHACLShape, shapeIndex map[string]ContractSHACLShape) []PolicyFinding {
	shape = normalizeContractSHACLShape(shape)
	if shape.TargetClass != "" && !jsonLDTypeMatches(valuesAtPath(contract, "@type"), shape.TargetClass) {
		return []PolicyFinding{contractStructureFinding(policy, shape.ID, shape.Title, shape.Severity, fmt.Sprintf("target class %q does not match contract @type", shape.TargetClass), "@type", shape.TargetClass)}
	}

	properties := shape.Properties
	properties = append(properties, shape.Property...)
	findings := []PolicyFinding{}
	for index, property := range properties {
		findings = append(findings, auditContractSHACLProperty(contract, policy, shape, normalizeContractSHACLProperty(shape, property, index), shapeIndex)...)
	}
	if len(findings) == 0 {
		findings = append(findings, contractStructureFinding(policy, shape.ID, shape.Title, "info", "SHACL shape conforms", shape.TargetClass, shape.TargetClass))
	}
	return findings
}

func auditContractSHACLProperty(contract map[string]any, policy ContractContentPolicy, shape ContractSHACLShape, property ContractSHACLProperty, shapeIndex map[string]ContractSHACLShape) []PolicyFinding {
	values := valuesAtPath(contract, property.Path)
	nonEmpty := nonEmptyValues(values)
	findings := []PolicyFinding{}
	if property.MinCount != nil && len(nonEmpty) < *property.MinCount {
		findings = append(findings, shaclPropertyFindingWithDetails(policy, shape, property, fmt.Sprintf("%s requires at least %d value(s)", propertyLabel(property), *property.MinCount), len(nonEmpty), *property.MinCount, nil, "minCount"))
	}
	if property.MaxCount != nil && len(nonEmpty) > *property.MaxCount {
		findings = append(findings, shaclPropertyFindingWithDetails(policy, shape, property, fmt.Sprintf("%s allows at most %d value(s)", propertyLabel(property), *property.MaxCount), len(nonEmpty), *property.MaxCount, nil, "maxCount"))
	}
	if len(nonEmpty) == 0 {
		return findings
	}
	if property.Datatype != "" {
		for _, value := range values {
			if !valueConformsDatatype(value, property.Datatype) {
				findings = append(findings, shaclPropertyFindingWithDetails(policy, shape, property, fmt.Sprintf("%s must use datatype %s", propertyLabel(property), property.Datatype), compactJSONLDValue(value), property.Datatype, nil, "datatype"))
				break
			}
		}
	}
	if property.MinInclusive != nil {
		for _, value := range values {
			number, ok := toFloat(value)
			if !ok || number+floatTolerance < *property.MinInclusive {
				findings = append(findings, shaclPropertyFindingWithDetails(policy, shape, property, fmt.Sprintf("%s must be at least %.4g", propertyLabel(property), *property.MinInclusive), compactJSONLDValue(value), *property.MinInclusive, nil, "gte"))
				break
			}
		}
	}
	if len(property.In) > 0 {
		allowed := normalizedSet(property.In)
		matched := false
		for _, value := range values {
			if allowed[strings.ToUpper(strings.TrimSpace(fmt.Sprint(compactJSONLDValue(value))))] {
				matched = true
				break
			}
		}
		if !matched {
			findings = append(findings, shaclPropertyFindingWithDetails(policy, shape, property, fmt.Sprintf("%s must be one of %s", propertyLabel(property), strings.Join(property.In, ", ")), compactAuditValues(values), nil, anySliceFromStrings(property.In), "in"))
		}
	}
	if property.Class != "" {
		for _, value := range values {
			if !valueHasClass(value, property.Class) {
				findings = append(findings, shaclPropertyFindingWithDetails(policy, shape, property, fmt.Sprintf("%s must reference class %s", propertyLabel(property), property.Class), compactJSONLDValue(value), property.Class, nil, "class"))
				break
			}
		}
	}
	if property.Node != "" {
		if nestedShape, ok := shapeIndex[property.Node]; ok {
			for _, value := range values {
				nested, ok := value.(map[string]any)
				if !ok {
					findings = append(findings, shaclPropertyFindingWithDetails(policy, shape, property, fmt.Sprintf("%s must be an object conforming to %s", propertyLabel(property), property.Node), compactJSONLDValue(value), property.Node, nil, "node"))
					break
				}
				for _, nestedFinding := range auditContractSHACLShape(nested, policy, nestedShape, shapeIndex) {
					if nestedFinding.Severity == "info" {
						continue
					}
					if nestedFinding.Path != "" {
						nestedFinding.Path = property.Path + "." + nestedFinding.Path
						nestedFinding.SemanticPath = nestedFinding.Path
					}
					findings = append(findings, nestedFinding)
				}
			}
		}
	}
	if len(findings) == 0 {
		return []PolicyFinding{contractFinding(property.ID, shape.Title, "info", fmt.Sprintf("%s conforms", propertyLabel(property)), property.Path, property.Path, property.OntologyTerm)}
	}
	return findings
}

func normalizeContractSHACLShape(shape ContractSHACLShape) ContractSHACLShape {
	if strings.TrimSpace(shape.ID) == "" {
		shape.ID = "FACIS-CONTRACT-SHACL-CUSTOM"
	}
	if strings.TrimSpace(shape.Title) == "" {
		shape.Title = shape.ID
	}
	if strings.TrimSpace(shape.Severity) == "" {
		shape.Severity = "error"
	}
	return shape
}

func normalizeContractSHACLProperty(shape ContractSHACLShape, property ContractSHACLProperty, index int) ContractSHACLProperty {
	if strings.TrimSpace(property.ID) == "" {
		property.ID = fmt.Sprintf("%s-PROP-%03d", shape.ID, index+1)
	}
	if strings.TrimSpace(property.Severity) == "" {
		property.Severity = shape.Severity
	}
	if strings.TrimSpace(property.OntologyTerm) == "" {
		property.OntologyTerm = shape.OntologyTerm
	}
	return property
}

func contractStructureFinding(policy ContractContentPolicy, ruleID, title, severity, message, path, ontologyTerm string) PolicyFinding {
	finding := contractFinding(ruleID, title, severity, message, path, path, ontologyTerm)
	finding.PolicySetID = policy.PolicySetID
	finding.PolicyVersion = policy.Version
	return finding
}

func shaclPropertyFindingWithDetails(policy ContractContentPolicy, shape ContractSHACLShape, property ContractSHACLProperty, fallbackMessage string, actualValue any, expectedValue any, expectedValues []any, operator string) PolicyFinding {
	message := property.Message
	if strings.TrimSpace(message) == "" {
		message = fallbackMessage
	}
	finding := contractFinding(property.ID, shape.Title, property.Severity, message, property.Path, property.Path, property.OntologyTerm)
	finding.PolicySetID = policy.PolicySetID
	finding.PolicyVersion = policy.Version
	applyPolicyDetails(&finding, property.Path, operator, actualValue, expectedValue, expectedValues)
	return finding
}

func extractContractODRLPolicies(contract map[string]any) []map[string]any {
	raw := topLevelValue(documentData(contract), "policies")
	items, ok := asArray(raw)
	if !ok {
		return nil
	}
	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if policy, ok := item.(map[string]any); ok {
			result = append(result, policy)
		}
	}
	return result
}

func externalODRLPolicies(raw []any) []map[string]any {
	result := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if policy, ok := item.(map[string]any); ok {
			result = append(result, policy)
		}
	}
	return result
}

type odrlFieldInfo struct {
	conditionID   string
	parameterName string
}

func buildODRLFieldIndex(contract map[string]any) map[string]odrlFieldInfo {
	index := map[string]odrlFieldInfo{}
	requirements, ok := asArray(topLevelValue(documentData(contract), "contractData"))
	if !ok {
		return index
	}
	for _, rawReq := range requirements {
		req, ok := rawReq.(map[string]any)
		if !ok {
			continue
		}
		conditionID, _ := req["dcs:conditionId"].(string)
		if conditionID == "" {
			conditionID, _ = req["conditionId"].(string)
		}
		rawFields := req["dcs:fields"]
		if rawFields == nil {
			rawFields = req["fields"]
		}
		fields, ok := asArray(rawFields)
		if !ok {
			continue
		}
		for _, rawField := range fields {
			field, ok := rawField.(map[string]any)
			if !ok {
				continue
			}
			fieldID, _ := field["@id"].(string)
			paramName, _ := field["dcs:parameterName"].(string)
			if paramName == "" {
				paramName, _ = field["parameterName"].(string)
			}
			if fieldID != "" {
				index[fieldID] = odrlFieldInfo{conditionID: conditionID, parameterName: paramName}
			}
		}
	}
	return index
}

func lookupSemanticConditionValue(contract map[string]any, conditionID, parameterName string) (any, bool) {
	values, ok := asArray(contract["semanticConditionValues"])
	if !ok {
		return nil, false
	}
	for _, rawVal := range values {
		val, ok := rawVal.(map[string]any)
		if !ok {
			continue
		}
		if val["conditionId"] == conditionID && val["parameterName"] == parameterName {
			pv := val["parameterValue"]
			if pv == nil {
				return nil, false
			}
			return compactJSONLDValue(pv), true
		}
	}
	return nil, false
}

func auditContractODRLPolicies(contract map[string]any, policies []map[string]any) []PolicyFinding {
	if len(policies) == 0 {
		return nil
	}
	fieldIndex := buildODRLFieldIndex(contract)
	findings := []PolicyFinding{}
	for _, policy := range policies {
		findings = append(findings, auditODRLPolicy(contract, policy, fieldIndex)...)
	}
	return findings
}

func auditODRLPolicy(contract map[string]any, policy map[string]any, fieldIndex map[string]odrlFieldInfo) []PolicyFinding {
	ruleID, _ := policy["@id"].(string)
	if ruleID == "" {
		ruleID = "FACIS-CONTRACT-ODRL-POLICY"
	}
	policyType, _ := policy["@type"].(string)

	constraint, ok := policy["odrl:constraint"].(map[string]any)
	if !ok {
		return nil
	}
	leftOperandObj, ok := constraint["odrl:leftOperand"].(map[string]any)
	if !ok {
		return nil
	}
	fieldID, _ := leftOperandObj["@id"].(string)
	if fieldID == "" {
		return nil
	}
	operatorObj, ok := constraint["odrl:operator"].(map[string]any)
	if !ok {
		return nil
	}
	operator, _ := operatorObj["@id"].(string)
	if operator == "" {
		return nil
	}
	rightOperand := constraint["odrl:rightOperand"]

	fieldInfo, ok := fieldIndex[fieldID]
	if !ok {
		finding := contractFinding(ruleID, ruleID, "error", fmt.Sprintf("ODRL policy %q references nonexistent contract data field %q", ruleID, fieldID), fieldID, fieldID, "dcs:RequirementField")
		applyODRLPolicyDetails(&finding, fieldID, operator, nil, false, rightOperand)
		return []PolicyFinding{finding}
	}
	actualValue, hasValue := lookupSemanticConditionValue(contract, fieldInfo.conditionID, fieldInfo.parameterName)

	isProhibition := compactTerm(policyType) == "Prohibition"
	isPermission := compactTerm(policyType) == "Permission"
	severity := "error"
	if isPermission {
		severity = "info"
	}

	if !hasValue {
		if isProhibition || isPermission {
			finding := contractFinding(ruleID, ruleID, "info", fmt.Sprintf("ODRL policy %q: value not present", ruleID), fieldID, fieldID, "")
			applyODRLPolicyDetails(&finding, fieldID, operator, nil, false, rightOperand)
			return []PolicyFinding{finding}
		}
		finding := contractFinding(ruleID, ruleID, severity, fmt.Sprintf("ODRL policy %q: required value not provided", ruleID), fieldID, fieldID, "")
		applyODRLPolicyDetails(&finding, fieldID, operator, nil, false, rightOperand)
		return []PolicyFinding{finding}
	}

	satisfied := evaluateODRLConstraint(operator, actualValue, rightOperand)
	violated := (isProhibition && satisfied) || (!isProhibition && !isPermission && !satisfied)

	if violated {
		finding := contractFinding(ruleID, ruleID, severity, fmt.Sprintf("ODRL policy %q violated: value %v does not satisfy %s", ruleID, actualValue, compactTerm(operator)), fieldID, fieldID, "")
		applyODRLPolicyDetails(&finding, fieldID, operator, actualValue, true, rightOperand)
		return []PolicyFinding{finding}
	}
	finding := contractFinding(ruleID, ruleID, "info", fmt.Sprintf("ODRL policy %q satisfied", ruleID), fieldID, fieldID, "")
	applyODRLPolicyDetails(&finding, fieldID, operator, actualValue, true, rightOperand)
	return []PolicyFinding{finding}
}

func evaluateODRLConstraint(operator string, actualValue any, rightOperand any) bool {
	op := compactTerm(operator)
	actualValue = compactJSONLDValue(actualValue)
	rightOperand = compactJSONLDValue(rightOperand)
	switch op {
	case "eq":
		return odrlValuesEqual(actualValue, rightOperand)
	case "neq":
		return !odrlValuesEqual(actualValue, rightOperand)
	case "gt":
		f1, ok1 := toFloat(actualValue)
		f2, ok2 := toFloat(rightOperand)
		return ok1 && ok2 && f1 > f2+floatTolerance
	case "gteq":
		f1, ok1 := toFloat(actualValue)
		f2, ok2 := toFloat(rightOperand)
		return ok1 && ok2 && f1+floatTolerance >= f2
	case "lt":
		f1, ok1 := toFloat(actualValue)
		f2, ok2 := toFloat(rightOperand)
		return ok1 && ok2 && f1 < f2-floatTolerance
	case "lteq":
		f1, ok1 := toFloat(actualValue)
		f2, ok2 := toFloat(rightOperand)
		return ok1 && ok2 && f1 <= f2+floatTolerance
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

func contractFinding(ruleID, title, severity, message, path, semanticPath, ontologyTerm string) PolicyFinding {
	return PolicyFinding{
		RuleID:       ruleID,
		Title:        title,
		Severity:     severity,
		Message:      message,
		Path:         path,
		SemanticPath: semanticPath,
		OntologyTerm: ontologyTerm,
	}
}

const floatTolerance = 0.0000001

func normalizeObject(raw any) (map[string]any, error) {
	if raw == nil {
		return nil, fmt.Errorf("document is required")
	}
	if doc, ok := raw.(map[string]any); ok {
		return doc, nil
	}
	if bytes, ok := raw.([]byte); ok {
		var doc map[string]any
		if err := json.Unmarshal(bytes, &doc); err != nil {
			return nil, err
		}
		return doc, nil
	}
	if text, ok := raw.(string); ok {
		var doc map[string]any
		if err := json.Unmarshal([]byte(text), &doc); err != nil {
			return nil, err
		}
		return doc, nil
	}
	bytes, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(bytes, &doc); err != nil {
		return nil, err
	}
	return doc, nil
}

func contractValue(contract map[string]any, semanticPath string) (any, bool) {
	if value, ok := contractSHACLAliasValue(contract, semanticPath); ok {
		return compactJSONLDValue(value), true
	}
	if value, ok := nestedValue(contract, strings.Split(semanticPath, ".")); ok {
		return compactJSONLDValue(value), true
	}
	if value, ok := semanticConditionValuesByParameterName(contract, semanticPath); ok {
		return compactJSONLDValue(value), true
	}
	if value, ok := recursiveExactKeyValue(contract, semanticPath); ok {
		return compactJSONLDValue(value), true
	}
	if value, ok := recursiveSemanticPathValue(contract, semanticPath); ok {
		return compactJSONLDValue(value), true
	}
	return nil, false
}

func contractSHACLAliasValue(contract map[string]any, semanticPath string) (any, bool) {
	switch compactTerm(semanticPath) {
	case "did":
		return firstExistingValue(contract, "@id", "did", "dcs:did")
	case "party":
		if value, ok := firstExistingValue(contract, "party", "dcs:party", "parties"); ok {
			return value, true
		}
		return companyPartiesFromSemanticValues(contract)
	}
	return nil, false
}

func firstExistingValue(contract map[string]any, keys ...string) (any, bool) {
	for _, key := range keys {
		if value, ok := contract[key]; ok {
			return value, true
		}
	}
	return nil, false
}

func semanticConditionValuesByParameterName(contract map[string]any, semanticPath string) (any, bool) {
	values, ok := asArray(contract["semanticConditionValues"])
	if !ok {
		return nil, false
	}
	matches := []any{}
	for _, rawValue := range values {
		value, ok := rawValue.(map[string]any)
		if !ok {
			continue
		}
		parameterName, _ := value["parameterName"].(string)
		if !equivalentSemanticPath(parameterName, semanticPath) {
			continue
		}
		parameterValue, exists := value["parameterValue"]
		if exists && !isEmptyAuditValue(parameterValue) {
			matches = append(matches, parameterValue)
		}
	}
	if len(matches) == 0 {
		return nil, false
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return matches, true
}

func companyPartiesFromSemanticValues(contract map[string]any) ([]any, bool) {
	values, ok := asArray(contract["semanticConditionValues"])
	if !ok {
		return nil, false
	}
	requirements := contractDataRequirementsByConditionID(contract)
	partiesByCondition := map[string]map[string]any{}
	order := []string{}
	for _, rawValue := range values {
		value, ok := rawValue.(map[string]any)
		if !ok {
			continue
		}
		conditionID, _ := value["conditionId"].(string)
		parameterName, _ := value["parameterName"].(string)
		if !strings.HasPrefix(parameterName, "company.") {
			continue
		}
		requirement := requirements[conditionID]
		if !isCompanyPartyRequirement(requirement) && parameterName != "company.role" {
			continue
		}
		party := partiesByCondition[conditionID]
		if party == nil {
			party = map[string]any{
				"@type": "dcs:CompanyParty",
			}
			if role := companyPartyRole(requirement); role != "" {
				party["role"] = role
			}
			partiesByCondition[conditionID] = party
			order = append(order, conditionID)
		}
		parameterValue := compactJSONLDValue(value["parameterValue"])
		switch parameterName {
		case "company.legalName":
			party["legalName"] = parameterValue
			party["dcs:legalName"] = parameterValue
		case "company.role":
			if text, ok := parameterValue.(string); ok {
				party["role"] = compactEntityRole(text)
			}
		}
	}
	if len(order) == 0 {
		return nil, false
	}
	parties := make([]any, 0, len(order))
	for _, conditionID := range order {
		parties = append(parties, partiesByCondition[conditionID])
	}
	return parties, true
}

func contractDataRequirementsByConditionID(contract map[string]any) map[string]map[string]any {
	requirements := map[string]map[string]any{}
	collectContractDataRequirements(contract, requirements)
	return requirements
}

func collectContractDataRequirements(current any, requirements map[string]map[string]any) {
	switch value := current.(type) {
	case map[string]any:
		if rawRequirements, ok := topLevelValue(documentData(value), "contractData").([]any); ok {
			for _, rawRequirement := range rawRequirements {
				requirement, ok := rawRequirement.(map[string]any)
				if !ok {
					continue
				}
				conditionID, _ := requirement["dcs:conditionId"].(string)
				if conditionID == "" {
					conditionID, _ = requirement["conditionId"].(string)
				}
				if conditionID != "" {
					requirements[conditionID] = requirement
				}
			}
		}
		for _, nested := range value {
			collectContractDataRequirements(nested, requirements)
		}
	case []any:
		for _, nested := range value {
			collectContractDataRequirements(nested, requirements)
		}
	}
}

func isCompanyPartyRequirement(requirement map[string]any) bool {
	entityType, _ := requirement["dcs:entityType"].(string)
	if entityType == "" {
		entityType, _ = requirement["entityType"].(string)
	}
	return compactTerm(entityType) == "CompanyParty"
}

func companyPartyRole(requirement map[string]any) string {
	role, _ := requirement["dcs:entityRole"].(string)
	if role == "" {
		role, _ = requirement["entityRole"].(string)
	}
	return compactEntityRole(role)
}

func valuesAtPath(contract map[string]any, semanticPath string) []any {
	value, ok := contractValue(contract, semanticPath)
	if !ok {
		return nil
	}
	if values, ok := value.([]any); ok {
		result := make([]any, 0, len(values))
		for _, item := range values {
			result = append(result, compactJSONLDValue(item))
		}
		return result
	}
	return []any{compactJSONLDValue(value)}
}

func contractString(contract map[string]any, semanticPath string) (string, bool) {
	value, ok := contractValue(contract, semanticPath)
	if !ok {
		return "", false
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return "", false
	}
	return text, true
}

func nestedValue(current any, parts []string) (any, bool) {
	if len(parts) == 0 {
		return current, true
	}
	obj, ok := current.(map[string]any)
	if !ok {
		return nil, false
	}
	part := parts[0]
	candidates := []string{part, compactTerm(part)}
	for _, candidate := range candidates {
		if value, ok := obj[candidate]; ok {
			return nestedValue(value, parts[1:])
		}
	}
	return nil, false
}

func recursiveExactKeyValue(current any, key string) (any, bool) {
	switch value := current.(type) {
	case map[string]any:
		for candidateKey, candidateValue := range value {
			if candidateKey == key || compactTerm(candidateKey) == key {
				return candidateValue, true
			}
		}
		for _, candidateValue := range value {
			if found, ok := recursiveExactKeyValue(candidateValue, key); ok {
				return found, true
			}
		}
	case []any:
		for _, item := range value {
			if found, ok := recursiveExactKeyValue(item, key); ok {
				return found, true
			}
		}
	}
	return nil, false
}

func recursiveSemanticPathValue(current any, semanticPath string) (any, bool) {
	switch value := current.(type) {
	case map[string]any:
		pathValue, _ := value["semanticPath"].(string)
		if pathValue == "" {
			pathValue, _ = value["dcs:semanticPath"].(string)
		}
		if equivalentSemanticPath(pathValue, semanticPath) {
			for key, found := range value {
				switch compactTerm(key) {
				case "value", "hasTargetValue", "targetValue", "actualValue", "hasActualValue":
					return found, true
				}
			}
			for key, threshold := range value {
				if compactTerm(key) == "hasThreshold" {
					if found, ok := recursiveExactKeyValue(threshold, "hasTargetValue"); ok {
						return found, true
					}
				}
			}
		}
		for _, candidateValue := range value {
			if found, ok := recursiveSemanticPathValue(candidateValue, semanticPath); ok {
				return found, true
			}
		}
	case []any:
		for _, item := range value {
			if found, ok := recursiveSemanticPathValue(item, semanticPath); ok {
				return found, true
			}
		}
	}
	return nil, false
}

func compactJSONLDValue(value any) any {
	if obj, ok := value.(map[string]any); ok {
		for _, key := range []string{"@value", "value", "schema:value"} {
			if nested, ok := obj[key]; ok {
				return compactJSONLDValue(nested)
			}
		}
	}
	return value
}

func validIRIOrURN(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return false
	}
	return strings.Contains(trimmed, "://") || strings.HasPrefix(strings.ToLower(trimmed), "urn:")
}

func jsonLDTypeMatches(values []any, targetClass string) bool {
	target := compactTerm(strings.TrimSpace(targetClass))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			continue
		}
		if text == targetClass || compactTerm(text) == target {
			return true
		}
	}
	return false
}

func nonEmptyValues(values []any) []any {
	result := make([]any, 0, len(values))
	for _, value := range values {
		if !isEmptyAuditValue(compactJSONLDValue(value)) {
			result = append(result, value)
		}
	}
	return result
}

func valueConformsDatatype(value any, datatype string) bool {
	value = compactJSONLDValue(value)
	normalized := strings.ToLower(compactTerm(datatype))
	switch normalized {
	case "string":
		_, ok := value.(string)
		return ok
	case "integer", "int", "long":
		float, ok := toFloat(value)
		return ok && math.Trunc(float) == float
	case "decimal", "double", "float":
		_, ok := toFloat(value)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "anyuri", "uri":
		text, ok := value.(string)
		return ok && validIRIOrURN(text)
	default:
		return true
	}
}

func valueHasClass(value any, class string) bool {
	obj, ok := value.(map[string]any)
	if !ok {
		return false
	}
	return jsonLDTypeMatches(valuesAtObjectPath(obj, "@type"), class)
}

func valuesAtObjectPath(obj map[string]any, semanticPath string) []any {
	value, ok := contractValue(obj, semanticPath)
	if !ok {
		return nil
	}
	if values, ok := value.([]any); ok {
		return values
	}
	return []any{value}
}

func propertyLabel(property ContractSHACLProperty) string {
	if strings.TrimSpace(property.Name) != "" {
		return property.Name
	}
	return property.Path
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
		var parsed float64
		_, err := fmt.Sscanf(strings.TrimSpace(typed), "%f", &parsed)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func normalizedSet(values []string) map[string]bool {
	set := map[string]bool{}
	for _, value := range values {
		normalized := strings.ToUpper(strings.TrimSpace(value))
		if normalized != "" {
			set[normalized] = true
		}
	}
	return set
}

func signatureLevelSatisfies(actual string, required string) bool {
	rank := map[string]int{"SES": 1, "AES": 2, "QES": 3}
	actualRank := rank[strings.ToUpper(strings.TrimSpace(actual))]
	requiredRank := rank[strings.ToUpper(strings.TrimSpace(required))]
	return actualRank >= requiredRank && requiredRank > 0
}

func compactTerm(value string) string {
	if index := strings.LastIndex(value, ":"); index >= 0 && index < len(value)-1 {
		return value[index+1:]
	}
	return value
}

func isEmptyAuditValue(value any) bool {
	switch typed := value.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(typed) == ""
	case []any:
		return len(typed) == 0
	default:
		return false
	}
}
