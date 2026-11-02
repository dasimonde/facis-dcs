import { config, flushPromises, RouterLinkStub, shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterAll, beforeAll, beforeEach, expect, it, vi } from 'vitest'
import { useContractEditorUiStore } from '@contract-workflow-engine/store/contractEditorUiStore'
import { useAuthStore } from '@/stores/auth-store'
import { useContractsStore } from '@/stores/contracts-store'
import ViewContractView from './ViewContractView.vue'
import type { Contract } from '@/models/contract/contract'

const mocks = vi.hoisted(() => ({ retrieve: vi.fn(), lifecycle: vi.fn(), listEvidence: vi.fn() }))
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
      }),
    retrieve: mocks.retrieve,
    lifecycle: mocks.lifecycle,
    listEvidence: mocks.listEvidence,
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
beforeEach(() => {
  vi.resetAllMocks()
  mocks.listEvidence.mockResolvedValue([])
  mocks.retrieve.mockResolvedValue({ contracts: [], review_tasks: [], approval_tasks: [], negotiation_tasks: [] })
})

function mountAs(roles: string[]) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'org', holder: 'user', roles } as never
  return pinia
}

// DCS-FR-CWE-29: a renewal created after the list was loaded shows when the structure tab opens.
it('reloads children and renewals when the structure tab opens', async () => {
  const pinia = mountAs(['CONTRACT_MANAGER'])
  useContractsStore().contracts = [listed('did:test:current')]
  mocks.retrieve.mockResolvedValue({
    contracts: [listed('did:test:current'), listed('did:test:next', { renews_contract_did: 'did:test:current' })],
    review_tasks: [],
    approval_tasks: [],
    negotiation_tasks: [],
  })
  useContractEditorUiStore().setActiveTab('structure')
  const wrapper = shallowMount(ViewContractView, {
    global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } },
  })
  await flushPromises()
  expect(wrapper.get('[data-testid="structure-renewed-by"]').text()).toContain('next')
})

// DCS-IR-CWE-04, DCS-FR-CWE-17: the version comparison stays available after review.
it('offers the version comparison to an observer', async () => {
  const pinia = mountAs(['CONTRACT_OBSERVER'])
  const wrapper = shallowMount(ViewContractView, { global: { plugins: [pinia] } })
  await flushPromises()
  const diff = wrapper.findComponent({ name: 'ContractHistoryDiffView' })
  expect(diff.exists()).toBe(true)
  expect(diff.props('restorable')).toBeFalsy()
})

// DCS-IR-CWE-12: Auditor and Compliance Officer inspect the contract with its
// evidence, and the page makes no call they hold no scope for.
it.each(['AUDITOR', 'COMPLIANCE_OFFICER'])('opens the contract with its evidence for %s', async (role) => {
  const { router, ROUTES } = await import('@/router/router')
  const view = router.getRoutes().find((route) => route.name === ROUTES.CONTRACTS.VIEW)
  expect(view?.meta.roles).toContain(role)

  const pinia = mountAs([role])
  useContractEditorUiStore().setActiveTab('audit')
  const wrapper = shallowMount(ViewContractView, { global: { plugins: [pinia] } })
  await flushPromises()
  expect(wrapper.findComponent({ name: 'AuditView' }).exists()).toBe(true)
  expect(wrapper.findComponent({ name: 'ContractLifecycleHistory' }).exists()).toBe(false)
  expect(wrapper.findComponent({ name: 'ContractHistoryDiffView' }).exists()).toBe(false)
  expect(mocks.retrieve).not.toHaveBeenCalled()
})
