import { type DcsContractData, type DcsContractField, fieldFillScalar, typedFieldFill } from '@/models/dcs-jsonld'
import type { SemanticConditionValue } from '@/models/contract/contract-data'

/**
 * Boundary between the editor's (blockId, conditionId, parameterName)
 * view-model and the canonical document shape, where a submitted value is
 * carried inline on its dcs:ContractField (dcs:value) — the same node an ODRL
 * constraint names as its odrl:leftOperand.
 */

/** The document's declared contract fields. */
export function collectDeclaredRequirements(cd: Partial<DcsContractData>): DcsContractField[] {
  return [...(cd['dcs:contractFields'] ?? [])]
}

/**
 * Writes each submitted value inline onto the dcs:ContractField it targets
 * (matched by @id), returning new field objects. A field with no
 * submitted value carries no dcs:value.
 */
export function applyInlineSemanticValues(
  fields: DcsContractField[],
  values: SemanticConditionValue[],
): DcsContractField[] {
  const byId = new Map<string, SemanticConditionValue>()
  for (const value of values) {
    byId.set(value.conditionId, value)
  }
  return fields.map((field) => {
    const value = byId.get(field['@id'])
    const { 'dcs:value': _value, ...rest } = field
    if (value?.parameterValue === undefined) {
      return rest
    }
    return {
      ...rest,
      'dcs:value': typedFieldFill(value.parameterValue, field['dcs:datatype']),
    }
  })
}

/** The editor view-model reconstructed from contract fields' inline values. */
export function fromDocumentSemanticValues(fields: DcsContractField[]): SemanticConditionValue[] {
  const values: SemanticConditionValue[] = []
  for (const field of fields) {
    const scalar = fieldFillScalar(field['dcs:value'], field['dcs:datatype'])
    if (scalar === undefined) continue
    values.push({
      blockId: '',
      conditionId: field['@id'],
      parameterName: field['dcs:label'],
      parameterValue: scalar,
    })
  }
  return values
}
