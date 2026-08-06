import { shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import { useAuthStore } from '@/stores/auth-store'
import NegotiateContractView from './NegotiateContractView.vue'
import type { UserRole } from '@/types/user-role'

/**
 * Submit closes a negotiation round. command/submit.go's NEGOTIATION branch
 * accepts Contract Creator, Contract Negotiator and Contract Reviewer — the
 * negotiator reaches the view through the Negotiation Tasks tab but had the
 * one control that ends the round permanently greyed out.
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
  'dcs:contractFields': [],
  'dcs:contractData': [],
}

vi.mock('@/services/contract-workflow-service', () => ({
  contractWorkflowService: {
    retrieveById: () =>
      Promise.resolve({
        did: 'did:web:example.com:contract',
        name: 'Supply agreement',
        description: '',
        state: 'NEGOTIATION',
        updated_at: '2026-07-31T00:00:00Z',
        contract_data: contractDocument,
        negotiations: [],
      }),
    retrieveNegotiationDraft: () => Promise.resolve(null),
    submit: vi.fn(() => Promise.resolve({ did: 'did:web:example.com:contract' })),
  },
}))

async function mountAs(roles: UserRole[]) {
  const pinia = createPinia()
  const wrapper = shallowMount(NegotiateContractView, { global: { plugins: [pinia] } })
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'did:web:example.com:org', holder: 'user', roles }
  await nextTick()
  await nextTick()
  await nextTick()
  return wrapper
}

const submitButton = async (roles: UserRole[]) =>
  (await mountAs(roles)).findAll('button').find((button) => button.text().includes('Submit'))

describe('closing a negotiation round', () => {
  it.each<UserRole>(['CONTRACT_CREATOR', 'CONTRACT_REVIEWER', 'CONTRACT_NEGOTIATOR'])(
    'enables Submit for %s',
    async (role) => {
      expect((await submitButton([role]))?.attributes('disabled')).toBeUndefined()
    },
  )

  // submit.go deliberately does not admit the manager to this transition.
  it.each<UserRole>(['CONTRACT_MANAGER', 'CONTRACT_OBSERVER'])('leaves Submit disabled for %s', async (role) => {
    expect((await submitButton([role]))?.attributes('disabled')).toBeDefined()
  })
})
