package validation

const (
	SchemaDocumentStructureV1 = "facis.dcs.document-structure.v1"
	SchemaTemplateDataV1      = "facis.dcs.template-data.v1"
	SchemaContractDataV1      = "facis.dcs.contract-data.v1"
	SchemaSemanticConditionV1 = "facis.dcs.semantic-condition.v1"
	SchemaPartyV1             = "facis.dcs.party.v1"
	SchemaContractV1          = "facis.dcs.contract.v1"
	SchemaServiceV1           = "facis.dcs.service.v1"
	SchemaSignatureV1         = "facis.dcs.signature.v1"
	SchemaJSONLDContextV1     = "https://w3id.org/facis/dcs/context/v1"
	SchemaOntologyV1          = "https://w3id.org/facis/sla/ontology"
	SchemaSHACLShapesV1       = "https://w3id.org/facis/dcs/shapes/v1"

	PolicyTemplateStructureV1          = "facis.dcs.template.structure"
	PolicyTemplateSemanticConditionsV1 = "facis.dcs.template.semantic-conditions"
	PolicyContractStructureV1          = "facis.dcs.contract.structure"
	PolicyContractSemanticValuesV1     = "facis.dcs.contract.semantic-values"

	semanticRuleOperatorProperty        = "operator"
	semanticRuleRightOperandProperty    = "rightOperand"
	semanticRuleAppliesToClauseProperty = "appliesToClause"

	semanticRuleSourceContract  = "contractSemantics"
	semanticRuleSourceCondition = "semanticCondition"
)

var (
	contractPolicyRefs = []map[string]any{
		{"policyId": PolicyContractStructureV1, "version": "v1", "enforcementPoint": "contract:create"},
		{"policyId": PolicyContractSemanticValuesV1, "version": "v1", "enforcementPoint": "contract:update"},
	}
)
