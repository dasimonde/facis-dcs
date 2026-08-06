import { describe, expect, it } from 'vitest'
import { getSemanticConditionsFromTemplateData } from '@template-repository/store/dcsDraftStore'
import { useSemanticValueVerification } from '@contract-workflow-engine/composables/useSemanticValueVerification'
import { fromDocumentSemanticValues } from '@contract-workflow-engine/utils/semantic-condition-values'
import type { SemanticConditionValue } from '@/models/contract/contract-data'
import type { DcsContractField, DcsDocumentData } from '@/models/dcs-jsonld'

/**
 * A negotiable leaf of a typed domain object (ADR-23) is reached through its
 * object, never named in clause prose — only the fee below is placed inline.
 * These check that a value filled into such a leaf survives the editor's
 * load-time cleanup and is then held to the policy that bounds it.
 */

const IRI = 'did:web:example:contract:sla'
const SLA_OBJECT = `${IRI}#object-sla`
const AVAILABILITY = `${IRI}#field-committed-availability`
const FEE = `${IRI}#field-payment-amount`
const CLAUSE = `${IRI}#block-clause`

function field(id: string, label: string, datatype: string, value?: string): DcsContractField {
  return {
    '@id': id,
    '@type': 'dcs:ContractField',
    'dcs:label': label,
    'dcs:datatype': datatype,
    'dcs:required': true,
    ...(value === undefined ? {} : { 'dcs:value': { '@value': value, '@type': datatype } }),
  } as DcsContractField
}

/** The contract as it reaches the consumer: the availability floor the
 *  producer published, on a leaf the SLA object binds. */
function document(availability?: string, fee?: string): DcsDocumentData {
  return {
    '@type': 'dcs:Contract',
    '@id': IRI,
    'dcs:metadata': { 'dcs:title': 'SLA' },
    'dcs:documentStructure': {
      'dcs:blocks': {
        '@list': [
          {
            '@type': 'dcs:Clause',
            '@id': CLAUSE,
            'dcs:title': 'Service levels',
            'dcs:content': { '@list': ['The provider invoices ', { '@id': FEE }] },
          },
        ],
      },
      'dcs:layout': { '@list': [] },
    },
    'dcs:contractData': [
      {
        '@id': SLA_OBJECT,
        '@type': 'https://example.org/sla#SLA',
        'https://example.org/sla#committedAvailability': { '@id': AVAILABILITY },
      },
    ],
    'dcs:contractFields': [
      field(AVAILABILITY, 'SLA · Committed availability', 'xsd:string', availability),
      field(FEE, 'Payment Amount', 'xsd:decimal', fee),
    ],
    'dcs:policies': {
      '@type': 'odrl:Offer',
      'odrl:permission': [
        {
          '@id': `${IRI}#policy-availability`,
          '@type': 'odrl:Permission',
          'odrl:action': { '@id': 'odrl:use' },
          'odrl:target': { '@id': SLA_OBJECT },
          'odrl:constraint': [
            {
              '@type': 'odrl:Constraint',
              'odrl:leftOperand': { '@id': AVAILABILITY },
              'odrl:operator': { '@id': 'odrl:gteq' },
              'odrl:rightOperand': { '@value': '99.5', '@type': 'xsd:decimal' },
            },
          ],
        },
      ],
    },
  } as unknown as DcsDocumentData
}

/** The cleanup the contract views run whenever the loaded document changes:
 *  a value the document no longer reaches is discarded. */
function surviveCleanup(cd: DcsDocumentData, values: SemanticConditionValue[]): SemanticConditionValue[] {
  const { hasConditionParameterForValue } = useSemanticValueVerification()
  const blocks = cd['dcs:documentStructure']['dcs:blocks']['@list']
  const conditions = getSemanticConditionsFromTemplateData(cd)
  return values.filter((value) => hasConditionParameterForValue(value, blocks, conditions, cd['dcs:contractData']))
}

describe('a value filled into a typed domain object leaf', () => {
  it('survives the load-time cleanup although no clause names it', () => {
    const cd = document('94.0', '20000')
    const loaded = fromDocumentSemanticValues(cd['dcs:contractFields'])
    expect(loaded.map((value) => value.conditionId)).toEqual([AVAILABILITY, FEE])
    expect(surviveCleanup(cd, loaded).map((value) => value.conditionId)).toEqual([AVAILABILITY, FEE])
  })

  it('is refused for breaking the floor the document publishes', () => {
    const cd = document('94.0', '20000')
    const { verifySemanticValue } = useSemanticValueVerification()
    const result = verifySemanticValue(
      getSemanticConditionsFromTemplateData(cd),
      surviveCleanup(cd, fromDocumentSemanticValues(cd['dcs:contractFields'])),
      cd['dcs:documentStructure']['dcs:blocks']['@list'],
    )
    expect(result.isValid).toBe(false)
    expect(result.errors.map((error) => error.message)).toContain(
      '"SLA · Committed availability" violates an ODRL obligation. Expected >= 99.5.',
    )
  })

  it('passes once the fill clears the floor', () => {
    const cd = document('99.9', '20000')
    const { verifySemanticValue } = useSemanticValueVerification()
    const result = verifySemanticValue(
      getSemanticConditionsFromTemplateData(cd),
      surviveCleanup(cd, fromDocumentSemanticValues(cd['dcs:contractFields'])),
      cd['dcs:documentStructure']['dcs:blocks']['@list'],
    )
    expect(result.errors.map((error) => error.message)).toEqual([])
    expect(result.isValid).toBe(true)
  })

  it('still discards a value whose field the document no longer reaches', () => {
    const cd = document('94.0', '20000')
    const orphaned: SemanticConditionValue = {
      blockId: '',
      conditionId: `${IRI}#field-removed`,
      parameterName: 'Removed',
      parameterValue: 'x',
    }
    expect(surviveCleanup(cd, [orphaned])).toEqual([])
  })
})
