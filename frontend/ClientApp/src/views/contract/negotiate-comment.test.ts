import { flushPromises, shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import NegotiateContractView from './NegotiateContractView.vue'

// DCS-IR-CWE-03: parties exchange comments with their proposals before approval.

const negotiate = vi.hoisted(() => vi.fn(() => Promise.resolve({ did: 'did:web:example.com:contract' })))

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
        name: 'Supply agreement',
        description: 'Supply of parts',
        state: 'NEGOTIATION',
        updated_at: '2026-07-31T00:00:00Z',
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
        },
        negotiations: [],
      }),
    retrieveNegotiationDraft: () => Promise.resolve(null),
    negotiate,
  },
}))

it('sends a comment to the other party with the proposal', async () => {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'did:web:example.com:org', holder: 'user', roles: ['CONTRACT_NEGOTIATOR'] }
  const wrapper = shallowMount(NegotiateContractView, { global: { plugins: [pinia] } })
  await flushPromises()

  await wrapper.get('[data-testid="proposal-comment-input"]').setValue('  Please extend the term.  ')
  await wrapper.get('[data-testid="change-proposal"]').trigger('click')
  await flushPromises()

  expect(negotiate).toHaveBeenCalledWith(
    expect.objectContaining({ change_request: { comment: 'Please extend the term.' } }),
  )
})
