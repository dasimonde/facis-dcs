import { shallowMount } from '@vue/test-utils'
import { createPinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import DataObjectsEditor from '@template-repository/components/data-objects/DataObjectsEditor.vue'
import NegotiateContractView from './NegotiateContractView.vue'

/**
 * A negotiable value declared on a dcs:contractData object (ADR-23) must be
 * redlinable on the Negotiate view, not only while the contract is created —
 * without the data-objects editor there the counterparty has no input for it.
 */

vi.mock(import('vue-router'), async (importOriginal) => ({
  ...(await importOriginal()),
  useRoute: () => ({ params: { did: 'did:web:example.com:contract' } }) as never,
}))

vi.mock('@/services/did-service', () => ({
  getLocalDIDFile: () => Promise.resolve({ id: 'did:web:example.com' }),
}))

const contractDocument = {
  '@type': 'dcs:Contract',
  '@id': 'did:web:example.com:contract',
  'dcs:metadata': { '@type': 'dcs:ContractMetadata' },
  'dcs:documentStructure': {
    '@type': 'dcs:DocumentStructure',
    'dcs:blocks': { '@list': [] },
    'dcs:layout': { '@list': [] },
  },
  'dcs:contractFields': [
    {
      '@id': 'did:web:example.com:contract#field-availability',
      '@type': 'dcs:ContractField',
      'dcs:label': 'Availability',
      'dcs:datatype': 'xsd:decimal',
      'dcs:required': true,
    },
  ],
  'dcs:contractData': [
    {
      '@id': 'did:web:example.com:contract#object-sla',
      '@type': 'https://example.org/shapes#ServiceLevel',
      'https://example.org/shapes#availability': { '@id': 'did:web:example.com:contract#field-availability' },
    },
  ],
  'dcs:policies': [],
}

vi.mock('@/services/contract-workflow-service', () => ({
  contractWorkflowService: {
    retrieveById: () =>
      Promise.resolve({
        did: 'did:web:example.com:contract',
        name: 'SLA',
        description: '',
        state: 'NEGOTIATION',
        contract_data: contractDocument,
        negotiations: [],
      }),
    retrieveNegotiationDraft: () => Promise.resolve(null),
  },
}))

function mountNegotiateView() {
  return shallowMount(NegotiateContractView, { global: { plugins: [createPinia()] } })
}

describe('NegotiateContractView contract-data redlining', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('offers the data-objects editor for a document carrying declared objects', async () => {
    const wrapper = mountNegotiateView()
    await nextTick()
    await nextTick()
    await nextTick()

    const editor = wrapper.findComponent(DataObjectsEditor)
    expect(editor.exists()).toBe(true)
    expect(editor.props('mode')).toBe('contract')
    expect(editor.props('editable')).toBe(true)
    expect(typeof editor.props('setSemanticConditionValue')).toBe('function')
  })

  it('writes an object leaf fill into the same value store the change request reads', async () => {
    const wrapper = mountNegotiateView()
    await nextTick()
    await nextTick()
    await nextTick()

    const setValue = wrapper.findComponent(DataObjectsEditor).props('setSemanticConditionValue')!
    setValue('', 'did:web:example.com:contract#field-availability', 'Availability', '99.5')
    await nextTick()

    const values = wrapper.findComponent(DataObjectsEditor).props('semanticConditionValues')
    expect(values).toEqual([
      {
        blockId: '',
        conditionId: 'did:web:example.com:contract#field-availability',
        parameterName: 'Availability',
        parameterValue: '99.5',
      },
    ])
  })
})
