import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import type { ContractTemplate } from '@/models/contract-template/contract-template'
import type { DcsTemplateData, OdrlConstraint } from '@/models/dcs-jsonld'

/**
 * Inlining a component rewrites every @id it owns, so every in-document
 * reference must be rewritten with it. An odrl:unit naming a contract field —
 * a currency the parties negotiate — is such a reference: left behind, it
 * points at a field id no longer in the document, and the boundary silently
 * stops saying what it is measured in.
 */

const COMPONENT_IRI = 'did:web:example:component:payment'
const AMOUNT_FIELD = `${COMPONENT_IRI}#field-payment-amount`
const CURRENCY_FIELD = `${COMPONENT_IRI}#field-payment-currency`
const CLAUSE_BLOCK = `${COMPONENT_IRI}#block-charges`
const ROOT_LAYOUT = `${COMPONENT_IRI}#layout-root`

function field(id: string, label: string, datatype: 'xsd:decimal' | 'xsd:string') {
  return {
    '@id': id,
    '@type': 'dcs:ContractField' as const,
    'dcs:label': label,
    'dcs:datatype': datatype,
    'dcs:required': true,
  }
}

function paymentComponent(): ContractTemplate {
  const templateData: DcsTemplateData = {
    '@type': 'dcs:ContractTemplate',
    'dcs:metadata': { 'dcs:name': 'Payment', 'dcs:description': 'A negotiated payment' },
    'dcs:contractFields': [
      field(AMOUNT_FIELD, 'Payment Amount', 'xsd:decimal'),
      field(CURRENCY_FIELD, 'Payment Currency', 'xsd:string'),
    ],
    'dcs:contractData': [],
    'dcs:documentStructure': {
      '@type': 'dcs:DocumentStructure',
      'dcs:blocks': {
        '@list': [
          {
            '@id': CLAUSE_BLOCK,
            '@type': 'dcs:Clause',
            'dcs:title': 'Charges',
            'dcs:content': { '@list': ['The customer pays ', { '@id': AMOUNT_FIELD }] },
          },
        ],
      },
      'dcs:layout': {
        '@list': [
          {
            '@id': ROOT_LAYOUT,
            '@type': 'dcs:LayoutNode',
            'dcs:isRoot': true,
            'dcs:children': { '@list': [{ '@id': CLAUSE_BLOCK }] },
          },
        ],
      },
    },
    'dcs:policies': {
      '@type': 'odrl:Offer',
      'odrl:profile': { '@id': 'https://w3id.org/facis/dcs/ontology/v1/odrl-profile' },
      'odrl:obligation': [
        {
          '@id': `${COMPONENT_IRI}#rule-payment`,
          '@type': 'odrl:Duty',
          'odrl:action': { '@id': 'dcs:provideCompliantValue' },
          'dcs:prose': { '@id': CLAUSE_BLOCK },
          'odrl:constraint': [
            {
              '@type': 'odrl:Constraint',
              'odrl:leftOperand': { '@id': AMOUNT_FIELD },
              'odrl:operator': { '@id': 'odrl:lteq' },
              'odrl:rightOperand': { '@value': '10000', '@type': 'xsd:decimal' },
              'odrl:unit': { '@id': CURRENCY_FIELD },
            },
          ],
        },
      ],
    },
  } as unknown as DcsTemplateData

  return { did: COMPONENT_IRI, name: 'Payment', template_data: templateData } as unknown as ContractTemplate
}

describe('inlining a component whose boundary is denominated in a negotiated unit', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('rewrites the odrl:unit field reference along with the field itself', () => {
    const store = useDcsDraftStore()
    store.$patch({ did: 'did:web:example:template:host' })
    const rootId = store.layout[0]?.['@id']
    expect(rootId).toBeTruthy()

    store.inlineComponent(paymentComponent(), rootId!, 0)

    const currency = store.contractFields.find((f) => f['dcs:label'] === 'Payment Currency')
    expect(currency).toBeDefined()
    expect(currency!['@id']).not.toBe(CURRENCY_FIELD)

    const constraint = store.policies[0]?.['odrl:constraint']?.[0] as OdrlConstraint
    expect(constraint['odrl:unit']).toEqual({ '@id': currency!['@id'] })
  })
})
