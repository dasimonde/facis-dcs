import { flushPromises, mount, RouterLinkStub } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth-store'
import { useContractStateFilterStore } from '@/stores/state-filter-store'
import ContractList from './ContractList.vue'
import type { ContractChange } from '@/composables/useContractChanges'
import type { Contract } from '@/models/contract/contract'

const mocks = vi.hoisted(() => ({
  onChange: vi.fn<(change: ContractChange) => void>(),
  onGap: vi.fn<() => void>(),
  retrieve: vi.fn(),
  search: vi.fn(),
}))
vi.mock('@/composables/useContractChanges', () => ({
  useContractChanges: (onChange: (change: ContractChange) => void, onGap: () => void) => {
    mocks.onChange.mockImplementation(onChange)
    mocks.onGap.mockImplementation(onGap)
  },
}))
vi.mock('@/services/contract-workflow-service', () => ({
  contractWorkflowService: { retrieve: mocks.retrieve, search: mocks.search },
}))

const row = (did: string, state: Contract['state']): Contract => ({
  did,
  name: did,
  state,
  contract_version: 1,
  created_by: 'org',
  created_at: '',
  updated_at: `${did}-t`,
})
const page = (contracts: Contract[]) => ({ contracts, review_tasks: [], approval_tasks: [], negotiation_tasks: [] })

async function mountList(roles = ['CONTRACT_MANAGER']) {
  const pinia = createPinia()
  setActivePinia(pinia)
  useAuthStore().user = { issuer: 'org', holder: 'user', roles } as never
  const wrapper = mount(ContractList, { global: { plugins: [pinia], stubs: { RouterLink: RouterLinkStub } } })
  await flushPromises()
  return wrapper
}

beforeEach(() => {
  vi.resetAllMocks()
  mocks.retrieve.mockResolvedValue(page([row('a', 'SIGNED'), row('b', 'ACTIVE')]))
  mocks.search.mockResolvedValue([])
})

// DCS-IR-CWE-11, DCS-FR-CWE-24: the server filters before it cuts the page, so a
// matching contract on a later page is found.
it('a state filter finds a contract that is not on the first page', async () => {
  const wrapper = await mountList()
  expect(mocks.retrieve).toHaveBeenLastCalledWith({ limit: 25, offset: 1, sort_by: 'created_at', sort_order: 'desc' })

  mocks.retrieve.mockResolvedValue(page([row('late', 'TERMINATED')]))
  useContractStateFilterStore().setFilter('TERMINATED')
  await flushPromises()

  expect(mocks.retrieve).toHaveBeenLastCalledWith({
    limit: 25,
    offset: 1,
    state: ['TERMINATED'],
    sort_by: 'created_at',
    sort_order: 'desc',
  })
  expect(wrapper.text()).toContain('late')
  expect(wrapper.text()).not.toContain('No contracts found.')
  wrapper.unmount()
})

// DCS-FR-CWE-24: a bulk action never covers a row the filter hides.
it('clears the selection when the filter changes', async () => {
  const wrapper = await mountList()
  await wrapper.get('[data-testid="select-a"]').setValue(true)
  expect(wrapper.find('[data-testid="contracts-bulk-actions"]').exists()).toBe(true)

  mocks.retrieve.mockResolvedValue(page([row('b', 'ACTIVE')]))
  useContractStateFilterStore().setFilter('ACTIVE')
  await flushPromises()
  expect(wrapper.find('[data-testid="contracts-bulk-actions"]').exists()).toBe(false)

  // The row comes back without its old selection.
  mocks.retrieve.mockResolvedValue(page([row('a', 'SIGNED'), row('b', 'ACTIVE')]))
  useContractStateFilterStore().removeFilter('ACTIVE')
  await flushPromises()
  expect(wrapper.text()).toContain('SIGNED')
  expect(wrapper.find('[data-testid="contracts-bulk-actions"]').exists()).toBe(false)
  wrapper.unmount()
})

it('offers row selection only to a user with a bulk action', async () => {
  const creator = await mountList(['CONTRACT_CREATOR'])
  expect(creator.find('[data-testid="select-a"]').exists()).toBe(false)
  creator.unmount()
  const manager = await mountList(['CONTRACT_MANAGER'])
  expect(manager.find('[data-testid="select-a"]').exists()).toBe(true)
  manager.unmount()
})

// DCS-FR-CWE-27: changes the stream could not deliver mark the list outdated.
it('marks the list outdated after a stream gap or a failed refetch', async () => {
  const wrapper = await mountList()
  mocks.onGap()
  await flushPromises()
  expect(wrapper.find('[data-testid="contracts-outdated"]').exists()).toBe(true)

  await wrapper.get('[data-testid="contracts-outdated"] button').trigger('click')
  await flushPromises()
  expect(wrapper.find('[data-testid="contracts-outdated"]').exists()).toBe(false)

  mocks.search.mockRejectedValueOnce(new Error('unavailable'))
  mocks.onChange({ did: 'a', event_type: 'SUBMIT_CONTRACT' })
  await flushPromises()
  expect(wrapper.find('[data-testid="contracts-outdated"]').exists()).toBe(true)
  wrapper.unmount()
})

// DCS-FR-CWE-24: a reload keeps the active search and loads once.
it('keeps the active search on reload', async () => {
  const wrapper = await mountList()
  const retrieveCalls = mocks.retrieve.mock.calls.length
  mocks.search.mockResolvedValue([{ ...row('found', 'ACTIVE') }])
  const searchFn = wrapper.findComponent({ name: 'ListSearch' }).props('searchFn') as (
    request: Record<string, unknown>,
  ) => Promise<Contract[]>
  wrapper.findComponent({ name: 'ContractListSearch' }).vm.$emit('searchResult', await searchFn({ name: 'found' }))
  await flushPromises()
  expect(wrapper.text()).toContain('found')

  mocks.onChange({ did: 'other', event_type: 'CREATE_CONTRACT' })
  await flushPromises()
  await wrapper.get('[data-testid="contracts-outdated"] button').trigger('click')
  await flushPromises()

  expect(mocks.search).toHaveBeenLastCalledWith({ name: 'found' })
  expect(mocks.search).toHaveBeenCalledTimes(2)
  expect(mocks.retrieve).toHaveBeenCalledTimes(retrieveCalls)
  expect(wrapper.text()).toContain('found')
  wrapper.unmount()
})

// DCS-IR-CWE-11: a search result is filtered as a whole before the page is cut.
it('filters the whole search result before it cuts the page', async () => {
  const wrapper = await mountList()
  const results = Array.from({ length: 30 }, (_, index) => row(`hit-${index}`, index === 28 ? 'EXPIRED' : 'ACTIVE'))
  wrapper.findComponent({ name: 'ContractListSearch' }).vm.$emit('searchResult', results)
  await flushPromises()
  expect(wrapper.text()).not.toContain('hit-28')

  useContractStateFilterStore().setFilter('EXPIRED')
  await flushPromises()
  expect(wrapper.text()).toContain('hit-28')
  wrapper.unmount()
})

// DCS-IR-CWE-11: a server reload for a filter keeps the filter menu and the
// search mounted, so a second state can be picked in the open menu.
it('keeps the filter controls mounted while a filter reloads the page', async () => {
  const wrapper = await mountList()
  const filterUid = () => wrapper.findComponent({ name: 'ListStateFilter' }).vm.$.uid
  const mounted = filterUid()
  let resolve!: (value: unknown) => void
  mocks.retrieve.mockReturnValue(new Promise((done) => (resolve = done)))
  useContractStateFilterStore().setFilter('DRAFT')
  await flushPromises()

  expect(wrapper.text()).toContain('Loading contracts...')
  expect(filterUid()).toBe(mounted)
  expect(wrapper.findComponent({ name: 'ContractListSearch' }).exists()).toBe(true)
  resolve(page([row('drafted', 'DRAFT')]))
  await flushPromises()
  expect(filterUid()).toBe(mounted)
  expect(wrapper.text()).toContain('drafted')
  wrapper.unmount()
})
