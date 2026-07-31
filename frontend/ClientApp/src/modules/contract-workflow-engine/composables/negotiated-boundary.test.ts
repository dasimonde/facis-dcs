import { describe, expect, it } from 'vitest'
import { getSemanticConditionsFromTemplateData } from '@template-repository/store/dcsDraftStore'
import { useSemanticValueVerification } from '@contract-workflow-engine/composables/useSemanticValueVerification'
import type { SemanticConditionValue } from '@/models/contract/contract-data'
import type { DcsContractField, DcsDocumentData, OdrlConstraint, OdrlRule } from '@/models/dcs-jsonld'

/**
 * A negotiated boundary is an odrl:rightOperand that references a contract
 * field instead of carrying a literal: the bound is agreed while the contract
 * is filled in. These check what the pre-submit verification makes of one.
 */

const IRI = 'did:web:example:template:sla'
const RESPONSE = `${IRI}#field-response-minutes`
const ESCALATION = `${IRI}#field-escalation-minutes`

function field(id: string, label: string, value?: number): DcsContractField {
  return {
    '@id': id,
    '@type': 'dcs:ContractField',
    'dcs:label': label,
    'dcs:datatype': 'xsd:integer',
    'dcs:required': true,
    ...(value === undefined ? {} : { 'dcs:value': { '@value': String(value), '@type': 'xsd:integer' } }),
  }
}

function obligation(constraint: OdrlConstraint): OdrlRule {
  return {
    '@id': `${IRI}#policy-response`,
    '@type': 'odrl:Duty',
    'odrl:action': { '@id': 'odrl:inform' },
    'odrl:target': { '@id': IRI },
    'odrl:constraint': [constraint],
  } as OdrlRule
}

function document(constraint: OdrlConstraint, fields: DcsContractField[]): DcsDocumentData {
  return {
    '@type': 'dcs:Contract',
    '@id': IRI,
    'dcs:metadata': { 'dcs:title': 'SLA' },
    'dcs:documentStructure': { 'dcs:blocks': { '@list': [] }, 'dcs:layout': { '@list': [] } },
    'dcs:contractData': [],
    'dcs:contractFields': fields,
    'dcs:policies': { '@type': 'odrl:Offer', 'odrl:obligation': [obligation(constraint)] },
  } as unknown as DcsDocumentData
}

function filled(id: string, label: string, value: number): SemanticConditionValue {
  return { blockId: '', conditionId: id, parameterName: label, parameterValue: value }
}

const { verifySemanticValue } = useSemanticValueVerification()

describe('a boundary that references a contract field', () => {
  // The constraint the rule builder emits for "left operand: a data field,
  // boundary: the agreed value of another data field".
  const boundedByField: OdrlConstraint = {
    '@type': 'odrl:Constraint',
    'odrl:leftOperand': { '@id': RESPONSE },
    'odrl:operator': { '@id': 'odrl:lteq' },
    'odrl:rightOperand': { '@id': ESCALATION },
  }

  it('compares the value against the referenced field, not against nothing', () => {
    const conditions = getSemanticConditionsFromTemplateData(
      document(boundedByField, [field(RESPONSE, 'Response time', 15), field(ESCALATION, 'Escalation threshold', 30)]),
    )
    const result = verifySemanticValue(
      conditions,
      [filled(RESPONSE, 'Response time', 15), filled(ESCALATION, 'Escalation threshold', 30)],
      [],
    )
    expect(result.errors.map((error) => error.message)).toEqual([])
    expect(result.isValid, '15 minutes is within a 30 minute threshold').toBe(true)
  })

  it('reads the boundary the user is typing, not the one last saved', () => {
    const conditions = getSemanticConditionsFromTemplateData(
      document(boundedByField, [field(RESPONSE, 'Response time', 15), field(ESCALATION, 'Escalation threshold', 30)]),
    )
    const result = verifySemanticValue(
      conditions,
      [filled(RESPONSE, 'Response time', 15), filled(ESCALATION, 'Escalation threshold', 10)],
      [],
    )
    expect(result.isValid, '15 minutes exceeds the 10 minutes now being negotiated').toBe(false)
    expect(result.errors[0]?.message).toContain('Expected <= 10')
  })

  it('holds nothing against a value while its boundary is still unfilled', () => {
    const conditions = getSemanticConditionsFromTemplateData(
      document(boundedByField, [field(RESPONSE, 'Response time'), field(ESCALATION, 'Escalation threshold')]),
    )
    const result = verifySemanticValue(conditions, [filled(RESPONSE, 'Response time', 15)], [])
    expect(result.errors.map((error) => error.message)).toEqual([])
  })

  // The SLA fixture's nine boundaries are all of this shape: the left operand
  // is an ODRL context operand (odrl:elapsedTime, odrl:spatial, …), evaluated
  // at use-time, so no contract field carries the operator at all.
  it('leaves a context operand out of the fields the editor verifies', () => {
    const conditions = getSemanticConditionsFromTemplateData(
      document(
        {
          '@type': 'odrl:Constraint',
          'odrl:leftOperand': { '@id': 'odrl:elapsedTime' },
          'odrl:operator': { '@id': 'odrl:lteq' },
          'odrl:rightOperand': { '@id': ESCALATION },
        },
        [field(RESPONSE, 'Response time', 15), field(ESCALATION, 'Escalation threshold', 30)],
      ),
    )
    expect(conditions.flatMap((condition) => condition.parameters).map((parameter) => parameter.operators)).toEqual([
      [],
      [],
    ])
  })
})
