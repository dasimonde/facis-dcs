import { flushPromises, shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { useAuthStore } from '@/stores/auth-store'
import NewContractView from './NewContractView.vue'
import type { ContractHistoryItem } from '@/models/responses/contract-response'

const mocks = vi.hoisted(() => ({
  retrieveById: vi.fn(),
  update: vi.fn(),
  retrieve: vi.fn(),
  reveal: vi.fn(),
}))
vi.mock('vue-router', async (original) => ({
  ...(await original<typeof import('vue-router')>()),
  useRoute: () => ({ params: { did: 'did:test:draft' } }),
  useRouter: () => ({ push: vi.fn() }),
  onBeforeRouteLeave: vi.fn(),
}))
vi.mock('@/services/contract-workflow-service', () => ({
  contractWorkflowService: { retrieveById: mocks.retrieveById, update: mocks.update, retrieve: mocks.retrieve },
}))

const ConfirmationModalStub = defineComponent({
  setup(_, { expose }) {
    expose({ reveal: mocks.reveal })
    return () => null
  },
})

const contractDocument = {
  '@type': 'dcs:Contract',
  '@id': 'did:test:draft',
  'dcs:metadata': { '@type': 'dcs:ContractMetadata' },
  'dcs:documentStructure': {
    '@type': 'dcs:DocumentStructure',
    'dcs:blocks': { '@list': [] },
    'dcs:layout': { '@list': [] },
  },
  'dcs:contractFields': [],
  'dcs:contractData': [],
}

const draft = () => ({
  did: 'did:test:draft',
  name: 'Current name',
  description: 'Current description',
  contract_version: 2,
  state: 'DRAFT',
  created_by: 'org',
  created_at: '',
  updated_at: '2026-10-30T12:00:00Z',
  start_date: '2027-01-01T00:00:00Z',
  contract_data: contractDocument,
})

const version: ContractHistoryItem = {
  id: 'history-1',
  did: 'did:test:draft',
  contract_version: 1,
  state: 'DRAFT',
  name: 'Old name',
  description: 'Old description',
  created_by: 'org',
  created_at: '',
  updated_at: '',
  start_date: '2027-02-01T00:00:00Z',
  exp_date: '2028-02-01T00:00:00Z',
  exp_policy: 'RENEW' as never,
  exp_notice_period: 30,
  contract_data: contractDocument as never,
}

beforeEach(() => {
  vi.resetAllMocks()
  mocks.retrieveById.mockImplementation(() => Promise.resolve(draft()))
  mocks.update.mockResolvedValue({})
  mocks.retrieve.mockResolvedValue({ contracts: [], review_tasks: [], approval_tasks: [], negotiation_tasks: [] })
})

async function mountEditor() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'org', holder: 'user', roles: ['CONTRACT_CREATOR'] } as never
  const wrapper = shallowMount(NewContractView, {
    global: { plugins: [pinia], stubs: { ConfirmationModal: ConfirmationModalStub } },
  })
  await flushPromises()
  return wrapper
}

// DCS-FR-CWE-08: a restored version brings back every versioned field.
it('restores every versioned field of the picked version', async () => {
  const wrapper = await mountEditor()
  wrapper.findComponent({ name: 'ContractHistoryDiffView' }).vm.$emit('restore', version)
  await flushPromises()
  expect(mocks.reveal).not.toHaveBeenCalled()
  expect(mocks.update).toHaveBeenCalledWith({
    did: 'did:test:draft',
    updated_at: '2026-10-30T12:00:00Z',
    name: 'Old name',
    description: 'Old description',
    start_date: '2027-02-01T00:00:00Z',
    exp_date: '2028-02-01T00:00:00Z',
    exp_policy: 'RENEW',
    exp_notice_period: 30,
    contract_data: contractDocument,
  })
  wrapper.unmount()
})

it('asks before a restore discards unsaved edits', async () => {
  mocks.reveal.mockResolvedValue({ isCanceled: true })
  const wrapper = await mountEditor()
  wrapper.findComponent({ name: 'ContractDetailsEditor' }).props('contract').name = 'Unsaved edit'
  wrapper.findComponent({ name: 'ContractHistoryDiffView' }).vm.$emit('restore', version)
  await flushPromises()
  expect(mocks.reveal).toHaveBeenCalledOnce()
  expect(mocks.update).not.toHaveBeenCalled()
  wrapper.unmount()
})

// DCS-IR-CWE-02: the creation UI offers the evidence panel for the draft.
it('shows the evidence panel while the draft is edited', async () => {
  const wrapper = await mountEditor()
  const panel = wrapper.findComponent({ name: 'ContractEvidencePanel' })
  expect(panel.exists()).toBe(true)
  expect(panel.props('contract').did).toBe('did:test:draft')
  wrapper.unmount()
})
