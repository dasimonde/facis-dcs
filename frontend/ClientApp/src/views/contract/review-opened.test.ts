import { flushPromises, shallowMount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import { ContractState } from '@/types/contract-state'
import ReviewContractView from './ReviewContractView.vue'

const mocks = vi.hoisted(() => ({ retrieve: vi.fn(), review: vi.fn() }))
vi.mock('vue-router', async (original) => ({
  ...(await original<typeof import('vue-router')>()),
  useRoute: () => ({ params: { did: 'did:test:c1' } }),
}))
vi.mock('@/services/did-service', () => ({ resolveLocalInstanceDid: () => Promise.resolve('local') }))
vi.mock('@/services/contract-workflow-service', () => ({
  contractWorkflowService: { retrieveById: mocks.retrieve, review: mocks.review },
}))

const contract = (state: string) => ({
  did: 'did:test:c1',
  name: 'C1',
  contract_version: 1,
  state,
  created_by: 'org',
  created_at: '',
  updated_at: '',
})

beforeEach(() => {
  vi.resetAllMocks()
  mocks.review.mockResolvedValue({ did: 'did:test:c1' })
})

async function mountAs(roles: string[]) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'org', holder: 'user', roles } as never
  const wrapper = shallowMount(ReviewContractView, { global: { plugins: [pinia] } })
  await flushPromises()
  return wrapper
}

// DCS-IR-CWE-04: opening a submitted contract for review is recorded once.
it('records that the reviewer opened the submitted contract', async () => {
  mocks.retrieve.mockResolvedValue(contract(ContractState.submitted))
  const wrapper = await mountAs(['CONTRACT_REVIEWER'])
  expect(mocks.review).toHaveBeenCalledOnce()
  expect(mocks.review).toHaveBeenCalledWith({ did: 'did:test:c1' })
  wrapper.unmount()
})

it('records no review opening for other roles or states', async () => {
  mocks.retrieve.mockResolvedValue(contract(ContractState.submitted))
  ;(await mountAs(['CONTRACT_OBSERVER'])).unmount()
  mocks.retrieve.mockResolvedValue(contract(ContractState.reviewed))
  ;(await mountAs(['CONTRACT_REVIEWER'])).unmount()
  expect(mocks.review).not.toHaveBeenCalled()
})
