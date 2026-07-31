package validation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ContractContentAuditMetadata struct {
	ContractDID     string
	ContractVersion string
	PolicyVersion   string
	AuditedBy       string
	HolderDID       string
}

type ContractContentPolicy struct {
	PolicySetID string `json:"policySetId"`
	Version     string `json:"version"`
	Policies    []any  `json:"dcs:policies"`
	// EnforceCanonicalShapes/EnforceValidationProfile opt a given audit call
	// into the Semantic Hub's canonical SHACL shapes / SLA validation
	// profile (the default disk policy document sets both; ad-hoc/test
	// policies that want to exercise only ODRL evaluation leave them unset).
	// "Enforce" here means "include in this audit's findings", not "block":
	// AuditContractContent is only reached from the read-only audit-trail
	// query. The blocking SHACL gate is RequireHubConformance, elsewhere.
	// The hub is the only source for their content — there is no
	// alternative/inline shape format anymore (ADR-8, ADR-9).
	EnforceCanonicalShapes   bool `json:"enforceCanonicalShapes"`
	EnforceValidationProfile bool `json:"enforceValidationProfile"`
	profiles                 []ValidationProfile
	// ShapesVersion/ProfileVersion record which hub version this audit ran
	// against (the pinned version for revalidation, or the currently-active
	// one for newly produced documents) — ADR-8.
	ShapesVersion  int `json:"-"`
	ProfileVersion int `json:"-"`
}

// AuditContractContent checks a produced contract document against its
// governing policies: the Semantic Hub's SHACL shapes (goRDFlib, ADR-9,
// version-pinned per ADR-8), the SLA validation profile, and the contract's
// own embedded ODRL policies.
func AuditContractContent(ctx context.Context, contractDocument any, policyDocument any, metadata ContractContentAuditMetadata) ([]PolicyFinding, error) {
	contract, err := normalizeObject(contractDocument)
	if err != nil {
		return nil, fmt.Errorf("decode contract document: %w", err)
	}
	policy, err := normalizeContractContentPolicy(ctx, policyDocument, metadata)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(metadata.PolicyVersion) != "" {
		policy.Version = metadata.PolicyVersion
	}

	source, err := requireShapeSource()
	if err != nil {
		return nil, err
	}
	// This load is a hub-liveness probe, nothing more: it hard-fails the audit
	// when the hub cannot serve the domain-field ontology. The parsed ontology
	// is NOT consulted on this path. (An earlier comment here named a value-
	// normalization function `compactEntityRole` as the reason; no such
	// function exists anywhere in the repository, and the value normalization
	// it implies was never written — which is why facis.sla.basic.v1.yaml has
	// to list both the bare `provider` and the full taxonomy IRI in one
	// `values` list.)
	if _, err := requireDomainOntology(ctx); err != nil {
		return nil, err
	}
	if policy.EnforceValidationProfile {
		pinnedProfile := pinnedHubProfileVersion(contract)
		if pinnedProfile <= 0 {
			if _, hasBundle := contract["dcs:effectiveShapes"]; hasBundle {
				return nil, errors.New("immutable semantic bundle is missing dcterms:conformsTo")
			}
		} else {
			bundleSource, ok := source.(EffectiveBundleShapeSource)
			if !ok {
				return nil, errors.New("shape source cannot resolve immutable validation profile")
			}
			profileContent, loadErr := bundleSource.ProfileAt(ctx, pinnedProfile)
			if loadErr != nil {
				return nil, fmt.Errorf("load pinned validation profile v%d: %w", pinnedProfile, loadErr)
			}
			hubProfile, parseErr := LoadValidationProfileYAML([]byte(profileContent))
			if parseErr != nil {
				return nil, fmt.Errorf("parse pinned validation profile v%d: %w", pinnedProfile, parseErr)
			}
			policy.profiles = []ValidationProfile{hubProfile}
			policy.ProfileVersion = pinnedProfile
		}
	}

	findings := []PolicyFinding{}
	if policy.EnforceCanonicalShapes {
		shaclFindings, shapesVersion, err := validateAgainstShapeSource(ctx, contract, source)
		if err != nil {
			return nil, fmt.Errorf("SHACL validation: %w", err)
		}
		policy.ShapesVersion = shapesVersion
		findings = append(findings, shaclFindings...)
	}
	root, err := expandForAudit(ctx, contract, source)
	if err != nil {
		return nil, fmt.Errorf("JSON-LD expansion: %w", err)
	}

	for _, profile := range policy.profiles {
		findings = append(findings, auditContractValidationProfile(contract, root, profile)...)
	}
	embeddedFindings, err := auditExpandedODRLPolicies(ctx, root, expandedODRLPolicyRules(root))
	if err != nil {
		return nil, err
	}
	findings = append(findings, embeddedFindings...)
	externalRules, err := expandExternalODRLRules(ctx, externalODRLPolicies(policy.Policies), source)
	if err != nil {
		return nil, err
	}
	externalFindings, err := auditExpandedODRLPolicies(ctx, root, externalRules)
	if err != nil {
		return nil, err
	}
	findings = append(findings, externalFindings...)

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
// embedded as dcs:policies against the values carried inline on the
// requirement fields.
func ValidateContractPolicySatisfaction(contractDocument any, metadata ContractContentAuditMetadata) error {
	contract, err := normalizeObject(contractDocument)
	if err != nil {
		return fmt.Errorf("decode contract document: %w", err)
	}
	source, err := requireShapeSource()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	root, err := expandForAudit(ctx, contract, source)
	if err != nil {
		return fmt.Errorf("ODRL evaluation: %w", err)
	}
	findings, err := auditExpandedODRLPolicies(ctx, root, expandedODRLPolicyRules(root))
	if err != nil {
		return fmt.Errorf("ODRL evaluation: %w", err)
	}
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

func normalizeContractContentPolicy(ctx context.Context, raw any, metadata ContractContentAuditMetadata) (ContractContentPolicy, error) {
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

	// At this normalization stage the contract is not yet in scope, so load the
	// hub's active profile as the provisional policy. AuditContractContent
	// replaces it with the document's pinned ProfileAt version when an immutable
	// effective bundle is present. There is no disk or executor-side fallback.
	if policy.EnforceValidationProfile {
		profileContent, profileVersion, err := activeShapeSource.ActiveProfile(ctx)
		if err != nil {
			return ContractContentPolicy{}, fmt.Errorf("load validation profile: %w", err)
		}
		hubProfile, err := LoadValidationProfileYAML([]byte(profileContent))
		if err != nil {
			return ContractContentPolicy{}, fmt.Errorf("parse validation profile (hub version %d): %w", profileVersion, err)
		}
		policy.profiles = append(policy.profiles, hubProfile)
		policy.ProfileVersion = profileVersion
	}

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

func auditContractValidationProfile(contract map[string]any, root map[string]any, profile ValidationProfile) []PolicyFinding {
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
		findings = append(findings, auditContractStatementValidationRules(root, ValidationProfile{
			ID:          profile.ID,
			Version:     profile.Version,
			Description: profile.Description,
			AppliesTo:   profile.AppliesTo,
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

func auditContractStatementValidationRules(root map[string]any, profile ValidationProfile) []PolicyFinding {
	statements := expandedStatements(root)
	if len(statements) == 0 || !statementsCoverProfile(statements, profile.AppliesTo) {
		return nil
	}
	issues := ValidateContractStatements(statements, profile)
	findings := make([]PolicyFinding, 0, len(issues))
	for _, issue := range issues {
		findings = append(findings, contractFinding(issue.RuleID, issue.RuleID, issue.Severity, issue.Message, issue.StatementID, "dcs:ContractStatement"))
	}
	return findings
}

func validationRuleFinding(rule ValidationRule, path string, severity string, message string) PolicyFinding {
	return validationRuleFindingWithDetails(rule, path, severity, message, nil, nil, nil, "")
}

func validationRuleFindingWithDetails(rule ValidationRule, path string, severity string, message string, actualValue any, expectedValue any, expectedValues []any, operator string) PolicyFinding {
	finding := contractFinding(rule.ID, rule.ID, severity, message, path, "")
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

// normalizePolicyOperator maps an operator term to its internal evaluation
// name. Accepted vocabulary: the ODRL core constraint operators
// (odrl:eq/neq/gt/lt/gteq/lteq/isAnyOf/isNoneOf, compacted or full IRI) and
// the validation-profile rule operators; anything else passes through
// verbatim and fails evaluation.
func normalizePolicyOperator(operator string) string {
	switch strings.ToLower(strings.TrimSpace(compactTerm(operator))) {
	case "":
		return ""
	case "gte", "gteq":
		return "gte"
	case "lte", "lteq":
		return "lte"
	case "gt":
		return "gt"
	case "lt":
		return "lt"
	case "eq":
		return "eq"
	case "neq":
		return "neq"
	case "isanyof", "in":
		return "in"
	case "isnoneof", "notin":
		return "notIn"
	case "contains":
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
	case "exists":
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
	label    string
	value    any
	hasValue bool
	// units are the distinct odrl:unit IRIs the document's constraints
	// denominate this field's boundaries in (applyDeclaredUnits).
	units []string
}

func contractFinding(ruleID, title, severity, message, path, ontologyTerm string) PolicyFinding {
	return PolicyFinding{
		RuleID:       ruleID,
		Title:        title,
		Severity:     severity,
		Message:      message,
		Path:         path,
		OntologyTerm: ontologyTerm,
	}
}

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

// contractValue resolves a validation-profile rule target (a document
// property path like "contract.jurisdiction", or a field dcs:label) to
// the document's runtime value.
func contractValue(contract map[string]any, target string) (any, bool) {
	if value, ok := contractSHACLAliasValue(contract, target); ok {
		return compactJSONLDValue(value), true
	}
	if value, ok := nestedValue(contract, strings.Split(target, ".")); ok {
		return compactJSONLDValue(value), true
	}
	if value, ok := semanticConditionValuesByParameterName(contract, target); ok {
		return compactJSONLDValue(value), true
	}
	if value, ok := recursiveExactKeyValue(contract, target); ok {
		return compactJSONLDValue(value), true
	}
	return nil, false
}

func contractSHACLAliasValue(contract map[string]any, target string) (any, bool) {
	switch compactTerm(target) {
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

// contractDataFieldValue pairs a declared ContractField with its inline
// dcs:value; the field @id is the IRI an ODRL
// constraint's odrl:leftOperand references, and dcs:label is its human name.
type contractDataFieldValue struct {
	label    string
	value    any
	hasValue bool
}

// contractDataFieldValues walks the document's declared ContractFields in
// document order (including composed sub-templates'), yielding each once with
// the value it carries inline.
func contractDataFieldValues(contract map[string]any) []contractDataFieldValue {
	out := []contractDataFieldValue{}
	seen := map[string]bool{}
	var walk func(current any)
	walk = func(current any) {
		switch value := current.(type) {
		case map[string]any:
			if rawFields, ok := topLevelValue(documentData(value), "contractFields").([]any); ok {
				for _, rawField := range rawFields {
					field, ok := rawField.(map[string]any)
					if !ok {
						continue
					}
					fieldID, _ := field["@id"].(string)
					if fieldID == "" || seen[fieldID] {
						continue
					}
					seen[fieldID] = true
					fieldValue, hasValue := inlineFieldValue(field)
					label, _ := firstOf(field, "dcs:label", "label").(string)
					out = append(out, contractDataFieldValue{
						label:    label,
						value:    fieldValue,
						hasValue: hasValue,
					})
				}
			}
			for _, nested := range value {
				walk(nested)
			}
		case []any:
			for _, nested := range value {
				walk(nested)
			}
		}
	}
	walk(contract)
	return out
}

// inlineFieldValue reads the value a field carries inline (dcs:value),
// treating an empty value as absent.
func inlineFieldValue(field map[string]any) (any, bool) {
	for _, key := range []string{"dcs:value", "value"} {
		value, ok := field[key]
		if !ok {
			continue
		}
		if isEmptyAuditValue(value) {
			return nil, false
		}
		return value, true
	}
	return nil, false
}

func semanticConditionValuesByParameterName(contract map[string]any, parameterName string) (any, bool) {
	matches := []any{}
	for _, field := range contractDataFieldValues(contract) {
		if !strings.EqualFold(field.label, parameterName) || !field.hasValue {
			continue
		}
		matches = append(matches, field.value)
	}
	if len(matches) == 0 {
		return nil, false
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return matches, true
}

// companyPartiesFromSemanticValues no longer derives parties from filled
// contractFields: the flat, self-contained field registry carries no
// entity grouping, and a contract's parties are materialized from its ODRL
// rules (materializeRuleParties) and stored in dcs:parties.
func companyPartiesFromSemanticValues(_ map[string]any) ([]any, bool) {
	return nil, false
}

func firstOf(node map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := node[key]; ok {
			return value
		}
	}
	return nil
}

func contractString(contract map[string]any, target string) (string, bool) {
	value, ok := contractValue(contract, target)
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
