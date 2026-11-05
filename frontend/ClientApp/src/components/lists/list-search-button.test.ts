import { flushPromises, mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
import ListSearch from './ListSearch.vue'

it('Search uses typed text even when Filters are empty and change has not fired', async () => {
  const searchFn = vi.fn().mockResolvedValue([{ did: 'did:web:hit' }])
  const wrapper = mount(ListSearch, {
    props: {
      items: [],
      emptyItem: { did: '' },
      filterLabels: { did: 'DID' },
      searchFn,
      extraRequest: {},
    },
  })

  const input = wrapper.get('input')

  ;(input.element as HTMLInputElement).value = 'did:web:hit'
  await wrapper
    .findAll('button')
    .find((button) => button.text() === 'Search')!
    .trigger('click')
  await flushPromises()

  expect(searchFn).toHaveBeenCalledWith({ did: 'did:web:hit' })
  const emitted = wrapper.emitted('searchResult')!
  expect(emitted[emitted.length - 1]?.[0]).toEqual([{ did: 'did:web:hit' }])
  wrapper.unmount()
})
