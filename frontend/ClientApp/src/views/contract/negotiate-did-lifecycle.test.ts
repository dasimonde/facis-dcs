import { flushPromises, shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { reactive } from 'vue'
import { useDcsDraftStore } from '@template-repository/store/dcsDraftStore'
import { useContractEditorUiStore } from '@contract-workflow-engine/store/contractEditorUiStore'
import { useAuthStore } from '@/stores/auth-store'
import ApproveContractView from './ApproveContractView.vue'
import NegotiateContractView from './NegotiateContractView.vue'
import ReviewContractView from './ReviewContractView.vue'
import ViewContractView from './ViewContractView.vue'
import type { Contract } from '@/models/contract/contract'

const mocks = vi.hoisted(() => ({ retrieve: vi.fn(), draft: vi.fn(), identity: vi.fn() }))
const route = reactive({ params: { did: 'did:test:first' } })
vi.mock('vue-router', async (original) => ({
  ...(await original<typeof import('vue-router')>()),
  useRoute: () => route,
}))
vi.mock('@/services/did-service', () => ({ resolveLocalInstanceDid: mocks.identity }))
vi.mock('@/services/contract-workflow-service', () => ({
  contractWorkflowService: { retrieveById: mocks.retrieve, retrieveNegotiationDraft: mocks.draft },
}))

function contract(did: string): Contract {
  return {
    did,
    name: 'Formal',
    contract_version: 1,
    state: 'NEGOTIATION',
    created_by: 'org',
    created_at: '',
    updated_at: '',
    negotiations: [
      {
        id: 'accepted',
        created_by: 'org',
        created_at: '',
        contract_version: 1,
        change_request: { name: `Accepted ${did}` },
        negotiation_decisions: [{ negotiator: 'local', decision: 'ACCEPTED' }],
      },
    ],
  }
}
beforeEach(() => {
  vi.resetAllMocks()
  route.params.did = 'did:test:first'
  mocks.identity.mockResolvedValue('local')
  mocks.retrieve.mockImplementation(({ did }: { did: string }) => Promise.resolve(contract(did)))
  mocks.draft.mockResolvedValue(null)
})
afterEach(() => vi.useRealTimers())
function setup(component = NegotiateContractView) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'org', holder: 'user', roles: ['CONTRACT_MANAGER'] }
  return shallowMount(component, { attachTo: document.body, global: { plugins: [pinia] } })
}
it('keeps the accepted baseline and audit contract for the new DID when an old draft arrives', async () => {
  let resolve!: (value: unknown) => void
  mocks.draft.mockReturnValueOnce(
    new Promise((done) => {
      resolve = done
    }),
  )
  const wrapper = setup()
  await flushPromises()
  route.params.did = 'did:test:second'
  await flushPromises()
  resolve({ change_request: { name: 'Stale draft' } })
  await flushPromises()
  expect(wrapper.findComponent({ name: 'ContractDetailsEditor' }).props('contract').name).toBe(
    'Accepted did:test:second',
  )
  const audit = wrapper.findComponent({ name: 'AuditView' })
  expect(audit.props('contract').did).toBe('did:test:second')
  expect(audit.props('blocks')).toEqual(useDcsDraftStore().blocks)
  expect(audit.props('layout')).toEqual(useDcsDraftStore().layout)
  expect(useContractEditorUiStore().activeTab).toBe('details')
  wrapper.unmount()
})
it.each([NegotiateContractView, ReviewContractView, ApproveContractView])(
  'does not retrieve the old DID after identity resolution crosses navigation',
  async (component) => {
    let resolve!: (value: string) => void
    mocks.identity.mockReturnValueOnce(
      new Promise((done) => {
        resolve = done
      }),
    )
    const wrapper = setup(component)
    route.params.did = 'did:test:second'
    await flushPromises()
    resolve('local')
    await flushPromises()
    expect(mocks.retrieve).not.toHaveBeenCalledWith({ did: 'did:test:first' })
    expect(wrapper.findComponent({ name: 'AuditView' }).props('contract').did).toBe('did:test:second')
    wrapper.unmount()
  },
)

it.each([ReviewContractView, ApproveContractView])(
  'keeps the new contract and peer proposal alert when a previous-contract response arrives late',
  async (component) => {
    const current = contract('did:test:first')
    current.state = 'REVIEWED'
    current.negotiations![0]!.negotiation_decisions = [{ negotiator: 'local', decision: undefined }]
    mocks.retrieve.mockResolvedValueOnce(current)
    const wrapper = setup(component)
    await flushPromises()
    expect(mocks.identity).toHaveBeenCalled()
    expect(wrapper.findComponent({ name: 'PeerProposalPendingAlert' }).props('show')).toBe(true)

    let resolve!: (value: Contract) => void
    mocks.retrieve.mockReturnValueOnce(
      new Promise<Contract>((done) => {
        resolve = done
      }),
    )
    route.params.did = 'did:test:second'
    await flushPromises()
    expect(wrapper.findComponent({ name: 'AuditView' }).exists()).toBe(false)
    mocks.retrieve.mockResolvedValueOnce({ ...current, did: 'did:test:third', negotiations: [] })
    route.params.did = 'did:test:third'
    await flushPromises()
    resolve({ ...current, did: 'did:test:second' })
    await flushPromises()
    expect(wrapper.findComponent({ name: 'AuditView' }).props('contract').did).toBe('did:test:third')
    expect(wrapper.findComponent({ name: 'PeerProposalPendingAlert' }).props('show')).toBe(false)
    wrapper.unmount()
  },
)

it.each([NegotiateContractView, ReviewContractView, ApproveContractView, ViewContractView])(
  'renders focusable native tab controls with selected state',
  async (component) => {
    const wrapper = setup(component)
    await flushPromises()
    const tabs = wrapper.findAll('[role="tab"]')
    expect(tabs.length).toBeGreaterThan(1)
    for (const tab of tabs) {
      expect(tab.element.tagName).toBe('BUTTON')
      expect(tab.attributes('type')).toBe('button')
      expect(tab.attributes('aria-selected')).toBeDefined()
    }
    const content = tabs.find((tab) => tab.text() === 'Contract Content')!
    ;(content.element as HTMLButtonElement).focus()
    expect(document.activeElement).toBe(content.element)
    await content.trigger('click')
    expect(content.attributes('aria-selected')).toBe('true')
    expect(tabs[0]!.attributes('aria-selected')).toBe('false')
    wrapper.unmount()
  },
)

it('ignores an old pending sync poll after navigation to another contract', async () => {
  vi.useFakeTimers()
  mocks.retrieve.mockResolvedValueOnce({ ...contract('did:test:first'), pending_negotiation_sync: true })
  const wrapper = setup()
  await flushPromises()
  let resolve!: (value: Contract) => void
  mocks.retrieve.mockReturnValueOnce(
    new Promise((done) => {
      resolve = done
    }),
  )
  await vi.advanceTimersByTimeAsync(3000)
  expect(mocks.retrieve).toHaveBeenLastCalledWith({ did: 'did:test:first' })
  route.params.did = 'did:test:second'
  await flushPromises()
  resolve({ ...contract('did:test:first'), updated_at: 'old-poll', pending_negotiation_sync: true, negotiations: [] })
  await flushPromises()
  const shown = wrapper.findComponent({ name: 'AuditView' }).props('contract')
  expect(shown).toEqual(contract('did:test:second'))
  expect(wrapper.findComponent({ name: 'ContractDetailsEditor' }).props('contract').name).toBe(
    'Accepted did:test:second',
  )
  wrapper.unmount()
})
