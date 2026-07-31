import { ONTOLOGY_DOMAIN_FIELDS } from './ontology-domain-fields'
import type { SemanticConditionParameter } from '@template-repository/models/contract-template'

const ontologyLabelByIri = new Map(ONTOLOGY_DOMAIN_FIELDS.map((field) => [field.ontologyId, field.label]))

export function semanticParameterLabel(parameter: SemanticConditionParameter): string {
  return parameter.uiMetadata?.label ?? ontologyLabelByIri.get(parameter.fieldIri) ?? parameter.parameterName
}
