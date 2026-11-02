import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import ListSearch from './ListSearch.vue'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (error: Error) => void
  const promise = new Promise<T>((yes, no) => {
    resolve = yes
    reject = no
  })
  return { promise, resolve, reject }
}
function setup() {
  const searchFn = vi.fn().mockResolvedValue([{ did: 'current' }])
  const wrapper = mount(ListSearch, {
    props: { items: [], emptyItem: { did: '' }, filterLabels: { did: 'DID' }, searchFn, filters: {} },
  })
  return { wrapper, searchFn }
}
describe('shared combined search request', () => {
  it('keeps text when party changes and keeps party when text is cleared', async () => {
    const { wrapper, searchFn } = setup()
    await wrapper.get('input').setValue('did:test:template')
    await wrapper.setProps({ filters: { party_type: 'MULTI_PARTY' } })
    await flushPromises()
    expect(searchFn).toHaveBeenLastCalledWith({ did: 'did:test:template', party_type: 'MULTI_PARTY' })
    expect(wrapper.emitted('searchResult')?.slice(-1)[0]).toEqual([[{ did: 'current' }]])
    await wrapper.get('input').setValue('')
    await flushPromises()
    expect(searchFn).toHaveBeenLastCalledWith({ party_type: 'MULTI_PARTY' })
    await wrapper.setProps({ filters: {} })
    await flushPromises()
    expect(wrapper.emitted('searchResult')?.slice(-1)[0]).toEqual([null])
    wrapper.unmount()
  })
  it.each(['resolve', 'reject'] as const)(
    'ignores a late %s after a newer combined query was published',
    async (outcome) => {
      const { wrapper, searchFn } = setup()
      const old = deferred<{ did: string }[]>()
      searchFn.mockReturnValueOnce(old.promise)
      await wrapper.get('input').setValue('did:test:old')
      await wrapper.get('button:last-of-type').trigger('click')
      await wrapper.setProps({ filters: { party_type: 'MULTI_PARTY' } })
      await flushPromises()
      const emissions = wrapper.emitted('searchResult')?.length
      if (outcome === 'resolve') old.resolve([{ did: 'stale' }])
      else old.reject(new Error('Stale failure'))
      await flushPromises()
      expect(wrapper.emitted('searchResult')).toHaveLength(emissions ?? 0)
      expect(wrapper.emitted('searchResult')?.slice(-1)[0]).toEqual([[{ did: 'current' }]])
      expect(wrapper.text()).not.toContain('stale')
      expect(wrapper.text()).not.toContain('Stale failure')
      wrapper.unmount()
    },
  )
  it('clearing the query invalidates a pending result for callers without additional filters', async () => {
    const { wrapper, searchFn } = setup()
    const old = deferred<{ did: string }[]>()
    searchFn.mockReturnValueOnce(old.promise)
    await wrapper.get('input').setValue('old')
    await wrapper.get('button:last-of-type').trigger('click')
    await wrapper.get('input').setValue('')
    await flushPromises()
    old.resolve([{ did: 'stale' }])
    await flushPromises()
    expect(wrapper.emitted('searchResult')).toEqual([[null]])
    wrapper.unmount()
  })
})

it.each(['Enter', ' '])('selects a named filter with %s and focuses the first popover option', async (key) => {
  const wrapper = mount(ListSearch, {
    attachTo: document.body,
    props: {
      items: [],
      emptyItem: { did: '', name: '' },
      filterLabels: { did: 'DID', name: 'Name' },
      searchFn: vi.fn().mockResolvedValue([]),
    },
  })
  const popover = wrapper.get('#list-popover-search')
  const hidePopover = vi.fn()
  Object.assign(popover.element, { hidePopover })
  // jsdom has no popover API and refuses focus inside a closed popover; emulate the open state.
  ;(popover.element as HTMLElement).style.display = 'block'
  await popover.trigger('toggle', { newState: 'open' })
  const options = popover.findAll('a')
  expect(popover.attributes('aria-label')).toBe('Select search filter')
  expect(document.activeElement).toBe(options[0]!.element)
  await options[1]!.trigger('keydown', { key })
  expect(wrapper.get('#list-btn-search').text()).toBe('Name')
  expect(hidePopover).toHaveBeenCalled()
  wrapper.unmount()
})
