import { shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useContractContentValuesStore } from '@contract-workflow-engine/store/contractContentValuesStore'
import { useAuthStore } from '@/stores/auth-store'
import { useErrorStore } from '@/stores/error-store'
import NegotiateContractView from './NegotiateContractView.vue'

/**
 * A counter-offer must be checked against the contract's own machine-readable
 * policy before it ships (FR-CWE-07): a value under a declared floor is refused
 * at approval anyway, so the proposing party has to learn which constraint it
 * violates while they can still fix it.
 */

vi.mock(import('vue-router'), async (importOriginal) => ({
  ...(await importOriginal()),
  useRoute: () => ({ params: { did: 'did:web:example.com:contract' } }) as never,
}))

vi.mock('@/services/did-service', () => ({
  getLocalDIDFile: () => Promise.resolve({ id: 'did:web:example.com' }),
}))

const availabilityFieldIri = 'did:web:example.com:contract#field-availability'

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
      '@id': availabilityFieldIri,
      '@type': 'dcs:ContractField',
      'dcs:label': 'Availability',
      'dcs:datatype': 'xsd:decimal',
      'dcs:required': true,
    },
  ],
  'dcs:contractData': [],
  'dcs:policies': {
    '@type': 'odrl:Offer',
    '@id': 'did:web:example.com:contract#policy-set',
    'odrl:obligation': [
      {
        '@type': 'odrl:Duty',
        '@id': 'did:web:example.com:contract#rule-availability-floor',
        'odrl:constraint': [
          {
            '@type': 'odrl:Constraint',
            'odrl:leftOperand': { '@id': availabilityFieldIri },
            'odrl:operator': { '@id': 'odrl:gteq' },
            'odrl:rightOperand': { '@value': '99', '@type': 'xsd:decimal' },
          },
        ],
      },
    ],
  },
}

const negotiate = vi.fn(() => Promise.resolve({ did: 'did:web:example.com:contract' }))

vi.mock('@/services/contract-workflow-service', () => ({
  contractWorkflowService: {
    retrieveById: () =>
      Promise.resolve({
        did: 'did:web:example.com:contract',
        name: 'SLA',
        description: '',
        state: 'NEGOTIATION',
        updated_at: '2026-07-29T00:00:00Z',
        contract_data: contractDocument,
        negotiations: [],
      }),
    retrieveNegotiationDraft: () => Promise.resolve(null),
    negotiate: (...args: unknown[]) => negotiate(...(args as [])),
  },
}))

async function mountNegotiateView() {
  const pinia = createPinia()
  const wrapper = shallowMount(NegotiateContractView, { global: { plugins: [pinia] } })
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'did:web:example.com:org', holder: 'user', roles: [] }
  await nextTick()
  await nextTick()
  await nextTick()
  return wrapper
}

function proposalButton(wrapper: Awaited<ReturnType<typeof mountNegotiateView>>) {
  return wrapper.findAll('button').find((button) => button.text().includes('Change Proposal'))
}

describe('NegotiateContractView change proposal verification', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('refuses to ship a proposal whose value breaks a declared floor, naming the constraint', async () => {
    const wrapper = await mountNegotiateView()
    useContractContentValuesStore().setSemanticConditionValue({
      blockId: '',
      conditionId: availabilityFieldIri,
      parameterName: 'Availability',
      parameterValue: 95,
    })
    await nextTick()

    await proposalButton(wrapper)?.trigger('click')
    await nextTick()

    expect(negotiate).not.toHaveBeenCalled()
    expect(useErrorStore().errors.map((error) => error.message)).toEqual([
      '"Availability" violates an ODRL obligation. Expected >= 99.',
    ])
  })

  it('ships a proposal whose value honours the declared floor', async () => {
    const wrapper = await mountNegotiateView()
    useContractContentValuesStore().setSemanticConditionValue({
      blockId: '',
      conditionId: availabilityFieldIri,
      parameterName: 'Availability',
      parameterValue: 99.5,
    })
    await nextTick()

    await proposalButton(wrapper)?.trigger('click')
    await nextTick()

    expect(negotiate).toHaveBeenCalledTimes(1)
    expect(useErrorStore().errors).toEqual([])
  })
})
