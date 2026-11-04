import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import NegotiateContractView from './NegotiateContractView.vue'
import type { ContractNegotiationRequest } from '@/models/requests/contract-request'

/** Signing workflow changes must enable and be included in a change proposal. */

const negotiate = vi.hoisted(() =>
  vi.fn((_request: ContractNegotiationRequest) => Promise.resolve({ did: 'did:web:example.com:contract' })),
)

vi.mock(import('vue-router'), async (importOriginal) => ({
  ...(await importOriginal()),
  useRoute: () => ({ params: { did: 'did:web:example.com:contract' } }) as never,
}))

vi.mock('@/services/did-service', () => ({
  getLocalDIDFile: () => Promise.resolve({ id: 'did:web:example.com' }),
  resolveLocalInstanceDid: () => Promise.resolve('did:web:example.com'),
}))

vi.mock('@/services/contract-workflow-service', () => ({
  contractWorkflowService: {
    retrieveById: () =>
      Promise.resolve({
        did: 'did:web:example.com:contract',
        name: 'SLA',
        description: 'Service level',
        state: 'NEGOTIATION',
        contract_version: 1,
        contract_data: {
          '@type': 'dcs:Contract',
          '@id': 'did:web:example.com:contract',
          'dcs:metadata': { '@type': 'dcs:ContractMetadata' },
          'dcs:documentStructure': {
            '@type': 'dcs:DocumentStructure',
            'dcs:blocks': { '@list': [] },
            'dcs:layout': { '@list': [] },
          },
          'dcs:contractFields': [],
          'dcs:contractData': [],
          'dcs:policies': [],
          'dcs:signatureFields': [{ 'dcs:signatoryName': 'did:web:a', 'dcs:signingOrder': 1 }],
          'dcs:signingDeadline': { '@value': '2026-11-30', '@type': 'xsd:date' },
        },
        negotiations: [],
      }),
    retrieveNegotiationDraft: () => Promise.resolve(null),
    negotiate,
  },
}))

describe('NegotiateContractView signing-workflow redlining', () => {
  beforeEach(() => {
    negotiate.mockClear()
  })

  it('enables Propose when only the signing deadline changes and ships contract_data', async () => {
    const pinia = createPinia()
    setActivePinia(pinia)
    useAuthStore().user = {
      issuer: 'did:web:example.com:org',
      holder: 'user',
      roles: ['CONTRACT_NEGOTIATOR'],
    } as never

    const wrapper = mount(NegotiateContractView, {
      global: {
        plugins: [pinia],
        stubs: {
          TemplatePreview: true,
          DataObjectsEditor: true,
          AuditView: true,
          ContractHistoryDiffView: true,
          ContractProposalReview: true,
          WorkflowStageBanner: true,
          ContractManagerActions: true,
        },
      },
    })
    await flushPromises()

    const propose = wrapper.get('[data-testid="change-proposal"]')
    expect(propose.attributes('disabled')).toBeDefined()

    await wrapper.get('[data-testid="contract-signing-deadline"]').setValue('2026-12-15')
    await wrapper.get('[data-testid="contract-signing-order-did:web:a"]').setValue('2')
    await flushPromises()

    expect(propose.attributes('disabled')).toBeUndefined()

    await propose.trigger('click')
    await flushPromises()

    expect(negotiate).toHaveBeenCalledWith(
      expect.objectContaining({
        change_request: expect.objectContaining({
          contract_data: expect.objectContaining({
            'dcs:signingDeadline': { '@value': '2026-12-15', '@type': 'xsd:date' },
          }),
        }),
      }),
    )
    const shipped = negotiate.mock.calls[0]?.[0]?.change_request?.contract_data as {
      'dcs:signatureFields': { 'dcs:signingOrder': number }[]
    }
    expect(shipped['dcs:signatureFields'][0]?.['dcs:signingOrder']).toBe(2)
  })
})
