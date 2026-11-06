import { flushPromises, mount } from '@vue/test-utils'
import { expect, it, vi } from 'vitest'
import ListSearch from './ListSearch.vue'

function searchButton(wrapper: ReturnType<typeof mount>) {
  return wrapper.findAll('button').find((button) => button.text() === 'Search')!
}

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
  await searchButton(wrapper).trigger('click')
  await flushPromises()

  expect(searchFn).toHaveBeenCalledWith({ did: 'did:web:hit' })
  const emitted = wrapper.emitted('searchResult')!
  expect(emitted[emitted.length - 1]?.[0]).toEqual([{ did: 'did:web:hit' }])
  wrapper.unmount()
})

it('keeps Search enabled while autocomplete is in flight', async () => {
  let resolveSearch!: (value: { did: string }[]) => void
  const searchFn = vi.fn(
    () =>
      new Promise<{ did: string }[]>((resolve) => {
        resolveSearch = resolve
      }),
  )
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
  await input.trigger('input')
  await input.trigger('change')
  await flushPromises()

  expect(searchFn).toHaveBeenCalled()
  expect(searchButton(wrapper).attributes('disabled')).toBeUndefined()

  resolveSearch([{ did: 'did:web:hit' }])
  await flushPromises()
  expect(wrapper.emitted('searchResult')).toBeUndefined()
  wrapper.unmount()
})
