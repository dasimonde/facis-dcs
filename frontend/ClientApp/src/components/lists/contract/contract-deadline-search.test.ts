import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, expect, it, vi } from 'vitest'
import ContractListSearch from './ContractListSearch.vue'

const search = vi.hoisted(() => vi.fn())
vi.mock('@/services/contract-workflow-service', () => ({ contractWorkflowService: { search } }))
beforeEach(() => search.mockReset().mockResolvedValue([]))

it('submits deadline alone and then text with the latest deadline via Enter', async () => {
  const wrapper = mount(ContractListSearch, { props: { contracts: [] } })
  const deadline = wrapper.get('[aria-label="Deadline until"]')
  await deadline.setValue('2026-11-01')
  await deadline.trigger('keydown', { key: 'Enter' })
  await flushPromises()
  expect(search).toHaveBeenLastCalledWith({ valid_until: '2026-11-01T23:59:59Z' })
  await wrapper.get('[aria-label="Search contracts"]').setValue('contract')
  await flushPromises()
  await deadline.setValue('2026-12-01')
  await deadline.trigger('keydown', { key: 'Enter' })
  await flushPromises()
  expect(search).toHaveBeenLastCalledWith({ did: 'contract', valid_until: '2026-12-01T23:59:59Z' })
  expect(wrapper.findAll('button').filter((button) => button.text() === 'Search')).toHaveLength(1)
  await deadline.setValue('2027-01-01')
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Search')!
    .trigger('click')
  await flushPromises()
  expect(search).toHaveBeenLastCalledWith({ did: 'contract', valid_until: '2027-01-01T23:59:59Z' })
  expect(wrapper.get('.join').classes()).toEqual(expect.arrayContaining(['flex-col', 'sm:flex-row']))
  expect(deadline.element.parentElement?.classList.contains('w-full')).toBe(true)
  expect(deadline.element.parentElement?.classList.contains('sm:w-auto')).toBe(true)
  wrapper.unmount()
})

it.each(['resolve', 'reject'])(
  'invalidates a pending text search on deadline change before its late %s',
  async (outcome) => {
    let resolve!: (value: unknown[]) => void
    let reject!: (error: Error) => void
    search.mockReturnValueOnce(
      new Promise((yes, no) => {
        resolve = yes
        reject = no
      }),
    )
    const wrapper = mount(ContractListSearch, { props: { contracts: [] } })
    await wrapper.get('[aria-label="Search contracts"]').setValue('contract')
    const deadline = wrapper.get('[aria-label="Deadline until"]')
    await deadline.setValue('2026-12-01')
    await deadline.trigger('keydown', { key: 'Enter' })
    await flushPromises()
    expect(search).toHaveBeenLastCalledWith({ did: 'contract', valid_until: '2026-12-01T23:59:59Z' })
    const emitted = wrapper.emitted('searchResult')?.length
    if (outcome === 'resolve') resolve([{ did: 'stale' }])
    else reject(new Error('stale failure'))
    await flushPromises()
    expect(wrapper.emitted('searchResult')).toHaveLength(emitted!)
    expect(wrapper.text()).not.toContain('stale')
    wrapper.unmount()
  },
)
