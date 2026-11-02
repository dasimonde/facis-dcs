import { config, flushPromises, RouterLinkStub, shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterAll, beforeAll, expect, it, vi } from 'vitest'
import { useContractEditorUiStore } from '@contract-workflow-engine/store/contractEditorUiStore'
import { useAuthStore } from '@/stores/auth-store'
import { useContractsStore } from '@/stores/contracts-store'
import ViewContractView from './ViewContractView.vue'
import type { Contract } from '@/models/contract/contract'

// DCS-FR-CWE-29: the structure tab shows the hierarchy (parent, children), the
// renewal links and the related documents of a contract.

vi.mock('vue-router', async (original) => ({
  ...(await original<typeof import('vue-router')>()),
  useRoute: () => ({ params: { did: 'did:test:current' } }),
  useRouter: () => ({ push: vi.fn() }),
}))
vi.mock('@/services/contract-workflow-service', () => ({
  contractWorkflowService: {
    retrieveById: () =>
      Promise.resolve({
        did: 'did:test:current',
        name: 'Current',
        state: 'ACTIVE',
        contract_version: 1,
        created_by: 'org',
        created_at: '',
        updated_at: '',
        contract_data: { 'dcs:parentContract': { '@id': 'did:test:frame' } },
      }),
    listEvidence: (did: string) =>
      Promise.resolve([{ evidence_id: 'e1', did, filename: 'annex-a.pdf', description: 'Price list' }]),
  },
}))

const listed = (did: string, extra: Partial<Contract> = {}): Contract =>
  ({ did, name: did.replace('did:test:', ''), state: 'ACTIVE', contract_version: 1, ...extra }) as Contract

beforeAll(() => {
  config.global.renderStubDefaultSlot = true
})
afterAll(() => {
  config.global.renderStubDefaultSlot = false
})

it('shows parent, child, renewal links and related documents', async () => {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'org', holder: 'user', roles: ['CONTRACT_MANAGER'] }
  useContractsStore().contracts = [
    listed('did:test:frame'),
    listed('did:test:current', { parent_contract_did: 'did:test:frame', renews_contract_did: 'did:test:previous' }),
    listed('did:test:annex', { parent_contract_did: 'did:test:current' }),
    listed('did:test:previous', { state: 'EXPIRED' }),
    listed('did:test:next', { state: 'DRAFT', renews_contract_did: 'did:test:current' }),
  ]

  useContractEditorUiStore().setActiveTab('structure')

  const wrapper = shallowMount(ViewContractView, {
    global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
  })
  await flushPromises()

  const linked = wrapper
    .findAllComponents(RouterLinkStub)
    .map((link) => (link.props('to') as { params: { did: string } }).params?.did)
  expect(linked).toContain('did:test:frame')
  expect(wrapper.findComponent({ name: 'ContractStructureTree' }).props('rootDid')).toBe('did:test:current')
  expect(wrapper.get('[data-testid="structure-renews"]').text()).toContain('previous')
  expect(wrapper.get('[data-testid="structure-renewed-by"]').text()).toContain('next')
  expect(wrapper.get('[data-testid="structure-documents"]').text()).toContain('annex-a.pdf')
})
