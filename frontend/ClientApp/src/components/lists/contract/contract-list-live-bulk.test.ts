import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import { useContractsStore } from '@/stores/contracts-store'
import ContractList from './ContractList.vue'
import type { ContractChange } from '@/composables/useContractChanges'
import type { Contract } from '@/models/contract/contract'

const mocks = vi.hoisted(() => ({
  onChange: vi.fn<(change: ContractChange) => void>(),
  search: vi.fn(),
  terminate: vi.fn(),
  add: vi.fn(),
}))
vi.mock('@/composables/useContractChanges', () => ({
  useContractChanges: (fn: (change: ContractChange) => void) => {
    mocks.onChange.mockImplementation(fn)
  },
}))
vi.mock('@/services/contract-workflow-service', () => ({
  contractWorkflowService: { search: mocks.search, terminate: mocks.terminate },
}))
vi.mock('@/stores/error-store', () => ({ useErrorStore: () => ({ add: mocks.add, remove: vi.fn() }) }))

const row = (did: string, state: Contract['state']): Contract => ({
  did,
  name: did,
  state,
  contract_version: 1,
  created_by: 'org',
  created_at: '',
  updated_at: `${did}-t`,
})

async function mountList() {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'org', holder: 'user', roles: ['CONTRACT_MANAGER'] } as never
  const store = useContractsStore()
  const load = vi.fn(() => {
    store.paginatedContracts = [row('a', 'SIGNED'), row('b', 'SIGNED')]
    return Promise.resolve()
  })
  store.loadPaginatedContracts = load
  const wrapper = mount(ContractList, { global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } } })
  await flushPromises()
  return { wrapper, load }
}

beforeEach(() => {
  vi.resetAllMocks()
  HTMLDialogElement.prototype.showModal = vi.fn()
  HTMLDialogElement.prototype.close = vi.fn()
})

// DCS-FR-CWE-24: a change refetches only the changed contract; the list is not
// read again, so no list-wide retrieval is audited per change.
it('updates a changed contract in place and offers a reload for others', async () => {
  const { wrapper, load } = await mountList()
  expect(load).toHaveBeenCalledTimes(1)
  mocks.search.mockResolvedValue([{ ...row('a', 'TERMINATED') }])

  mocks.onChange({ did: 'a', event_type: 'TERMINATE_CONTRACT' })
  await flushPromises()
  expect(mocks.search).toHaveBeenCalledWith({ did: 'a' })
  expect(wrapper.text()).toContain('TERMINATED')
  expect(load).toHaveBeenCalledTimes(1)

  mocks.onChange({ did: 'new', event_type: 'CREATE_CONTRACT' })
  await flushPromises()
  expect(wrapper.find('[data-testid="contracts-outdated"]').exists()).toBe(true)
  expect(load).toHaveBeenCalledTimes(1)
  // DCS-FR-CSA-23: the list passes each change on, so the attention list reloads.
  expect(wrapper.emitted('change')).toEqual([
    [{ did: 'a', event_type: 'TERMINATE_CONTRACT' }],
    [{ did: 'new', event_type: 'CREATE_CONTRACT' }],
  ])
  wrapper.unmount()
})

// DCS-FR-CWE-24: a bulk action calls the per-contract endpoint for each
// selected contract and reports one summary.
it('terminates the selected contracts with one reason', async () => {
  const { wrapper } = await mountList()
  mocks.terminate.mockResolvedValueOnce({ did: 'a' }).mockRejectedValueOnce(new Error('refused'))
  await wrapper.get('[data-testid="select-a"]').setValue(true)
  await wrapper.get('[data-testid="select-b"]').setValue(true)
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Terminate selected')!
    .trigger('click')
  await flushPromises()
  const modals = document.querySelectorAll('dialog')
  const modal = modals[modals.length - 1]!
  const reason = modal.querySelector('textarea')!
  reason.value = 'end of cooperation'
  reason.dispatchEvent(new Event('input'))
  await flushPromises()
  modal.querySelector<HTMLButtonElement>('[data-testid="confirmation-confirm"]')!.click()
  await flushPromises()

  expect(mocks.terminate).toHaveBeenCalledWith({ did: 'a', updated_at: 'a-t', reason: 'end of cooperation' })
  expect(mocks.terminate).toHaveBeenCalledWith({ did: 'b', updated_at: 'b-t', reason: 'end of cooperation' })
  expect(mocks.add).toHaveBeenCalledWith('Terminate: 1 succeeded, 1 failed (b: refused).', 'error', 8000)
  wrapper.unmount()
})
