import { describe, expect, it } from 'vitest'
import {
  findVerificationError,
  useSemanticValueVerification,
} from '@contract-workflow-engine/composables/useSemanticValueVerification'
import type { SemanticConditionValue } from '@/models/contract/contract-data'
import type { DcsBlock } from '@/models/dcs-jsonld'
import type { SemanticCondition } from '@template-repository/models/contract-template'

/**
 * The preview marks a placeholder invalid by looking its error up in the
 * verification result. These check that a value-level error actually reaches
 * the block(s) rendering that placeholder.
 */

const IRI = 'did:web:example:template:sla'
const RESPONSE = `${IRI}#field-response-minutes`
const CLAUSE_A = `${IRI}#clause-a`
const CLAUSE_B = `${IRI}#clause-b`

function condition(): SemanticCondition {
  return {
    conditionId: RESPONSE,
    conditionName: 'Response time',
    schemaVersion: 'v1',
    parameters: [
      {
        parameterName: 'responseMinutes',
        fieldId: RESPONSE,
        fieldIri: RESPONSE,
        type: 'integer',
        isRequired: true,
        operators: [],
        value: undefined,
        valueConstraint: { max: 60 },
      },
    ],
  }
}

function clause(id: string): DcsBlock {
  return {
    '@type': 'dcs:Clause',
    '@id': id,
    'dcs:content': { '@list': ['Response within ', { '@id': RESPONSE }, ' minutes.'] },
  }
}

/** A filled value is keyed by its placeholder @id, block-agnostically. */
function filled(value: string | number | boolean): SemanticConditionValue {
  return { blockId: '', conditionId: RESPONSE, parameterName: 'responseMinutes', parameterValue: value }
}

describe('inline invalid markers', () => {
  const { verifySemanticValue } = useSemanticValueVerification()

  it('marks the placeholder in the clause that renders a violating value', () => {
    const result = verifySemanticValue([condition()], [filled(120)], [clause(CLAUSE_A)])

    expect(result.isValid).toBe(false)
    expect(findVerificationError(result, CLAUSE_A, RESPONSE, 'responseMinutes')?.message).toContain(
      'less than or equal to 60',
    )
  })

  it('marks every clause that renders the same placeholder', () => {
    const result = verifySemanticValue([condition()], [filled(120)], [clause(CLAUSE_A), clause(CLAUSE_B)])

    expect(findVerificationError(result, CLAUSE_A, RESPONSE, 'responseMinutes')).not.toBeNull()
    expect(findVerificationError(result, CLAUSE_B, RESPONSE, 'responseMinutes')).not.toBeNull()
  })

  it('marks nothing when the value satisfies its constraint', () => {
    const result = verifySemanticValue([condition()], [filled(30)], [clause(CLAUSE_A)])

    expect(result.isValid).toBe(true)
    expect(findVerificationError(result, CLAUSE_A, RESPONSE, 'responseMinutes')).toBeNull()
  })

  it('reports a missing required value on the clause that requires it', () => {
    const result = verifySemanticValue([condition()], [], [clause(CLAUSE_A)])

    expect(findVerificationError(result, CLAUSE_A, RESPONSE, 'responseMinutes')?.message).toContain(
      'required but has no value',
    )
  })

  it('keeps a block-scoped error out of other blocks', () => {
    const scoped = {
      isValid: false,
      errors: [{ blockId: CLAUSE_A, conditionId: RESPONSE, parameterName: 'responseMinutes', message: 'nope' }],
    }

    expect(findVerificationError(scoped, CLAUSE_A, RESPONSE, 'responseMinutes')).not.toBeNull()
    expect(findVerificationError(scoped, CLAUSE_B, RESPONSE, 'responseMinutes')).toBeNull()
  })

  it('matches on the value identity, not just the block', () => {
    const other = {
      isValid: false,
      errors: [{ blockId: '', conditionId: RESPONSE, parameterName: 'otherParam', message: 'nope' }],
    }

    expect(findVerificationError(other, CLAUSE_A, RESPONSE, 'responseMinutes')).toBeNull()
    expect(findVerificationError(null, CLAUSE_A, RESPONSE, 'responseMinutes')).toBeNull()
  })
})
