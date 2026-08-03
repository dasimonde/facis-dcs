import { describe, expect, it } from 'vitest'
import { getSemanticConditionsFromTemplateData } from '@template-repository/store/dcsDraftStore'
import { useSemanticValueVerification } from '@contract-workflow-engine/composables/useSemanticValueVerification'
import type { SemanticConditionValue } from '@/models/contract/contract-data'
import type { DcsContractField, DcsDocumentData, OdrlConstraint, OdrlRule } from '@/models/dcs-jsonld'

/**
 * An xsd:duration boundary — the SLA profile's "respond within P14D" — must
 * be checked by magnitude. Byte order puts "PT6H" after "PT24H", so a lexical
 * comparison reports a six-hour response window as breaching a 24-hour limit:
 * it does not fail, it answers, and the answer is wrong.
 */

const IRI = 'did:web:example:template:sla'
const WINDOW = `${IRI}#field-response-window`

function durationField(): DcsContractField {
  return {
    '@id': WINDOW,
    '@type': 'dcs:ContractField',
    'dcs:label': 'Response window',
    'dcs:datatype': 'xsd:duration',
    'dcs:required': true,
  }
}

function document(bound: string): DcsDocumentData {
  const constraint: OdrlConstraint = {
    '@type': 'odrl:Constraint',
    'odrl:leftOperand': { '@id': WINDOW },
    'odrl:operator': { '@id': 'odrl:lteq' },
    'odrl:rightOperand': { '@value': bound, '@type': 'xsd:duration' },
  }
  const obligation = {
    '@id': `${IRI}#policy-response`,
    '@type': 'odrl:Duty',
    'odrl:action': { '@id': 'odrl:inform' },
    'odrl:target': { '@id': IRI },
    'odrl:constraint': [constraint],
  } as OdrlRule
  return {
    '@type': 'dcs:Contract',
    '@id': IRI,
    'dcs:metadata': { 'dcs:title': 'SLA' },
    'dcs:documentStructure': { 'dcs:blocks': { '@list': [] }, 'dcs:layout': { '@list': [] } },
    'dcs:contractData': [],
    'dcs:contractFields': [durationField()],
    'dcs:policies': { '@type': 'odrl:Offer', 'odrl:obligation': [obligation] },
  } as unknown as DcsDocumentData
}

function fill(value: string): SemanticConditionValue[] {
  return [{ blockId: '', conditionId: WINDOW, parameterName: 'Response window', parameterValue: value }]
}

const { verifySemanticValue } = useSemanticValueVerification()

function errorsFor(filled: string, bound: string): string[] {
  const conditions = getSemanticConditionsFromTemplateData(document(bound))
  return verifySemanticValue(conditions, fill(filled), []).errors.map((error) => error.message)
}

describe('an xsd:duration boundary', () => {
  it('clears a six-hour window against a 24-hour limit, which byte order does not', () => {
    expect('PT6H' <= 'PT24H').toBe(false)
    expect(errorsFor('PT6H', 'PT24H')).toEqual([])
  })

  it('clears P5D against P14D', () => {
    expect(errorsFor('P5D', 'P14D')).toEqual([])
  })

  it('still reports a window that genuinely exceeds the limit', () => {
    expect(errorsFor('P30D', 'P14D')).toHaveLength(1)
  })

  it('accepts the limit itself', () => {
    expect(errorsFor('P14D', 'P14D')).toEqual([])
  })

  it('compares across notations: PT24H is P1D', () => {
    expect(errorsFor('PT24H', 'P1D')).toEqual([])
  })
})
