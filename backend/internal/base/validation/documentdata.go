package validation

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"digital-contracting-service/internal/base/datatype"
)

var jsonLDContextIRI string

func SetJSONLDContextIRI(iri string) {
	jsonLDContextIRI = iri
}

type domainField struct {
	SchemaRef      string
	Type           string
	DomainPath     string
	OntologyTerm   string
	StatementField string
	StatementType  string
	StatementID    string
	ValuePrefix    string
	Constraint     *valueConstraint
}

type valueConstraint struct {
	Format           string
	Pattern          string
	ValueType        string
	AllowedValues    []string
	ValueOptions     []valueOption
	AllowedValuesRef string
	Min              *float64
	Max              *float64
	Description      string
}

type valueOption struct {
	Value  string
	Label  string
	Symbol string
	IRI    string
}

func (constraint *valueConstraint) asMap() map[string]any {
	result := map[string]any{}
	if constraint.Format != "" {
		result["format"] = constraint.Format
	}
	if constraint.Pattern != "" {
		result["pattern"] = constraint.Pattern
	}
	if constraint.ValueType != "" {
		result["valueType"] = constraint.ValueType
	}
	if allowedValues := allowedValuesForConstraint(constraint); len(allowedValues) > 0 {
		values := make([]any, len(allowedValues))
		for i, value := range allowedValues {
			values[i] = value
		}
		result["allowedValues"] = values
	}
	if valueOptions := valueOptionsForConstraint(constraint); len(valueOptions) > 0 {
		options := make([]any, 0, len(valueOptions))
		for _, option := range valueOptions {
			if option.Value == "" {
				continue
			}
			item := map[string]any{"value": option.Value}
			if option.Label != "" {
				item["label"] = option.Label
			}
			if option.Symbol != "" {
				item["symbol"] = option.Symbol
			}
			if option.IRI != "" {
				item["iri"] = option.IRI
			}
			options = append(options, item)
		}
		if len(options) > 0 {
			result["valueOptions"] = options
		}
	}
	if constraint.AllowedValuesRef != "" {
		result["allowedValuesRef"] = constraint.AllowedValuesRef
	}
	if constraint.Min != nil {
		result["min"] = *constraint.Min
	}
	if constraint.Max != nil {
		result["max"] = *constraint.Max
	}
	if constraint.Description != "" {
		result["description"] = constraint.Description
	}
	return result
}

type documentData map[string]any

// NormalizeTemplateData validates and normalizes template JSON-LD data.
func NormalizeTemplateData(raw *datatype.JSON) (*datatype.JSON, error) {
	data, err := decodeDocumentData(raw)
	if err != nil {
		return nil, err
	}
	if !isCanonicalEnvelope(data) {
		return nil, errors.New("template data must use the canonical dcs:documentStructure envelope")
	}
	normalizeCanonicalEnvelope(data, "dcs:ContractTemplate")
	if err := validateCanonicalEnvelope(data); err != nil {
		return nil, err
	}
	return encodeDocumentData(data)
}

// NormalizeTemplateDataForPersistence keeps stored template JSON-LD
// self-identifying when it is read outside the relational row envelope.
func NormalizeTemplateDataForPersistence(raw *datatype.JSON, did string) (*datatype.JSON, error) {
	normalized, err := NormalizeTemplateData(raw)
	if err != nil {
		return nil, err
	}
	return addDocumentIdentity(normalized, did)
}

// NormalizeContractData adds FACIS contract schema and policy references and
// validates the contract JSON-LD structure.
func NormalizeContractData(raw *datatype.JSON, requireSemanticValues bool) (*datatype.JSON, error) {
	data, err := decodeDocumentData(raw)
	if err != nil {
		return nil, err
	}
	if isCanonicalEnvelope(data) {
		normalizeCanonicalEnvelope(data, "dcs:Contract")
		if err := validateCanonicalEnvelope(data); err != nil {
			return nil, err
		}
		return encodeDocumentData(data)
	}
	normalizeContractMetadata(data)
	if err := normalizeSemanticConditions(data); err != nil {
		return nil, err
	}
	if err := validateSchemaRefs(data, false); err != nil {
		return nil, err
	}
	if err := validatePolicyRefs(data, false); err != nil {
		return nil, err
	}
	normalizeContractSemanticRuntime(data)
	if err := validateSemanticRules(data); err != nil {
		return nil, err
	}
	if err := validateContractSemanticsData(data, requireSemanticValues); err != nil {
		return nil, err
	}
	if err := validateRoleEntities(data); err != nil {
		return nil, err
	}
	return encodeDocumentData(data)
}

// NormalizeContractDataForPersistence keeps stored contract JSON-LD
// self-identifying when it is read outside the relational row envelope.
func NormalizeContractDataForPersistence(raw *datatype.JSON, did string, requireSemanticValues bool) (*datatype.JSON, error) {
	normalized, err := NormalizeContractData(raw, requireSemanticValues)
	if err != nil {
		return nil, err
	}
	return addDocumentIdentity(normalized, did)
}

// BuildContractStatements returns machine-readable contract statements from the
// canonical contract content graph.
func BuildContractStatements(raw *datatype.JSON) ([]map[string]any, error) {
	data, err := decodeDocumentData(raw)
	if err != nil {
		return nil, err
	}
	return buildContractStatements(data)
}

// ValidateContractSemantics validates placeholders, bindings, derived
// contractStatements, and generated contract semantic rules.
func ValidateContractSemantics(raw *datatype.JSON) error {
	data, err := decodeDocumentData(raw)
	if err != nil {
		return err
	}
	if isCanonicalEnvelope(data) {
		return validateCanonicalEnvelope(data)
	}
	normalizeContractSemanticRuntime(data)
	return validateContractSemanticsData(data, true)
}

func addDocumentIdentity(raw *datatype.JSON, did string) (*datatype.JSON, error) {
	data, err := decodeDocumentData(raw)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(did) != "" {
		previousID, _ := data["@id"].(string)
		rebaseDocumentIDs(map[string]any(data), previousID, did)
		data["@id"] = did
		if metadata, ok := topLevelValue(data, "metadata").(map[string]any); ok {
			metadata["@id"] = did + "#metadata"
		}
		if structure, ok := topLevelValue(data, "documentStructure").(map[string]any); ok {
			structure["@id"] = did + "#document-structure"
		}
	}
	return encodeDocumentData(data)
}

func rebaseDocumentIDs(value any, previousID string, did string) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if text, ok := nested.(string); ok {
				switch {
				case strings.HasPrefix(text, "urn:uuid:"):
					typed[key] = did + "#" + strings.TrimPrefix(text, "urn:uuid:")
					continue
				case previousID != "" && previousID != did && strings.HasPrefix(text, previousID+"#"):
					typed[key] = did + strings.TrimPrefix(text, previousID)
					continue
				}
			}
			rebaseDocumentIDs(nested, previousID, did)
		}
	case []any:
		for _, nested := range typed {
			rebaseDocumentIDs(nested, previousID, did)
		}
	}
}

func decodeDocumentData(raw *datatype.JSON) (documentData, error) {
	if raw == nil || !raw.IsNotNullValue() {
		return documentData{}, nil
	}
	var data map[string]any
	if err := json.Unmarshal(*raw, &data); err != nil {
		return nil, fmt.Errorf("document data is not valid JSON: %w", err)
	}
	if data == nil {
		data = map[string]any{}
	}
	return data, nil
}

func encodeDocumentData(data documentData) (*datatype.JSON, error) {
	normalized, err := datatype.NewJSON(map[string]any(data))
	if err != nil {
		return nil, err
	}
	return &normalized, nil
}

func isCanonicalEnvelope(data documentData) bool {
	_, hasPrefixedDocumentStructure := data["dcs:documentStructure"]
	_, hasDocumentStructure := data["documentStructure"]
	return hasPrefixedDocumentStructure || hasDocumentStructure
}

func normalizeCanonicalEnvelope(data documentData, documentType string) {
	normalizeCanonicalContext(data)
	if rawType, _ := data["@type"].(string); strings.TrimSpace(rawType) == "" {
		data["@type"] = documentType
	}
	if _, ok := topLevelValue(data, "contractData").([]any); !ok {
		if _, exists := topLevelValueExists(data, "contractData"); !exists {
			setTopLevelValue(data, "dcs:contractData", []any{})
		}
	}
	if _, ok := topLevelValue(data, "policies").([]any); !ok {
		if _, exists := topLevelValueExists(data, "policies"); !exists {
			setTopLevelValue(data, "dcs:policies", []any{})
		}
	}
}

func normalizeCanonicalContext(data documentData) {
	context, ok := data["@context"].(map[string]any)
	if !ok {
		data["@context"] = map[string]any{
			"dcs":  "https://w3id.org/facis/dcs/ontology/v1#",
			"odrl": "http://www.w3.org/ns/odrl/2/",
			"xsd":  "http://www.w3.org/2001/XMLSchema#",
		}
		return
	}
	if _, ok := context["dcs"]; !ok {
		context["dcs"] = "https://w3id.org/facis/dcs/ontology/v1#"
	}
	if _, ok := context["odrl"]; !ok {
		context["odrl"] = "http://www.w3.org/ns/odrl/2/"
	}
	if _, ok := context["xsd"]; !ok {
		context["xsd"] = "http://www.w3.org/2001/XMLSchema#"
	}
}

func validateCanonicalEnvelope(data documentData) error {
	documentStructure, ok := topLevelValue(data, "documentStructure").(map[string]any)
	if !ok {
		return errors.New("documentStructure must be an object")
	}
	if containsODRLTerms(documentStructure) {
		return errors.New("documentStructure must not contain ODRL policy terms")
	}
	if metadata, exists := topLevelValueExists(data, "metadata"); exists {
		if _, ok := metadata.(map[string]any); !ok {
			return errors.New("metadata must be an object")
		}
	}
	if contractData, exists := topLevelValueExists(data, "contractData"); exists {
		if _, ok := contractData.([]any); !ok {
			return errors.New("contractData must be an array")
		}
	}
	if policies, exists := topLevelValueExists(data, "policies"); exists {
		if _, ok := policies.([]any); !ok {
			return errors.New("policies must be an array")
		}
	}
	return validateCanonicalReferences(data, documentStructure)
}

func validateCanonicalReferences(data documentData, documentStructure map[string]any) error {
	blocks, ok := jsonLDList(documentStructure["dcs:blocks"])
	if !ok {
		blocks, ok = documentStructure["dcs:blocks"].([]any)
	}
	if !ok {
		return errors.New("documentStructure.dcs:blocks must be an array")
	}
	layout, ok := documentStructure["dcs:layout"].([]any)
	if !ok {
		return errors.New("documentStructure.dcs:layout must be an array")
	}

	blockIDs := map[string]bool{}
	blockTypes := map[string]string{}
	for index, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			return fmt.Errorf("documentStructure.dcs:blocks.%d must be an object", index)
		}
		id, _ := block["@id"].(string)
		if strings.TrimSpace(id) == "" {
			return fmt.Errorf("documentStructure.dcs:blocks.%d.@id is required", index)
		}
		if blockIDs[id] {
			return fmt.Errorf("duplicate document block @id %q", id)
		}
		blockIDs[id] = true
		blockTypes[id], _ = block["@type"].(string)
	}

	referencedBlocks := map[string]bool{}
	rootCount := 0
	for index, rawNode := range layout {
		node, ok := rawNode.(map[string]any)
		if !ok {
			return fmt.Errorf("documentStructure.dcs:layout.%d must be an object", index)
		}
		nodeID, _ := node["@id"].(string)
		if isTrueValue(node["dcs:isRoot"]) {
			rootCount++
		} else if !blockIDs[nodeID] {
			return fmt.Errorf("layout references nonexistent block %q", nodeID)
		}
		children, ok := jsonLDList(node["dcs:children"])
		if !ok {
			return fmt.Errorf("documentStructure.dcs:layout.%d.dcs:children must be an @list", index)
		}
		for _, rawChild := range children {
			child, ok := rawChild.(map[string]any)
			if !ok {
				return fmt.Errorf("layout child in %q must be an @id reference", nodeID)
			}
			childID, _ := child["@id"].(string)
			if !blockIDs[childID] {
				return fmt.Errorf("layout references nonexistent block %q", childID)
			}
			referencedBlocks[childID] = true
		}
	}
	if rootCount != 1 {
		return fmt.Errorf("documentStructure must contain exactly one root layout node, got %d", rootCount)
	}
	for blockID := range blockIDs {
		if !referencedBlocks[blockID] {
			if blockTypes[blockID] == "dcs:Clause" {
				continue
			}
			return fmt.Errorf("document block %q is not referenced by layout", blockID)
		}
	}

	fieldIDs, err := canonicalFieldIDs(data)
	if err != nil {
		return err
	}
	for _, rawBlock := range blocks {
		block := rawBlock.(map[string]any)
		if err := validateBlockPlaceholders(block, fieldIDs); err != nil {
			return err
		}
	}
	return validatePolicyOperands(data, fieldIDs)
}

func canonicalFieldIDs(data documentData) (map[string]bool, error) {
	contractData, _ := topLevelValue(data, "contractData").([]any)
	fieldIDs := map[string]bool{}
	for requirementIndex, rawRequirement := range contractData {
		requirement, ok := rawRequirement.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("contractData.%d must be an object", requirementIndex)
		}
		fields, ok := requirement["dcs:fields"].([]any)
		if !ok {
			return nil, fmt.Errorf("contractData.%d.dcs:fields must be an array", requirementIndex)
		}
		for fieldIndex, rawField := range fields {
			field, ok := rawField.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("contractData.%d.dcs:fields.%d must be an object", requirementIndex, fieldIndex)
			}
			id, _ := field["@id"].(string)
			if strings.TrimSpace(id) == "" {
				return nil, fmt.Errorf("contractData.%d.dcs:fields.%d.@id is required", requirementIndex, fieldIndex)
			}
			if fieldIDs[id] {
				return nil, fmt.Errorf("duplicate contract data field @id %q", id)
			}
			fieldIDs[id] = true
		}
	}
	return fieldIDs, nil
}

func validateBlockPlaceholders(block map[string]any, fieldIDs map[string]bool) error {
	content, ok := jsonLDList(block["dcs:content"])
	if !ok {
		return nil
	}
	for _, rawSegment := range content {
		segment, ok := rawSegment.(map[string]any)
		if !ok || segment["@type"] != "dcs:Placeholder" {
			continue
		}
		bindsTo, _ := segment["dcs:bindsTo"].(map[string]any)
		fieldID, _ := bindsTo["@id"].(string)
		if !fieldIDs[fieldID] {
			return fmt.Errorf("placeholder binds to nonexistent contract data field %q", fieldID)
		}
	}
	return nil
}

func validatePolicyOperands(data documentData, fieldIDs map[string]bool) error {
	policies, _ := topLevelValue(data, "policies").([]any)
	for index, rawPolicy := range policies {
		policy, ok := rawPolicy.(map[string]any)
		if !ok {
			return fmt.Errorf("policies.%d must be an object", index)
		}
		switch policy["@type"] {
		case "odrl:Duty", "odrl:Permission", "odrl:Prohibition":
		default:
			return fmt.Errorf("policies.%d has unsupported @type %q", index, policy["@type"])
		}
		constraint, ok := policy["odrl:constraint"].(map[string]any)
		if !ok {
			return fmt.Errorf("policies.%d.odrl:constraint must be an object", index)
		}
		leftOperand, _ := constraint["odrl:leftOperand"].(map[string]any)
		fieldID, _ := leftOperand["@id"].(string)
		if !fieldIDs[fieldID] {
			return fmt.Errorf("policy references nonexistent contract data field %q", fieldID)
		}
	}
	return nil
}

func jsonLDList(value any) ([]any, bool) {
	container, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}
	items, ok := container["@list"].([]any)
	return items, ok
}

func isTrueValue(value any) bool {
	result, _ := value.(bool)
	return result
}

func topLevelValue(data documentData, localName string) any {
	value, _ := topLevelValueExists(data, localName)
	return value
}

func topLevelValueExists(data documentData, localName string) (any, bool) {
	if value, ok := data["dcs:"+localName]; ok {
		return value, true
	}
	value, ok := data[localName]
	return value, ok
}

func setTopLevelValue(data documentData, key string, value any) {
	data[key] = value
}

func containsODRLTerms(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if strings.HasPrefix(key, "odrl:") {
				return true
			}
			if raw, ok := nested.(string); ok && strings.HasPrefix(raw, "odrl:") {
				return true
			}
			if containsODRLTerms(nested) {
				return true
			}
		}
	case []any:
		for _, nested := range typed {
			if containsODRLTerms(nested) {
				return true
			}
		}
	}
	return false
}

func normalizeContractMetadata(data documentData) {
	data["@context"] = jsonLDContextIRI
	data["@type"] = "Contract"
	data["schemaRefs"] = map[string]any{
		"documentStructure": SchemaDocumentStructureV1,
		"semanticCondition": SchemaSemanticConditionV1,
		"contractData":      SchemaContractDataV1,
		"jsonLdContext":     SchemaJSONLDContextV1,
		"ontology":          SchemaOntologyV1,
		"shaclShapes":       SchemaSHACLShapesV1,
	}
	data["policyRefs"] = contractPolicyRefs
	data["validation"] = map[string]any{
		"schemaVersion":     "v1",
		"profile":           "FACIS_DCS_CONTRACT_V1",
		"requiredPolicies":  []string{PolicyContractStructureV1, PolicyContractSemanticValuesV1},
		"validatedBySchema": true,
	}
	normalizeSemanticRuntimeMetadata(data)
}

func normalizeSemanticRuntimeMetadata(data documentData) {
	data["placeholderBindings"] = buildPlaceholderBindings(data)
	data["semanticRules"] = mergeSemanticRules(data["semanticRules"], buildSemanticRules(data))
	data["policyBundle"] = buildODRLPolicyBundle(data["semanticRules"])
}

func normalizeContractSemanticRuntime(data documentData) {
	statements, err := buildContractStatements(data)
	if err == nil {
		data[statementSetDocumentProperty()] = map[string]any{
			"@type":      statementSetOntologyType(),
			"statements": statementsToAny(statements),
		}
	}
	generated := buildSemanticRules(data)
	data["semanticRules"] = mergeSemanticRules(data["semanticRules"], generated)
	data["policyBundle"] = buildODRLPolicyBundle(data["semanticRules"])
}

func validateSchemaRefs(data documentData, template bool) error {
	refs, ok := data["schemaRefs"].(map[string]any)
	if !ok {
		return errors.New("schemaRefs must be an object")
	}
	required := map[string]string{
		"documentStructure": SchemaDocumentStructureV1,
		"semanticCondition": SchemaSemanticConditionV1,
	}
	if template {
		required["templateData"] = SchemaTemplateDataV1
	} else {
		required["contractData"] = SchemaContractDataV1
	}
	for key, expected := range required {
		if actual, _ := refs[key].(string); actual != expected {
			return fmt.Errorf("schemaRefs.%s must be %q", key, expected)
		}
	}
	return nil
}

func validatePolicyRefs(data documentData, template bool) error {
	policies, ok := asArray(data["policyRefs"])
	if !ok {
		return errors.New("policyRefs must be an array")
	}
	required := []string{PolicyContractStructureV1, PolicyContractSemanticValuesV1}
	if template {
		required = []string{PolicyTemplateStructureV1, PolicyTemplateSemanticConditionsV1}
	}
	seen := map[string]bool{}
	for _, item := range policies {
		policy, ok := item.(map[string]any)
		if !ok {
			return errors.New("policyRefs entries must be objects")
		}
		policyID, _ := policy["policyId"].(string)
		version, _ := policy["version"].(string)
		if strings.TrimSpace(policyID) == "" || strings.TrimSpace(version) == "" {
			return errors.New("policyRefs entries require policyId and version")
		}
		seen[policyID] = true
	}
	for _, policyID := range required {
		if !seen[policyID] {
			return fmt.Errorf("required policy %q is missing", policyID)
		}
	}
	return nil
}

func buildPlaceholderBindings(data documentData) []map[string]any {
	blocks, _ := asArray(data["documentBlocks"])
	conditions, _ := semanticConditionIndex(data)

	placeholderPattern := regexp.MustCompile(`\{\{([^}.]+)\.([^}]+)\}\}`)
	seen := map[string]bool{}
	bindings := []map[string]any{}
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok || block["type"] != "CLAUSE" {
			continue
		}
		blockID, _ := block["blockId"].(string)
		text, _ := block["text"].(string)
		for _, match := range placeholderPattern.FindAllStringSubmatch(text, -1) {
			if len(match) != 3 {
				continue
			}
			conditionID := match[1]
			parameterName := match[2]
			condition := conditions.conditionForBlock(blockID, conditionID)
			if condition == nil {
				continue
			}
			if _, found := findParameter(condition, parameterName); !found {
				continue
			}
			key := blockID + "\x00" + conditionID + "\x00" + parameterName
			if seen[key] {
				continue
			}
			seen[key] = true
			bindings = append(bindings, map[string]any{
				"@type":            "PlaceholderBinding",
				"placeholder":      "{{" + conditionID + "." + parameterName + "}}",
				"boundToCondition": conditionID,
				"boundToParameter": parameterName,
				"blockId":          blockID,
				"source":           "clause-placeholder",
			})
		}
	}
	return bindings
}

func buildSemanticRules(data documentData) []map[string]any {
	blocks, _ := asArray(data["documentBlocks"])
	conditions, _ := asArray(data["semanticConditions"])
	blockIDsByCondition := map[string][]string{}
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok || block["type"] != "CLAUSE" {
			continue
		}
		blockID, _ := block["blockId"].(string)
		refs, _ := asArray(block["conditionIds"])
		for _, rawConditionID := range refs {
			conditionID, ok := rawConditionID.(string)
			if !ok {
				continue
			}
			blockIDsByCondition[conditionID] = append(blockIDsByCondition[conditionID], blockID)
		}
	}

	rules := []map[string]any{}
	for _, item := range conditions {
		condition, ok := item.(map[string]any)
		if !ok {
			continue
		}
		conditionID, _ := condition["conditionId"].(string)
		conditionName, _ := condition["conditionName"].(string)
		if conditionName == "" {
			conditionName = conditionID
		}
		parameters, _ := asArray(condition["parameters"])
		for _, rawParam := range parameters {
			param, ok := rawParam.(map[string]any)
			if !ok {
				continue
			}
			parameterName, _ := param["parameterName"].(string)
			parameterType, _ := param["type"].(string)
			operators, _ := asArray(param["operators"])
			for _, rawOperator := range operators {
				operate, targets := parseSemanticOperator(rawOperator)
				operator := normalizeSemanticOperator(operate)
				if operator == "" {
					continue
				}
				ruleType := "SemanticRule"
				switch parameterType {
				case "date":
					ruleType = "DateConstraintRule"
				case "decimal", "integer":
					ruleType = "ThresholdRule"
				}
				var rightOperand any = targets
				if len(targets) == 1 && !isSetOperator(operator) {
					rightOperand = targets[0]
				}
				rules = append(rules, map[string]any{
					"@type":                             ruleType,
					"ruleId":                            "rule-" + slugify(conditionID) + "-" + slugify(parameterName) + "-" + slugify(operator),
					"conditionId":                       conditionID,
					"parameterName":                     parameterName,
					semanticRuleAppliesToClauseProperty: stringSliceToAny(blockIDsByCondition[conditionID]),
					"leftOperand":                       "{{" + conditionID + "." + parameterName + "}}",
					semanticRuleOperatorProperty:        operator,
					semanticRuleRightOperandProperty:    rightOperand,
					"valueType":                         parameterType,
					"severity":                          semanticRuleSeverity(param),
					"source":                            semanticRuleSourceCondition,
					"message":                           fmt.Sprintf("%s.%s must satisfy %s.", conditionName, parameterName, operator),
				})
			}
		}
	}
	return rules
}

func buildODRLPolicyBundle(rawRules any) map[string]any {
	rules, ok := asArray(rawRules)
	if !ok {
		return map[string]any{
			"@type":  "PolicyBundle",
			"format": "odrl-jsonld",
			"rules":  []any{},
		}
	}
	duties := []any{}
	for _, item := range rules {
		rule, ok := item.(map[string]any)
		if !ok {
			continue
		}
		operator, _ := rule[semanticRuleOperatorProperty].(string)
		odrlOperator := odrlOperator(operator)
		if odrlOperator == "" {
			continue
		}
		ruleID, _ := rule["ruleId"].(string)
		if ruleID == "" {
			ruleID = "semantic-rule-" + fmt.Sprint(len(duties)+1)
		}
		duties = append(duties, map[string]any{
			"@id":   ruleID + "-duty",
			"@type": "odrl:Duty",
			"odrl:constraint": []any{
				map[string]any{
					"@type":             "odrl:Constraint",
					"odrl:leftOperand":  rule["leftOperand"],
					"odrl:operator":     map[string]any{"@id": odrlOperator},
					"odrl:rightOperand": rule[semanticRuleRightOperandProperty],
				},
			},
		})
	}
	return map[string]any{
		"@type":  "PolicyBundle",
		"format": "odrl-jsonld",
		"rules":  duties,
	}
}

func odrlOperator(operator string) string {
	switch operator {
	case "Equals", "odrl:eq":
		return "odrl:eq"
	case "NotEquals", "odrl:neq":
		return "odrl:neq"
	case "In", "odrl:isAnyOf":
		return "odrl:isAnyOf"
	case "NotIn", "odrl:isNoneOf":
		return "odrl:isNoneOf"
	case "GreaterThan", "odrl:gt":
		return "odrl:gt"
	case "GreaterThanOrEqual", "odrl:gteq":
		return "odrl:gteq"
	case "LessThan", "odrl:lt":
		return "odrl:lt"
	case "LessThanOrEqual", "odrl:lteq":
		return "odrl:lteq"
	case "Contains", "odrl:hasPart":
		return "odrl:hasPart"
	default:
		return ""
	}
}

func isSetOperator(operator string) bool {
	return operator == "In" || operator == "NotIn" || operator == "odrl:isAnyOf" || operator == "odrl:isNoneOf"
}

func mergeSemanticRules(rawExisting any, generated []map[string]any) []any {
	result := []any{}
	seen := map[string]bool{}
	if existing, ok := asArray(rawExisting); ok {
		for _, item := range existing {
			rule, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if source, _ := rule["source"].(string); source == semanticRuleSourceCondition || source == semanticRuleSourceContract {
				continue
			}
			canonicalizeSemanticRule(rule)
			ruleID, _ := rule["ruleId"].(string)
			if strings.TrimSpace(ruleID) == "" || seen[ruleID] {
				continue
			}
			seen[ruleID] = true
			result = append(result, rule)
		}
	}
	for _, rule := range generated {
		ruleID, _ := rule["ruleId"].(string)
		if strings.TrimSpace(ruleID) == "" || seen[ruleID] {
			continue
		}
		seen[ruleID] = true
		result = append(result, rule)
	}
	return result
}

// Keep generated and client-provided rules on the JSON-LD v1 ontology terms.
func canonicalizeSemanticRule(rule map[string]any) {
	if rawOperator, ok := rule[semanticRuleOperatorProperty].(string); ok {
		if operator := normalizeSemanticOperator(rawOperator); operator != "" {
			rule[semanticRuleOperatorProperty] = operator
		}
	}
}

func parseSemanticOperator(raw any) (string, []any) {
	switch value := raw.(type) {
	case string:
		return value, nil
	case map[string]any:
		operate, _ := value["operate"].(string)
		if strings.TrimSpace(operate) == "" {
			operate, _ = value[semanticRuleOperatorProperty].(string)
		}
		if strings.TrimSpace(operate) == "" {
			operate = semanticResourceID(value["odrl:operator"])
		}
		targets := []any{}
		if rawTargets, ok := asArray(value["targets"]); ok {
			targets = append(targets, rawTargets...)
		} else if rawTarget, ok := value[semanticRuleRightOperandProperty]; ok {
			if rawTargets, ok := asArray(rawTarget); ok {
				targets = append(targets, rawTargets...)
			} else {
				targets = append(targets, rawTarget)
			}
		} else if rawTarget, ok := value["odrl:rightOperand"]; ok {
			if rawTargets, ok := asArray(rawTarget); ok {
				targets = append(targets, rawTargets...)
			} else {
				targets = append(targets, rawTarget)
			}
		}
		return operate, targets
	default:
		return "", nil
	}
}

func semanticResourceID(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case map[string]any:
		id, _ := typed["@id"].(string)
		return id
	default:
		return ""
	}
}

func normalizeSemanticOperator(value string) string {
	switch value {
	case "odrl:eq", "Equals":
		return "odrl:eq"
	case "odrl:neq", "NotEquals":
		return "odrl:neq"
	case "odrl:isAnyOf", "In":
		return "odrl:isAnyOf"
	case "odrl:isNoneOf", "NotIn":
		return "odrl:isNoneOf"
	case "odrl:gt", "GreaterThan":
		return "odrl:gt"
	case "odrl:gteq", "GreaterThanOrEqual":
		return "odrl:gteq"
	case "odrl:lt", "LessThan":
		return "odrl:lt"
	case "odrl:lteq", "LessThanOrEqual":
		return "odrl:lteq"
	case "odrl:hasPart", "Contains":
		return "odrl:hasPart"
	case "dcs:between", "Between":
		return "dcs:between"
	case "dcs:matchesRegex", "MatchesRegex":
		return "dcs:matchesRegex"
	default:
		return ""
	}
}

func normalizeSemanticOperateValue(value string) string {
	switch value {
	case "Equals":
		return "odrl:eq"
	case "NotEquals":
		return "odrl:neq"
	case "In":
		return "odrl:isAnyOf"
	case "NotIn":
		return "odrl:isNoneOf"
	case "GreaterThan":
		return "odrl:gt"
	case "GreaterThanOrEqual":
		return "odrl:gteq"
	case "LessThan":
		return "odrl:lt"
	case "LessThanOrEqual":
		return "odrl:lteq"
	case "Contains":
		return "odrl:hasPart"
	case "Between":
		return "dcs:between"
	case "MatchesRegex":
		return "dcs:matchesRegex"
	case "odrl:eq", "odrl:neq", "odrl:isAnyOf", "odrl:isNoneOf", "odrl:gt", "odrl:gteq", "odrl:lt", "odrl:lteq", "odrl:hasPart", "dcs:between", "dcs:matchesRegex":
		return value
	default:
		return ""
	}
}

func semanticRuleSeverity(param map[string]any) string {
	if isTrue(param["isRequired"]) {
		return "blocking"
	}
	return "error"
}

func stringSliceToAny(values []string) []any {
	result := make([]any, len(values))
	for i, value := range values {
		result[i] = value
	}
	return result
}

func slugify(value string) string {
	value = regexp.MustCompile(`([a-z])([A-Z])`).ReplaceAllString(value, "${1}-${2}")
	value = regexp.MustCompile(`[^a-zA-Z0-9]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	return strings.ToLower(value)
}

func buildContractStatements(data documentData) ([]map[string]any, error) {
	if statements := statementsFromValue(data["content"]); len(statements) > 0 {
		return statements, nil
	}
	if statements := statementsFromValue(data["contractData"]); len(statements) > 0 {
		return statements, nil
	}
	if statementSet, ok := data[statementSetDocumentProperty()].(map[string]any); ok {
		return statementsFromValue(statementSet["statements"]), nil
	}
	return []map[string]any{}, nil
}

func normalizeSemanticConditions(data documentData) error {
	conditions, ok := asArray(data["semanticConditions"])
	if !ok {
		return nil
	}
	conditionIDs := map[string]bool{}
	for _, item := range conditions {
		condition, ok := item.(map[string]any)
		if !ok {
			return errors.New("semanticConditions entries must be objects")
		}
		id, err := validateSemanticCondition(condition)
		if err != nil {
			return err
		}
		if conditionIDs[id] {
			return fmt.Errorf("duplicate semantic condition id %q", id)
		}
		conditionIDs[id] = true
	}
	return nil
}

func statementsFromValue(value any) []map[string]any {
	switch typed := value.(type) {
	case []any:
		statements := []map[string]any{}
		for _, item := range typed {
			if statement, ok := item.(map[string]any); ok {
				statements = append(statements, statement)
			}
		}
		return statements
	case map[string]any:
		if statements := statementsFromValue(typed["statements"]); len(statements) > 0 {
			return statements
		}
		if statements := statementsFromValue(typed["@graph"]); len(statements) > 0 {
			return statements
		}
		if len(typed) > 0 {
			return []map[string]any{typed}
		}
	}
	return []map[string]any{}
}

func validateContractSemanticsData(data documentData, requireCompleteStatements bool) error {
	if err := validatePlaceholderBindings(data, requireCompleteStatements); err != nil {
		return err
	}
	if _, err := buildContractStatements(data); err != nil {
		return err
	}
	if requireCompleteStatements && hasContractStatementIntent(data) {
		if err := validateContractStatementCompleteness(data); err != nil {
			return err
		}
	}
	return nil
}

func validateSemanticRules(data documentData) error {
	rules, ok := asArray(data["semanticRules"])
	if !ok {
		return errors.New("semanticRules must be an array")
	}
	for _, item := range rules {
		rule, ok := item.(map[string]any)
		if !ok {
			return errors.New("semanticRules entries must be objects")
		}
		ruleID, _ := rule["ruleId"].(string)
		operator, _ := rule[semanticRuleOperatorProperty].(string)
		if normalizeSemanticOperator(operator) == "" {
			return fmt.Errorf("semantic rule %q uses unsupported semantic rule operator %q", ruleID, operator)
		}
	}
	return nil
}

func hasContractStatementIntent(data documentData) bool {
	statements, err := buildContractStatements(data)
	if err != nil {
		return false
	}
	profile := defaultContractStatementValidationProfile()
	for _, rule := range profile.Rules {
		if CountStatements(statements, rule.Where) > 0 {
			return true
		}
	}
	return false
}

func validatePlaceholderBindings(data documentData, _ bool) error {
	conditions, err := semanticConditionIndex(data)
	if err != nil {
		return err
	}

	bindings, ok := asArray(data["placeholderBindings"])
	if !ok {
		return errors.New("placeholderBindings must be an array")
	}
	bindingByPlaceholder := map[string]map[string]any{}
	for _, item := range bindings {
		binding, ok := item.(map[string]any)
		if !ok {
			return errors.New("placeholderBindings entries must be objects")
		}
		blockID, _ := binding["blockId"].(string)
		placeholder, _ := binding["placeholder"].(string)
		conditionID, _ := binding["boundToCondition"].(string)
		parameterName, _ := binding["boundToParameter"].(string)
		condition := conditions.conditionForBlock(blockID, conditionID)
		if condition == nil {
			return fmt.Errorf("placeholder binding %q references unknown condition %q", placeholder, conditionID)
		}
		if _, found := findParameter(condition, parameterName); !found {
			return fmt.Errorf("placeholder binding %q references unknown parameter %q on condition %q", placeholder, parameterName, conditionID)
		}
		bindingByPlaceholder[blockID+"\x00"+placeholder] = binding
	}

	placeholderPattern := regexp.MustCompile(`\{\{([^}.]+)\.([^}]+)\}\}`)
	blocks, _ := asArray(data["documentBlocks"])
	for _, item := range blocks {
		block, ok := item.(map[string]any)
		if !ok || block["type"] != "CLAUSE" {
			continue
		}
		blockID, _ := block["blockId"].(string)
		text, _ := block["text"].(string)
		for _, match := range placeholderPattern.FindAllStringSubmatch(text, -1) {
			placeholder := match[0]
			if bindingByPlaceholder[blockID+"\x00"+placeholder] == nil {
				return fmt.Errorf("placeholder %q in block %q has no binding", placeholder, blockID)
			}
		}
	}
	return nil
}

func validateContractStatementCompleteness(data documentData) error {
	statements, err := buildContractStatements(data)
	if err != nil {
		return err
	}
	issues := ValidateContractStatements(statements, defaultContractStatementValidationProfile())
	if len(issues) > 0 {
		return ContractStatementValidationError{Issues: issues}
	}
	return nil
}

func numericValue(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	default:
		return 0, false
	}
}

type semanticConditionsByBlock struct {
	topLevel map[string]map[string]any
}

func (conditions semanticConditionsByBlock) conditionForBlock(_ string, conditionID string) map[string]any {
	return conditions.topLevel[conditionID]
}

func semanticConditionIndex(data documentData) (semanticConditionsByBlock, error) {
	conditions := semanticConditionsByBlock{topLevel: map[string]map[string]any{}}
	topLevelConditions, _ := asArray(data["semanticConditions"])
	for _, item := range topLevelConditions {
		condition, ok := item.(map[string]any)
		if !ok {
			return conditions, errors.New("semanticConditions entries must be objects")
		}
		conditionID, _ := condition["conditionId"].(string)
		conditions.topLevel[conditionID] = condition
	}
	return conditions, nil
}

//nolint:unused
func allowedValuesForDomainPath(domainPath string) []any {
	field, ok := ontologyDomainFieldIndex[domainPath]
	if !ok || field.Constraint == nil {
		return []any{}
	}
	values := make([]any, len(field.Constraint.AllowedValues))
	for i, value := range field.Constraint.AllowedValues {
		values[i] = value
	}
	return values
}

func statementsToAny(statements []map[string]any) []any {
	result := make([]any, len(statements))
	for i, statement := range statements {
		result[i] = statement
	}
	return result
}

func validateSemanticCondition(condition map[string]any) (string, error) {
	id, _ := condition["conditionId"].(string)
	if strings.TrimSpace(id) == "" {
		return "", errors.New("semanticConditions entries require conditionId")
	}
	if version, _ := condition["schemaVersion"].(string); version != "v1" {
		return "", fmt.Errorf("semantic condition %q must use schemaVersion v1", id)
	}
	if err := validateSemanticConditionEntity(id, condition); err != nil {
		return "", err
	}
	parameters, ok := asArray(condition["parameters"])
	if !ok {
		return "", fmt.Errorf("semantic condition %q parameters must be an array", id)
	}
	for _, rawParam := range parameters {
		param, ok := rawParam.(map[string]any)
		if !ok {
			return "", fmt.Errorf("semantic condition %q parameter entries must be objects", id)
		}
		name, _ := param["parameterName"].(string)
		paramType, _ := param["type"].(string)
		if strings.TrimSpace(name) == "" || !validSemanticType(paramType) {
			return "", fmt.Errorf("semantic condition %q has invalid parameter", id)
		}
		if err := validateDomainParameter(id, param); err != nil {
			return "", err
		}
		if _, exists := param["fixedValue"]; exists {
			return "", fmt.Errorf("semantic condition %q parameter %q must not use fixedValue", id, param["parameterName"])
		}
		if err := validateSemanticOperators(id, param); err != nil {
			return "", err
		}
	}
	return id, nil
}

func validateSemanticConditionEntity(conditionID string, condition map[string]any) error {
	rawEntityType, _ := condition["entityType"].(string)
	rawEntityRole, hasEntityRole := condition["entityRole"].(string)
	if strings.TrimSpace(rawEntityType) == "" {
		if hasEntityRole && strings.TrimSpace(rawEntityRole) != "" {
			return fmt.Errorf("semantic condition %q entityRole requires entityType", conditionID)
		}
		return nil
	}
	entityType := canonicalStatementEntityType(rawEntityType)
	if entityType == "" {
		return fmt.Errorf("semantic condition %q uses unsupported entityType %q", conditionID, rawEntityType)
	}
	condition["entityType"] = entityType
	if strings.TrimSpace(rawEntityRole) == "" {
		if inferredRole := entityRoleFromEntityType(rawEntityType); inferredRole != "" {
			condition["entityRole"] = inferredRole
		}
		return nil
	}
	if hasEntityRole && strings.TrimSpace(rawEntityRole) != "" {
		if !statementEntityTypeSupportsRole(entityType) {
			return fmt.Errorf("semantic condition %q entityRole is not supported for entityType %q", conditionID, rawEntityType)
		}
		condition["entityRole"] = canonicalEntityRole(rawEntityRole)
	}
	return nil
}

func asArray(value any) ([]any, bool) {
	switch items := value.(type) {
	case []any:
		return items, true
	case []map[string]any:
		result := make([]any, len(items))
		for i, item := range items {
			result[i] = item
		}
		return result, true
	case []string:
		result := make([]any, len(items))
		for i, item := range items {
			result[i] = item
		}
		return result, true
	default:
		return nil, false
	}
}

func validSemanticType(value string) bool {
	switch value {
	case "date", "string", "integer", "decimal", "boolean", "enum":
		return true
	default:
		return false
	}
}

func validateDomainParameter(conditionID string, param map[string]any) error {
	semanticPath, _ := param["semanticPath"].(string)
	if strings.TrimSpace(semanticPath) == "" {
		return fmt.Errorf("semantic condition %q parameter %q requires semanticPath", conditionID, param["parameterName"])
	}
	field, ok := ontologyDomainFieldIndex[semanticPath]
	if !ok {
		return fmt.Errorf("semantic condition %q uses unknown domain semanticPath %q", conditionID, semanticPath)
	}
	if field.OntologyTerm != "" {
		param["semanticPath"] = field.OntologyTerm
	}
	schemaRef, _ := param["schemaRef"].(string)
	if schemaRef != field.SchemaRef {
		return fmt.Errorf("semantic condition %q parameter %q schemaRef must be %q", conditionID, param["parameterName"], field.SchemaRef)
	}
	paramType, _ := param["type"].(string)
	if paramType != field.Type {
		return fmt.Errorf("semantic condition %q parameter %q type must be %q for semanticPath %q", conditionID, param["parameterName"], field.Type, semanticPath)
	}
	if field.Constraint != nil {
		param["valueConstraint"] = field.Constraint.asMap()
	}
	return nil
}

func validateSemanticOperators(conditionID string, param map[string]any) error {
	rawOperators, exists := param["operators"]
	if !exists {
		param["operators"] = []any{}
		return nil
	}
	operators, ok := asArray(rawOperators)
	if !ok {
		return fmt.Errorf("semantic condition %q parameter %q operators must be an array", conditionID, param["parameterName"])
	}
	for _, rawOperator := range operators {
		operate, targets := parseSemanticOperator(rawOperator)
		normalizedOperator := normalizeSemanticOperateValue(operate)
		if normalizedOperator == "" {
			return fmt.Errorf("semantic condition %q parameter %q uses unsupported operator %q", conditionID, param["parameterName"], operate)
		}
		if operatorMap, ok := rawOperator.(map[string]any); ok {
			operatorMap["operate"] = normalizedOperator
		}
		if err := validateSemanticOperatorTargets(conditionID, param, targets); err != nil {
			return err
		}
	}
	return nil
}

func validateSemanticOperatorTargets(conditionID string, param map[string]any, targets []any) error {
	if len(targets) == 0 {
		return nil
	}
	semanticPath, _ := param["semanticPath"].(string)
	field, ok := ontologyDomainFieldIndex[semanticPath]
	if !ok || field.Constraint == nil {
		return nil
	}
	for _, target := range targets {
		if err := valueMatchesConstraint(target, field.Constraint); err != nil {
			return fmt.Errorf("semantic condition %q parameter %q operator target violates constraint: %w", conditionID, param["parameterName"], err)
		}
	}
	return nil
}

func isTrue(value any) bool {
	v, ok := value.(bool)
	return ok && v
}

func findParameter(condition map[string]any, parameterName string) (map[string]any, bool) {
	parameters, _ := asArray(condition["parameters"])
	for _, rawParam := range parameters {
		param := rawParam.(map[string]any)
		if param["parameterName"] == parameterName {
			return param, true
		}
	}
	return nil, false
}

func valueMatchesConstraint(value any, constraint *valueConstraint) error {
	if allowedValues := allowedValuesForConstraint(constraint); len(allowedValues) > 0 {
		text, ok := value.(string)
		if !ok || !containsString(allowedValues, text) {
			return fmt.Errorf("expected one of %s", strings.Join(allowedValues, ", "))
		}
	}
	if constraint.Pattern != "" {
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("expected value matching %s", constraint.Pattern)
		}
		matched, err := regexp.MatchString(constraint.Pattern, text)
		if err != nil {
			return fmt.Errorf("invalid constraint pattern %q: %w", constraint.Pattern, err)
		}
		if !matched {
			return fmt.Errorf("expected value matching %s", constraint.Pattern)
		}
	}
	if constraint.Format != "" {
		if err := valueMatchesFormat(value, constraint.Format); err != nil {
			return err
		}
	}
	if constraint.Min != nil || constraint.Max != nil {
		number, ok := value.(float64)
		if !ok {
			return errors.New("expected numeric constrained value")
		}
		if constraint.Min != nil && number < *constraint.Min {
			return fmt.Errorf("expected value greater than or equal to %v", *constraint.Min)
		}
		if constraint.Max != nil && number > *constraint.Max {
			return fmt.Errorf("expected value less than or equal to %v", *constraint.Max)
		}
	}
	return nil
}

func valueMatchesFormat(value any, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "iso-3166-1-alpha-3", "iso-4217":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("expected value matching format %s", format)
		}
		if !regexp.MustCompile(`^[A-Z]{3}$`).MatchString(text) {
			return fmt.Errorf("expected value matching format %s", format)
		}
	}
	return nil
}

func containsString(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func validateRoleEntities(data documentData) error {
	documentField := ontologyRuntime.RoleEntityDocumentField
	if documentField == "" {
		return nil
	}
	rawEntities, exists := data[documentField]
	if !exists {
		return nil
	}
	entities, ok := asArray(rawEntities)
	if !ok {
		return fmt.Errorf("%s must be an array", documentField)
	}
	for index, rawEntity := range entities {
		entity, ok := rawEntity.(map[string]any)
		if !ok {
			return fmt.Errorf("%s.%d must be an object", documentField, index)
		}
		if err := validateOntologyRoleEntity(entity); err != nil {
			return fmt.Errorf("%s.%d.%w", documentField, index, err)
		}
	}
	return nil
}
